// Package jpegheader reads what a JPEG's markers state before its first scan, beyond the size image.DecodeConfig gives.
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
}

const (
	markerSOF0 = 0xc0
	markerSOF1 = 0xc1
	markerSOF2 = 0xc2
	markerSOS  = 0xda
	// A frame header: its length, the sample precision, the height, the width and the number of components; then
	// three bytes a component, of which the second holds the sampling factors.
	frameFixed     = 8
	frameComponent = 3
)

var soi = []byte{0xff, 0xd8}

// Read walks the markers up to the frame: false for a file that is not a JPEG, or ends before its frame.
func Read(file []byte) (Header, bool) {
	if !bytes.HasPrefix(file, soi) {
		return Header{}, false
	}
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
			return frame(segment, marker == markerSOF2)
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
