package api

import (
	"strings"
	"time"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/render"
)

const (
	exampleImage  = "https://i.ytimg.com/vi/dQw4w9WgXcQ/maxresdefault.jpg"
	examplePlayer = "https://www.youtube.com/embed/dQw4w9WgXcQ?feature=oembed"
	exampleEmbed  = `<iframe width="200" height="113" src="` + examplePlayer + `" allowfullscreen></iframe>`
	exampleIcon   = "https://www.youtube.com/s/desktop/eae06bf0/img/favicon_144x144.png"
)

// renderedText is the example in a text format.
func renderedText(renderer card.Renderer, preview card.Preview) string {
	var text strings.Builder
	if err := renderer.Render(&text, preview); err != nil {
		panic(err) // the example is part of the build
	}
	return text.String()
}

// examplePreview is the card the reference shows, as /v1/preview made it for a YouTube video.
var examplePreview = card.Preview{
	URL:          "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
	CanonicalURL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
	Type:         card.TypeVideo,
	Title:        "Rick Astley - Never Gonna Give You Up (Official Video) (4K Remaster)",
	Description:  "The official video for “Never Gonna Give You Up” by Rick Astley.",
	SiteName:     "YouTube",
	Author:       "Rick Astley",
	AuthorURL:    "https://www.youtube.com/@RickAstleyYT",
	PublishedAt:  time.Date(2009, 10, 25, 6, 57, 33, 0, time.UTC),
	Icon:         card.Image{URL: exampleIcon, Width: 144, Height: 144},
	Image:        card.Image{URL: exampleImage, Width: 1280, Height: 720},
	Images:       []card.Image{{URL: exampleImage, Width: 1280, Height: 720}},
	Video:        card.Video{URL: "https://www.youtube.com/embed/dQw4w9WgXcQ", Type: "text/html", Width: 1280, Height: 720},
	Embed: card.Embed{
		HTML:  exampleEmbed,
		Width: 200, Height: 113, Provider: "YouTube",
	},
	Links: []card.Link{
		{
			Href: examplePlayer, Rel: []string{card.RelPlayer, card.SourceOEmbed},
			Type: "text/html", Media: card.MediaOf(200, 113),
			HTML: exampleEmbed,
		},
		{
			Href: exampleImage, Rel: []string{card.RelThumbnail, card.SourceOpenGraph},
			Type: "image/jpeg", Media: card.MediaOf(1280, 720),
		},
		{
			Href: exampleIcon, Rel: []string{card.RelIcon, card.SourceHTML},
			Type: "image/png", Media: card.MediaOf(144, 144),
		},
	},
	Sources: []string{card.SourceOEmbed, card.SourceOpenGraph, card.SourceHTML},
}

var (
	exampleIframely = render.IframelyOf(examplePreview)
	exampleOEmbed   = render.OEmbedOf(examplePreview)
	exampleText     = renderedText(render.Text{}, examplePreview)
	exampleProblem  = refusal(card.CodeBlockedURL) //nolint:errname // an example body, not an error to return
	exampleRefusal  = render.IframelyError{Error: render.IframelyErrorBody{Source: "embisto", Code: 404, Message: "Not Found"}}
)
