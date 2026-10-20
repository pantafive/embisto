package api

import (
	"strings"
	"time"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/iframely"
	"github.com/pantafive/embisto/render"
)

const (
	exampleTitle  = "Rick Astley - Never Gonna Give You Up (Official Video) (4K Remaster)"
	exampleImage  = "https://i.ytimg.com/vi/dQw4w9WgXcQ/maxresdefault.jpg"
	examplePlayer = "https://www.youtube.com/embed/dQw4w9WgXcQ"
	exampleEmbed  = `<iframe width="560" height="315" src="` + examplePlayer + `" title="` + exampleTitle + `" frameborder="0" ` +
		`allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" ` +
		`referrerpolicy="strict-origin-when-cross-origin" allowfullscreen></iframe>`
	exampleIcon     = "https://www.youtube.com/s/desktop/eae06bf0/img/favicon_144x144.png"
	exampleProvider = "youtube" // sites.YouTube's name
)

// renderedText is the example in a text format.
func renderedText(renderer card.Renderer, preview card.Preview) string {
	var text strings.Builder
	if err := renderer.Render(&text, preview); err != nil {
		panic(err) // the example is part of the build
	}
	return text.String()
}

// examplePreview is the card the reference shows, as /v1/preview makes it for a YouTube video (the youtube fixture),
// without its keywords and with three of its links.
var examplePreview = card.Preview{
	URL:          "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
	CanonicalURL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
	Type:         card.TypeVideo,
	Title:        exampleTitle,
	Description: "The official video for “Never Gonna Give You Up” by Rick Astley. Never: The Autobiography 📚 OUT NOW! " +
		"Follow this link to get your copy and listen to Rick’s ...",
	SiteName:    "YouTube",
	Author:      "Rick Astley",
	AuthorURL:   "https://www.youtube.com/@RickAstleyYT",
	PublishedAt: time.Date(2009, 10, 25, 6, 57, 33, 0, time.UTC),
	Icon:        card.Image{URL: exampleIcon, Width: 144, Height: 144},
	Image:       card.Image{URL: exampleImage, Width: 1280, Height: 720},
	Images: []card.Image{
		{URL: exampleImage, Width: 1280, Height: 720},
		{URL: "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg", Width: 480, Height: 360},
	},
	Video: card.Video{URL: examplePlayer, Type: "text/html", Width: 560, Height: 315},
	Embed: card.Embed{
		HTML:  exampleEmbed,
		Width: 560, Height: 315, Provider: "YouTube",
	},
	Links: []card.Link{
		{
			Href: examplePlayer, Rel: []string{card.RelPlayer, exampleProvider, card.SourceOpenGraph, card.SourceTwitter},
			Type: "text/html", Media: card.MediaOf(560, 315),
			HTML: exampleEmbed,
		},
		{
			Href: exampleImage, Rel: []string{card.RelThumbnail, card.SourceOpenGraph, card.SourceJSONLD, card.SourceTwitter},
			Type: "image/jpeg", Media: card.MediaOf(1280, 720),
		},
		{
			Href: exampleIcon, Rel: []string{card.RelIcon, card.SourceHTML},
			Type: "image/png", Media: card.MediaOf(144, 144),
		},
	},
	Sources: []string{exampleProvider, card.SourceOEmbed, card.SourceOpenGraph, card.SourceJSONLD, card.SourceTwitter, card.SourceHTML},
}

var (
	exampleIframely = iframely.Of(examplePreview)
	exampleOEmbed   = render.OEmbedOf(examplePreview)
	exampleText     = renderedText(render.Text{}, examplePreview)
	exampleProblem  = refusal(card.CodeBlockedURL) //nolint:errname // an example body, not an error to return
	exampleRefusal  = iframely.Error{Error: iframely.ErrorBody{Source: "embisto", Code: 404, Message: "Not Found"}}
)
