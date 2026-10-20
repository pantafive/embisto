package jpegheader_test

import (
	"bytes"
	"encoding/binary"
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
	assert.Equal(t, jpegheader.Header{Width: 40, Height: 30, Sampling: []image.Point{{2, 2}, {1, 1}, {1, 1}}, Orientation: 1}, header)
}

// withAPP1 is a 40×30 JPEG with an APP1 segment of the given payload after its start.
func withAPP1(t *testing.T, payload []byte) []byte {
	t.Helper()
	var file bytes.Buffer
	require.NoError(t, jpeg.Encode(&file, image.NewRGBA(image.Rect(0, 0, 40, 30)), nil))
	segment := append([]byte{0xff, 0xe1, 0, byte(2 + len(payload))}, payload...)
	return append(append(file.Bytes()[:2:2], segment...), file.Bytes()[2:]...)
}

func TestReadGivesTheOrientation(t *testing.T) {
	big, little := binary.AppendByteOrder(binary.BigEndian), binary.AppendByteOrder(binary.LittleEndian)
	// entry is a directory entry of one value: its tag, its type and, in the value's first bytes, a SHORT.
	entry := func(order binary.AppendByteOrder, tag, kind, value uint16) []byte {
		e := order.AppendUint16(order.AppendUint16(nil, tag), kind)
		return append(order.AppendUint16(order.AppendUint32(e, 1), value), 0, 0)
	}
	tiff := func(order binary.AppendByteOrder, entries ...[]byte) []byte {
		name := map[binary.AppendByteOrder]string{big: "MM", little: "II"}[order]
		file := order.AppendUint16(order.AppendUint32(order.AppendUint16([]byte("Exif\x00\x00"+name), 42), 8), uint16(len(entries)))
		for _, e := range entries {
			file = append(file, e...)
		}
		return file
	}
	tests := []struct {
		name    string
		payload []byte
		want    int
	}{
		{"big-endian", tiff(big, entry(big, 0x0112, 3, 6)), 6},
		{"little-endian, after another tag", tiff(little, entry(little, 0x010f, 3, 1), entry(little, 0x0112, 3, 8)), 8},
		{"not a SHORT", tiff(big, entry(big, 0x0112, 4, 6)), 1},
		{"out of range", tiff(big, entry(big, 0x0112, 3, 9)), 1},
		{"no orientation", tiff(big), 1},
		{"cut in the directory", tiff(big, entry(big, 0x0112, 3, 6))[:20], 1},
		{"XMP, not EXIF", []byte("http://ns.adobe.com/xap/1.0/\x00<x/>"), 1},
		{"unknown byte order", []byte("Exif\x00\x00XX\x00\x2a\x00\x00\x00\x08"), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header, ok := jpegheader.Read(withAPP1(t, tt.payload))

			require.True(t, ok)
			assert.Equal(t, []int{40, 30, tt.want}, []int{header.Width, header.Height, header.Orientation})
		})
	}
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
