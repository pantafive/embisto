package render

import (
	"fmt"
	"html"
	"math"
	"strconv"

	"github.com/pantafive/embisto/card"
)

// percent is how a ratio becomes the padding that keeps a responsive frame's proportions, kept to four places.
const (
	percent          = 100
	paddingPrecision = 10000
)

// frame is the HTML that embeds a player page: a frame filling the width at the player's proportions, or at its
// fixed height; empty for a player of unknown size, which no layout can reserve room for.
func frame(href string, media card.Media) string {
	src := html.EscapeString(href)
	const attributes = `allowfullscreen="" scrolling="no" allow="encrypted-media; accelerometer; clipboard-write; gyroscope; picture-in-picture; web-share; fullscreen"`
	switch {
	case media.AspectRatio > 0:
		ratio := 1 / media.AspectRatio
		if media.Width > 0 {
			ratio = float64(media.Height) / float64(media.Width)
		}
		padding := strconv.FormatFloat(math.Round(ratio*percent*paddingPrecision)/paddingPrecision, 'f', -1, 64)
		return fmt.Sprintf(`<div style="left: 0; width: 100%%; height: 0; position: relative; padding-bottom: %s%%;">`+
			`<iframe src="%s" style="border: 0; top: 0; left: 0; width: 100%%; height: 100%%; position: absolute;" %s></iframe></div>`,
			padding, src, attributes)
	case media.Height > 0:
		return fmt.Sprintf(`<iframe src="%s" style="border: 0; width: 100%%; height: %dpx;" %s></iframe>`, src, media.Height, attributes)
	default:
		return ""
	}
}
