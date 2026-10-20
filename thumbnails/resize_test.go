package thumbnails_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gen2brain/vpx/webp"

	"github.com/pantafive/embisto/thumbnails"
)

func pngOf(t *testing.T, width, height int, fill color.Color) []byte {
	t.Helper()
	picture := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			picture.Set(x, y, fill)
		}
	}
	var out bytes.Buffer
	require.NoError(t, png.Encode(&out, picture))
	return out.Bytes()
}

// claiming rewrites a PNG's header to state another size, with the checksum the header needs to stay valid; the
// pixels that follow are never read.
func claiming(t *testing.T, file []byte, width, height uint32) []byte {
	t.Helper()
	patched := bytes.Clone(file)
	const ihdr = 8 // the signature; the IHDR chunk follows: length, type, data, CRC
	binary.BigEndian.PutUint32(patched[ihdr+8:], width)
	binary.BigEndian.PutUint32(patched[ihdr+12:], height)
	binary.BigEndian.PutUint32(patched[ihdr+8+13:], crc32.ChecksumIEEE(patched[ihdr+4:ihdr+8+13]))
	return patched
}

func TestResizeRefusesAPictureOverThePixelLimitFromItsHeader(t *testing.T) {
	file := claiming(t, pngOf(t, 4, 4, color.White), 100000, 100000)

	_, err := thumbnails.Resize(file, thumbnails.Box{Width: 160, Mode: thumbnails.Fit}, 1_000_000, thumbnails.Classic)

	require.ErrorIs(t, err, thumbnails.ErrTooManyPixels)
}

func TestResizeKeepsTransparencyInPNG(t *testing.T) {
	file := pngOf(t, 400, 200, color.NRGBA{R: 255, A: 128})

	result, err := thumbnails.Resize(file, thumbnails.Box{Width: 160, Mode: thumbnails.Fit}, 1_000_000, thumbnails.Classic)

	require.NoError(t, err)
	assert.Equal(t, []any{"image/png", 160, 80}, []any{result.ContentType, result.Width, result.Height})
}

func TestResizeWritesAnOpaquePictureAsJPEG(t *testing.T) {
	file := pngOf(t, 400, 200, color.NRGBA{G: 255, A: 255})

	result, err := thumbnails.Resize(file, thumbnails.Box{Width: 160, Height: 160, Mode: thumbnails.Fill}, 1_000_000, thumbnails.Classic)

	require.NoError(t, err)
	assert.Equal(t, []any{"image/jpeg", 160, 160}, []any{result.ContentType, result.Width, result.Height})
}

