package render

import (
	"io"
	"slices"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/iframely"
)

// OEmbed writes the preview as an oEmbed 1.0 answer with the fields Iframely adds: a description, and the author as
// author rather than author_name.
type OEmbed struct{}

// OEmbedResponse is an oEmbed answer: a photo carries its picture in url, a video or rich one its HTML.
type OEmbedResponse struct {
	Type            string `json:"type" enum:"link,photo,video,rich"`
	Version         string `json:"version"`
	Title           string `json:"title,omitempty"`
	URL             string `json:"url,omitempty"`
	Author          string `json:"author,omitempty"`
	AuthorURL       string `json:"author_url,omitempty"`
	ProviderName    string `json:"provider_name,omitempty"`
	Description     string `json:"description,omitempty"`
	ThumbnailURL    string `json:"thumbnail_url,omitempty"`
	ThumbnailWidth  int    `json:"thumbnail_width,omitempty"`
	ThumbnailHeight int    `json:"thumbnail_height,omitempty"`
	HTML            string `json:"html,omitempty"`
	Width           int    `json:"width,omitempty"`
	Height          int    `json:"height,omitempty"`
}

func (OEmbed) Name() string {
	return formatOEmbed
}

func (OEmbed) ContentType() string {
	return contentTypeJSON
}

func (OEmbed) Render(w io.Writer, preview card.Preview) error {
	return encode(w, OEmbedOf(preview))
}

func (OEmbed) RenderError(w io.Writer, failed *card.Error) (int, error) {
	return iframely.Renderer{}.RenderError(w, failed) //nolint:wrapcheck // the same failure in the same shape; its error is the writer's
}

// OEmbedOf is the preview as an oEmbed answer, its type from the primary embed.
func OEmbedOf(preview card.Preview) OEmbedResponse {
	response := OEmbedResponse{
		Type: string(card.TypeLink), Version: "1.0", Title: preview.Title, URL: preview.CanonicalURL,
		Author: preview.Author, AuthorURL: preview.AuthorURL, ProviderName: preview.SiteName, Description: preview.Description,
		ThumbnailURL: preview.Image.URL, ThumbnailWidth: preview.Image.Width, ThumbnailHeight: preview.Image.Height,
	}
	primary := iframely.Of(preview)
	switch {
	case preview.Type == card.TypePhoto:
		response.Type, response.URL = string(card.TypePhoto), preview.Image.URL
		response.Width, response.Height = preview.Image.Width, preview.Image.Height
		response.ThumbnailURL, response.ThumbnailWidth, response.ThumbnailHeight = "", 0, 0
	case primary.HTML != "" && slices.Contains(primary.Rel, card.RelPlayer) && preview.Type == card.TypeVideo:
		response.Type, response.HTML = string(card.TypeVideo), primary.HTML
	case primary.HTML != "":
		response.Type, response.HTML = string(card.TypeRich), primary.HTML
		if players := primary.Links[card.RelPlayer]; len(players) > 0 && players[0].Media.AspectRatio == 0 {
			response.Height = players[0].Media.Height
		}
	}
	return response
}
