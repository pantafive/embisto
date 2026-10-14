package thumbs

// Pictures are resized in pure Go: the size is read from the header before decoding, what is too large is refused,
// x/image/draw scales, and PNG is encoded when the result has transparency, JPEG otherwise.

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"slices"

	"github.com/gen2brain/vpx/webp"
	"golang.org/x/image/draw"

	// The decoders the header check and the decoding use; GIF gives its first frame.
	_ "image/gif"
	// The webp decoder, which the standard library lacks.
	_ "golang.org/x/image/webp"
)

var (
	// ErrUnsupported is a file that is not a JPEG, PNG, GIF or WebP picture.
	ErrUnsupported = errors.New("not a JPEG, PNG, GIF or WebP picture")
	// ErrTooManyPixels is a picture whose header states more pixels than the limit.
	ErrTooManyPixels = errors.New("the picture has too many pixels")
)

// formats are the ones decoded; SVG and ICO are refused, never passed through.
var formats = []string{"jpeg", "png", "gif", "webp"}

// jpegQuality is a quality at which artefacts are rare and files stay small.
const jpegQuality = 82

// webpQuality matches the JPEG quality's look at about half the bytes.
const webpQuality = 75

// resized is an encoded picture.
type resized struct {
	Body          []byte
	ContentType   string
	Width, Height int
}

// What one picture may make the scaler hold, whatever its box: a picture far taller than wide keeps its top, one far
// wider keeps its middle, and no output is larger than a megapixel.
const (
	maxAspect       = 3
	maxOutputPixels = 1_000_000
	// kernelBytes is what CatmullRom keeps of a pixel in its intermediate image, the output's width by the source's
	// height: four float64 channels.
	kernelBytes = 32
	rgbaBytes   = 4
	// encoderBytes is what an encoder holds of an output pixel at most, measured with WebP's, the largest.
	encoderBytes = 16
)

// plan is how a picture is made into its box: the part of the source kept, the size of an intermediate the source is
// first shrunk to, when it is more than twice the output, and the output's size.
type plan struct {
	crop          image.Rectangle
	mid           image.Point
	width, height int
}

func planOf(bounds image.Rectangle, box Box) plan {
	var crop image.Rectangle
	var width, height int
	if box.Mode == Fill && box.Width > 0 && box.Height > 0 {
		width, height = min(box.Width, bounds.Dx()), min(box.Height, bounds.Dy())
		crop = centered(bounds, float64(width)/float64(height))
	} else {
		crop = limitedAspect(bounds)
		width, height = fitted(crop.Dx(), crop.Dy(), box)
	}
	if pixels := width * height; pixels > maxOutputPixels {
		ratio := math.Sqrt(float64(maxOutputPixels) / float64(pixels))
		width, height = max(1, int(float64(width)*ratio)), max(1, int(float64(height)*ratio))
	}
	shaped := plan{crop: crop, width: width, height: height}
	if crop.Dx() > 2*width || crop.Dy() > 2*height {
		shaped.mid = image.Pt(min(crop.Dx(), 2*width), min(crop.Dy(), 2*height))
	}
	return shaped
}

// scalingBytes is what scaling and encoding hold besides the decoded source: the intermediate, CatmullRom's own, the
// output and the encoder's.
func (p plan) scalingBytes() int64 {
	sourceHeight := p.crop.Dy()
	if p.mid != (image.Point{}) {
		sourceHeight = p.mid.Y
	}
	return int64(rgbaBytes*p.mid.X*p.mid.Y + kernelBytes*p.width*sourceHeight + (rgbaBytes+encoderBytes)*p.width*p.height)
}

// limitedAspect is the part of a picture within maxAspect: the top of a tall one, the middle of a wide one.
func limitedAspect(bounds image.Rectangle) image.Rectangle {
	width, height := bounds.Dx(), bounds.Dy()
	switch {
	case height > maxAspect*width:
		return image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Min.Y+maxAspect*width)
	case width > maxAspect*height:
		left := bounds.Min.X + (width-maxAspect*height)/2
		return image.Rect(left, bounds.Min.Y, left+maxAspect*height, bounds.Max.Y)
	default:
		return bounds
	}
}

// fitted is the size a picture takes inside the box, keeping its proportions and never growing.
func fitted(width, height int, box Box) (int, int) {
	ratio := 1.0
	if box.Width > 0 {
		ratio = min(ratio, float64(box.Width)/float64(width))
	}
	if box.Height > 0 {
		ratio = min(ratio, float64(box.Height)/float64(height))
	}
	return max(1, int(float64(width)*ratio+0.5)), max(1, int(float64(height)*ratio+0.5))
}

// Cost is the memory resizing the picture into the box takes, from its header alone: the decoded picture and the
// scaling. It fails as resize would for a picture it refuses.
func Cost(source []byte, box Box, maxPixels int) (int64, error) {
	config, err := header(source, maxPixels)
	if err != nil {
		return 0, err
	}
	decoded := int64(config.Width) * int64(config.Height) * int64(bytesPerPixel(config.ColorModel))
	return decoded + planOf(image.Rect(0, 0, config.Width, config.Height), box).scalingBytes(), nil
}