func TestResizeRefusesWhatIsNotARasterPicture(t *testing.T) {
	_, err := thumbnails.Resize([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>`), thumbnails.Box{Width: 160, Mode: thumbnails.Fit}, 1_000_000, thumbnails.Classic)

	require.ErrorIs(t, err, thumbnails.ErrUnsupported)
}

func TestScaled(t *testing.T) {
	tests := []struct {
		name                  string
		width, height         int
		box                   thumbnails.Box
		wantWidth, wantHeight int
	}{
		{"fit by width", 1280, 720, thumbnails.Box{Width: 600, Mode: thumbnails.Fit}, 600, 338},
		{"fit inside both", 1280, 720, thumbnails.Box{Width: 600, Height: 200, Mode: thumbnails.Fit}, 356, 200},
		{"fit never enlarges", 100, 50, thumbnails.Box{Width: 600, Mode: thumbnails.Fit}, 100, 50},
		{"fill takes the box", 1280, 720, thumbnails.Box{Width: 160, Height: 160, Mode: thumbnails.Fill}, 160, 160},
		{"fill never enlarges", 100, 50, thumbnails.Box{Width: 160, Height: 160, Mode: thumbnails.Fill}, 100, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width, height := thumbnails.Scaled(tt.width, tt.height, tt.box)

			assert.Equal(t, []int{tt.wantWidth, tt.wantHeight}, []int{width, height})
		})
	}
}

func TestResizeWritesWebPKeepingTransparency(t *testing.T) {
	file := pngOf(t, 400, 200, color.NRGBA{R: 255, A: 128})

	result, err := thumbnails.Resize(file, thumbnails.Box{Width: 160, Mode: thumbnails.Fit}, 1_000_000, thumbnails.WebP)

	require.NoError(t, err)
	decoded, err := webp.Decode(bytes.NewReader(result.Body))
	require.NoError(t, err)
	alpha := color.NRGBAModel.Convert(decoded.At(80, 40)).(color.NRGBA).A
	assert.Equal(t, []any{"image/webp", 160, 80, true}, []any{result.ContentType, result.Width, result.Height, alpha > 112 && alpha < 144})
}

// quadrants is a 400×200 JPEG in four colours, red, green, blue and white from the top left, stating the EXIF
// orientation given.
func quadrants(t *testing.T, orientation byte) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 400, 200))
	colours := []color.RGBA{{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255}, {R: 255, G: 255, B: 255, A: 255}}
	for y := range 200 {
		for x := range 400 {
			picture.SetRGBA(x, y, colours[y/100*2+x/200])
		}
	}
	var file bytes.Buffer
	require.NoError(t, jpeg.Encode(&file, picture, &jpeg.Options{Quality: 95}))
	exif := []byte{
		0xff, 0xe1, 0, 34, 'E', 'x', 'i', 'f', 0, 0,
		'M', 'M', 0, 42, 0, 0, 0, 8, // the TIFF header; the first directory follows
		0, 1, 0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, orientation, 0, 0, // one entry: the orientation, a SHORT
		0, 0, 0, 0, // no next directory
	}
	return append(append(file.Bytes()[:2:2], exif...), file.Bytes()[2:]...)
}

// named is which of red, green, blue and white a pixel is.
func named(c color.Color) string {
	r, g, b, _ := c.RGBA()
	switch {
	case r > 0xc000 && g > 0xc000 && b > 0xc000:
		return "white"
	case r > 0xc000:
		return "red"
	case g > 0xc000:
		return "green"
	case b > 0xc000:
		return "blue"
	default:
		return "unclear"
	}
}

func TestResizeTurnsAJPEGAsItsOrientationSays(t *testing.T) {
	tests := []struct {
		orientation byte
		size        image.Point
		// shown are the colours of the result's quadrants, from the top left, row by row.
		shown []string
	}{
		{1, image.Pt(160, 80), []string{"red", "green", "blue", "white"}},
		{2, image.Pt(160, 80), []string{"green", "red", "white", "blue"}},
		{3, image.Pt(160, 80), []string{"white", "blue", "green", "red"}},
		{4, image.Pt(160, 80), []string{"blue", "white", "red", "green"}},
		{5, image.Pt(160, 320), []string{"red", "blue", "green", "white"}},
		{6, image.Pt(160, 320), []string{"blue", "red", "white", "green"}},
		{7, image.Pt(160, 320), []string{"white", "green", "blue", "red"}},
		{8, image.Pt(160, 320), []string{"green", "white", "red", "blue"}},
	}
	for _, tt := range tests {
		t.Run(string('0'+tt.orientation), func(t *testing.T) {
			result, err := thumbnails.Resize(quadrants(t, tt.orientation), thumbnails.Box{Width: 160, Mode: thumbnails.Fit}, 1_000_000, thumbnails.Classic)

			require.NoError(t, err)
			decoded, err := jpeg.Decode(bytes.NewReader(result.Body))
			require.NoError(t, err)
			width, height := tt.size.X, tt.size.Y
			var shown []string
			for _, at := range []image.Point{{width / 4, height / 4}, {3 * width / 4, height / 4}, {width / 4, 3 * height / 4}, {3 * width / 4, 3 * height / 4}} {
				shown = append(shown, named(decoded.At(at.X, at.Y)))
			}
			assert.Equal(t, []any{tt.size, tt.shown}, []any{decoded.Bounds().Size(), shown})
		})
	}
}

func TestCostChargesATurnedPictureInItsStoredTerms(t *testing.T) {
	box := thumbnails.Box{Width: 160, Mode: thumbnails.Fit}
	upright, err := thumbnails.Cost(quadrants(t, 1), box, 1_000_000)
	require.NoError(t, err)
	turned, err := thumbnails.Cost(quadrants(t, 6), box, 1_000_000)
	require.NoError(t, err)

	// Upright: a 320×160 intermediate, CatmullRom's 160 by 160 and a 160×80 output. Turned to 160×320: no
	// intermediate, CatmullRom's 320 by the stored 200, a 320×160 output and its turned copy.
	scaling := int64(4*320*160 + 32*160*160 + 20*160*80)
	turnedScaling := int64(32*320*200 + 20*320*160 + 4*320*160)
	assert.Equal(t, turnedScaling-scaling, turned-upright)
}
