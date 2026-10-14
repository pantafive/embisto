package thumbnails_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
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
