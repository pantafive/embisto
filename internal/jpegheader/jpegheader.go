// Package jpegheader reads what a JPEG's markers state before its first scan, beyond the size image.DecodeConfig gives:
// how the frame is coded, and which way up the picture is shown.
package jpegheader

import (
	"bytes"
	"encoding/binary"
	"image"
)

// Header is what a JPEG states before its pixels.
type Header struct {
	Width, Height int
	// Progressive is a frame coded in successive scans, whose every coefficient a decoder keeps until the last.
	Progressive bool
	// Sampling is each component's horizontal and vertical sampling factor.
	Sampling []image.Point
	// Orientation is how the picture is turned to be shown, as EXIF numbers it: 1 as stored, 2 to 8 mirrored or
	// rotated, 5 to 8 with width and height swapped.
	Orientation int
}

// IsSwapped is an orientation that turns the picture a quarter, so it is shown its height wide.
func (h Header) IsSwapped() bool {
	return h.Orientation >= 5 && h.Orientation <= maxOrientation
}

const (
	markerSOF0 = 0xc0
	markerSOF1 = 0xc1
	markerSOF2 = 0xc2
	markerSOS  = 0xda
	markerAPP1 = 0xe1
	// A frame header: its length, the sample precision, the height, the width and the number of components; then
	// three bytes a component, of which the second holds the sampling factors.
	frameFixed     = 8
	frameComponent = 3
	// A TIFF header: the byte order, 42 and the offset of the first directory; a directory: its count, then twelve
	// bytes an entry: the tag, the type, the count and the value.
	tiffHeader = 8
	entryBytes = 12
	// tagOrientation holds a SHORT, the first two bytes of its value.
	tagOrientation = 0x0112
	typeShort      = 3
	maxOrientation = 8
)

var (
	soi        = []byte{0xff, 0xd8}
	exifPrefix = []byte("Exif\x00\x00")
)

// Read walks the markers up to the frame: false for a file that is not a JPEG, or ends before its frame.
func Read(file []byte) (Header, bool) {
	if !bytes.HasPrefix(file, soi) {
		return Header{}, false
	}
	orientation := 1
	for at := len(soi); at+4 <= len(file); {
		if file[at] != 0xff {
			return Header{}, false
		}
		marker := file[at+1]
		if marker == 0xff { // a fill byte before the marker
			at++
			continue
		}
		length := int(binary.BigEndian.Uint16(file[at+2:]))
		segment := file[at+2 : min(len(file), at+2+length)]
		switch marker {
		case markerSOF0, markerSOF1, markerSOF2:
			read, ok := frame(segment, marker == markerSOF2)
			read.Orientation = orientation
			return read, ok
		case markerAPP1:
			if exif, ok := bytes.CutPrefix(segment[min(len(segment), 2):], exifPrefix); ok && orientation == 1 {
				orientation = orientationOf(exif)
			}
		case markerSOS:
			return Header{}, false
		}
		at += 2 + length
	}
	return Header{}, false
}

// frame reads a frame header, the segment after its marker.
func frame(segment []byte, isProgressive bool) (Header, bool) {
	if len(segment) < frameFixed {
		return Header{}, false
	}
	count := int(segment[7])
	if len(segment) < frameFixed+frameComponent*count {
		return Header{}, false
	}
	read := Header{
		Height:      int(binary.BigEndian.Uint16(segment[3:])),
		Width:       int(binary.BigEndian.Uint16(segment[5:])),
		Progressive: isProgressive,
	}
	for i := range count {
		factors := segment[frameFixed+frameComponent*i+1]
		read.Sampling = append(read.Sampling, image.Pt(int(factors>>4), int(factors&0x0f)))
	}
	return read, true
}

// orientationOf reads the orientation from the first directory of EXIF data, a TIFF file; 1 when it states none.
func orientationOf(tiff []byte) int {
	order, ok := byteOrder(tiff)
	if !ok || len(tiff) < tiffHeader {
		return 1
	}
	directory := int(order.Uint32(tiff[4:]))
	if directory < tiffHeader || directory+2 > len(tiff) {
		return 1
	}
	// The entries the directory states, of those the data holds.
	count := min(int(order.Uint16(tiff[directory:])), (len(tiff)-directory-2)/entryBytes)
	for i := range count {
		entry := directory + 2 + i*entryBytes
		if order.Uint16(tiff[entry:]) == tagOrientation && order.Uint16(tiff[entry+2:]) == typeShort {
			if value := int(order.Uint16(tiff[entry+8:])); value >= 1 && value <= maxOrientation {
				return value
			}
			return 1
		}
	}
	return 1
}

// byteOrder is the order a TIFF file's first two bytes name.
func byteOrder(tiff []byte) (binary.ByteOrder, bool) {
	switch {
	case bytes.HasPrefix(tiff, []byte("II")):
		return binary.LittleEndian, true
	case bytes.HasPrefix(tiff, []byte("MM")):
		return binary.BigEndian, true
	default:
		return nil, false
	}
}
