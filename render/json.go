// Package render holds the output formats a preview is written in.
package render

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/iframely"
)

const contentTypeJSON = "application/json"

// Defaults are embisto's own formats: the card as JSON, as text and as HTML, and Iframely's two.
func Defaults() []card.Renderer {
	return []card.Renderer{JSON{}, Text{}, HTML{}, iframely.Renderer{}, OEmbed{}}
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

func encode(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	// Embeds are HTML by nature; escaping their brackets only makes them harder to read.
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("writing the response: %w", err)
	}
	return nil
}
