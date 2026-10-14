// Package render holds the output formats a preview is written in.
package render

import (
	"io"

	"github.com/pantafive/embisto/card"
)

// Defaults are embisto's own formats: the card as JSON, as text and as HTML, and Iframely's two.
func Defaults() []card.Renderer {
	return []card.Renderer{JSON{}, Text{}, HTML{}, Iframely{}, OEmbed{}}
}

// JSON writes the preview as embisto's own card.
type JSON struct{}

func (JSON) Name() string {
	return formatJSON
}

func (JSON) ContentType() string {
	return contentTypeJSON
}

func (JSON) Render(w io.Writer, preview card.Preview) error {
	return encode(w, preview)
}
