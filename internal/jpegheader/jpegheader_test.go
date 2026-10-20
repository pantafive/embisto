package jpegheader_test

import (
	"bytes"
	"image"
	"image/jpeg"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/internal/jpegheader"
)

func TestReadGivesTheFrame(t *testing.T) {
	var file bytes.Buffer
	require.NoError(t, jpeg.Encode(&file, image.NewRGBA(image.Rect(0, 0, 40, 30)), nil))

	header, ok := jpegheader.Read(file.Bytes())

	require.True(t, ok)
	assert.Equal(t, jpegheader.Header{Width: 40, Height: 30, Sampling: []image.Point{{2, 2}, {1, 1}, {1, 1}}}, header)
}

func TestReadRefusesWhatEndsBeforeTheFrame(t *testing.T) {
	var file bytes.Buffer
	require.NoError(t, jpeg.Encode(&file, image.NewRGBA(image.Rect(0, 0, 40, 30)), nil))
	frameAt := bytes.Index(file.Bytes(), []byte{0xff, 0xc0})
	require.Positive(t, frameAt)
	tests := map[string][]byte{
		"empty":            nil,
		"not a JPEG":       []byte("GIF89a"),
		"cut in a segment": file.Bytes()[:frameAt-3],
		"cut in the frame": file.Bytes()[:frameAt+12],
	}
	for name, file := range tests {
		t.Run(name, func(t *testing.T) {
			_, ok := jpegheader.Read(file)

			assert.False(t, ok)
		})
	}
}
