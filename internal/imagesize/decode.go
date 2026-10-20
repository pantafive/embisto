package imagesize

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"image"

	// The decoders image.DecodeConfig reads headers with.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strconv"
	"strings"

	// The webp header decoder, which the standard library lacks.
	_ "golang.org/x/image/webp"

	"github.com/pantafive/embisto/internal/jpegheader"
)

const (
	icoHeader = 6
	icoEntry  = 16
	// icoFull is how an icon entry writes a side of 256 pixels in its one byte.
	icoFull = 256
	// svgSniff is how far into a file an <svg> root is looked for.
	svgSniff = 1024
)

var icoMagic = []byte{0, 0, 1, 0}

// size reads a picture's width and height as it is shown from the first bytes of its file, so a JPEG its EXIF
// orientation turns a quarter has them swapped; false when they say nothing readable.
func size(head []byte) (int, int, bool) {
	switch {
	case bytes.HasPrefix(head, icoMagic):
		return icoSize(head)
	case bytes.Contains(head[:min(len(head), svgSniff)], []byte("<svg")):
		return svgSize(head)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(head))
	if err != nil {
		return 0, 0, false
	}
	if header, ok := jpegheader.Read(head); ok && header.IsSwapped() {
		config.Width, config.Height = config.Height, config.Width
	}
	return config.Width, config.Height, config.Width > 0 && config.Height > 0
}

// icoSize is the size of an icon file's largest picture.
func icoSize(head []byte) (int, int, bool) {
	if len(head) < icoHeader {
		return 0, 0, false
	}
	count := int(binary.LittleEndian.Uint16(head[4:6]))
	width, height := 0, 0
	for i := range count {
		at := icoHeader + i*icoEntry
		if at+2 > len(head) {
			break
		}
		w, h := int(head[at]), int(head[at+1])
		if w == 0 {
			w = icoFull
		}
		if h == 0 {
			h = icoFull
		}
		if w > width {
			width, height = w, h
		}
	}
	return width, height, width > 0
}

// svgSize reads the root's width and height in pixels, or its viewBox when they are missing or relative.
func svgSize(head []byte) (int, int, bool) {
	decoder := xml.NewDecoder(bytes.NewReader(head))
	decoder.Strict = false
	for {
		token, err := decoder.Token()
		if err != nil {
			return 0, 0, false
		}
		if root, ok := token.(xml.StartElement); ok && root.Name.Local == "svg" {
			return rootSize(root)
		}
	}
}

func rootSize(root xml.StartElement) (int, int, bool) {
	values := map[string]string{}
	for _, a := range root.Attr {
		values[a.Name.Local] = a.Value
	}
	if w, h := pixels(values["width"]), pixels(values["height"]); w > 0 && h > 0 {
		return w, h, true
	}
	box := strings.Fields(strings.ReplaceAll(values["viewBox"], ",", " "))
	if len(box) != 4 {
		return 0, 0, false
	}
	w, h := pixels(box[2]), pixels(box[3])
	return w, h, w > 0 && h > 0
}

func pixels(value string) int {
	n, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), "px"), 64)
	if err != nil || n <= 0 {
		return 0
	}
	return int(n + 0.5)
}