// bytesPerPixel is what the decoder keeps a pixel of the colour model in.
func bytesPerPixel(model color.Model) int {
	switch model {
	case color.RGBA64Model, color.NRGBA64Model:
		return 8
	case color.Gray16Model:
		return 2
	case color.GrayModel, color.AlphaModel:
		return 1
	default:
		return rgbaBytes
	}
}

func header(source []byte, maxPixels int) (image.Config, error) {
	config, kind, err := image.DecodeConfig(bytes.NewReader(source))
	if err != nil || !slices.Contains(formats, kind) {
		return image.Config{}, ErrUnsupported
	}
	if config.Width*config.Height > maxPixels {
		return image.Config{}, fmt.Errorf("%w: %dx%d", ErrTooManyPixels, config.Width, config.Height)
	}
	return config, nil
}

// resize decodes a picture of at most maxPixels, scales it into the box, never up, and encodes it.
func resize(source []byte, box Box, maxPixels int, format Format) (resized, error) {
	if _, err := header(source, maxPixels); err != nil {
		return resized{}, err
	}
	decoded, _, err := image.Decode(bytes.NewReader(source))
	if err != nil {
		return resized{}, fmt.Errorf("%w: %w", ErrUnsupported, err)
	}
	scaled := scale(decoded, box)
	if format == WebP {
		return encodeWebP(scaled)
	}
	return encode(scaled)
}

// Scaled is the size a picture of the given size takes in the box: never larger than the picture itself, within the
// aspect and the area every output keeps to.
func Scaled(width, height int, box Box) (int, int) {
	if width <= 0 || height <= 0 {
		return 0, 0
	}
	shaped := planOf(image.Rect(0, 0, width, height), box)
	return shaped.width, shaped.height
}

func scale(source image.Image, box Box) image.Image {
	shaped := planOf(source.Bounds(), box)
	if shaped.crop == source.Bounds() && shaped.width == shaped.crop.Dx() && shaped.height == shaped.crop.Dy() {
		return source
	}
	crop := shaped.crop
	if shaped.mid != (image.Point{}) {
		// A fast pass first: CatmullRom keeps the output's width by the source's height in float64s, which for a
		// picture many times the output's size is most of the memory resizing takes.
		mid := image.NewRGBA(image.Rectangle{Max: shaped.mid})
		draw.ApproxBiLinear.Scale(mid, mid.Bounds(), source, crop, draw.Src, nil)
		source, crop = mid, mid.Bounds()
	}
	scaled := image.NewRGBA(image.Rect(0, 0, shaped.width, shaped.height))
	draw.CatmullRom.Scale(scaled, scaled.Bounds(), source, crop, draw.Src, nil)
	return scaled
}

// centered is the largest rectangle of the given proportions in the middle of bounds.
func centered(bounds image.Rectangle, ratio float64) image.Rectangle {
	width, height := bounds.Dx(), bounds.Dy()
	if float64(width)/float64(height) > ratio {
		cropped := int(float64(height)*ratio + 0.5)
		left := bounds.Min.X + (width-cropped)/2
		return image.Rect(left, bounds.Min.Y, left+cropped, bounds.Max.Y)
	}
	cropped := int(float64(width)/ratio + 0.5)
	top := bounds.Min.Y + (height-cropped)/2
	return image.Rect(bounds.Min.X, top, bounds.Max.X, top+cropped)
}

// encode writes PNG when any pixel is transparent, so transparency survives; JPEG otherwise, being far smaller.
func encode(picture image.Image) (resized, error) {
	var out bytes.Buffer
	result := resized{Width: picture.Bounds().Dx(), Height: picture.Bounds().Dy()}
	if isOpaque(picture) {
		if err := jpeg.Encode(&out, picture, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return resized{}, fmt.Errorf("encoding JPEG: %w", err)
		}
		result.ContentType = "image/jpeg"
	} else {
		if err := png.Encode(&out, picture); err != nil {
			return resized{}, fmt.Errorf("encoding PNG: %w", err)
		}
		result.ContentType = "image/png"
	}
	result.Body = out.Bytes()
	return result, nil
}

func encodeWebP(picture image.Image) (resized, error) {
	var out bytes.Buffer
	if err := webp.Encode(&out, picture, webp.EncodeOptions{Quality: webpQuality, Method: webp.DefaultMethod}); err != nil {
		return resized{}, fmt.Errorf("encoding WebP: %w", err)
	}
	return resized{Body: out.Bytes(), ContentType: "image/webp", Width: picture.Bounds().Dx(), Height: picture.Bounds().Dy()}, nil
}

func isOpaque(picture image.Image) bool {
	if opaque, ok := picture.(interface{ Opaque() bool }); ok {
		return opaque.Opaque()
	}
	bounds := picture.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, alpha := picture.At(x, y).RGBA(); alpha != 0xffff {
				return false
			}
		}
	}
	return true
}
