package render

import (
	"cmp"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/pantafive/embisto/card"
)

// Iframely writes the preview the way Iframely's API does, links grouped by rel, so a client of Iframely can switch by
// changing the address. docs/iframely-compat.md lists where the two differ.
type Iframely struct{}

// IframelyResponse is Iframely's answer: the page's meta, its links by rel, and the primary embed.
type IframelyResponse struct {
	URL   string                    `json:"url,omitempty" doc:"The canonical URL of the page"`
	Meta  IframelyMeta              `json:"meta"`
	Links map[string][]IframelyLink `json:"links" doc:"Links by their primary rel: player, app, image, file, thumbnail, icon"`
	Rel   []string                  `json:"rel" doc:"The rels of the primary embed; empty when the page has none"`
	HTML  string                    `json:"html,omitempty" doc:"The primary embed's HTML"`
}

// IframelyMeta is what the page says about itself, in Iframely's names.
type IframelyMeta struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Author      string `json:"author,omitempty"`
	AuthorURL   string `json:"author_url,omitempty"`
	Site        string `json:"site,omitempty"`
	Canonical   string `json:"canonical,omitempty"`
	Date        string `json:"date,omitempty" doc:"Publication date, ISO 8601 in UTC"`
	Keywords    string `json:"keywords,omitempty"`
	Medium      string `json:"medium,omitempty" enum:"link,video,image,file"`
}

// IframelyLink is one resource with its rels, its MIME type, its size and, for a player or an app, its HTML.
type IframelyLink struct {
	Href  string        `json:"href,omitempty"`
	Rel   []string      `json:"rel"`
	Type  string        `json:"type,omitempty"`
	Media IframelyMedia `json:"media,omitzero"`
	HTML  string        `json:"html,omitempty"`
}

// IframelyMedia is a link's size, or for a responsive player its proportions.
type IframelyMedia struct {
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	AspectRatio float64 `json:"aspect-ratio,omitempty"` //nolint:tagliatelle // Iframely's own name
	MaxWidth    int     `json:"max-width,omitempty"`    //nolint:tagliatelle // Iframely's own name
}

// IframelyError is Iframely's shape of a failure.
type IframelyError struct {
	Error IframelyErrorBody `json:"error"`
}

// IframelyErrorBody names the failure by its HTTP status, in the catalogue's words.
type IframelyErrorBody struct {
	Source  string `json:"source"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// functionalRels are the rels a link is grouped by, the one that makes a primary embed first.
var functionalRels = []string{card.RelPlayer, card.RelApp, card.RelImage, card.RelFile, card.RelThumbnail, card.RelIcon}

// sourceRels are Iframely's names for the sources it shares with embisto; the others leave no rel.
var sourceRels = map[string]string{card.SourceOpenGraph: "og", card.SourceTwitter: "twitter", card.SourceOEmbed: "oembed"}

const (
	isoMillis       = "2006-01-02T15:04:05.000Z07:00"
	contentTypeJSON = "application/json"
)

func (Iframely) Name() string {
	return formatIframely
}

func (Iframely) ContentType() string {
	return contentTypeJSON
}

func (Iframely) Render(w io.Writer, preview card.Preview) error {
	return encode(w, IframelyOf(preview))
}

func (Iframely) RenderError(w io.Writer, failed *card.Error) (int, error) {
	status := statusOf(failed)
	return status, encode(w, IframelyError{Error: IframelyErrorBody{Source: "embisto", Code: status, Message: failed.Error()}})
}

// IframelyOf is the preview in Iframely's shape.
func IframelyOf(preview card.Preview) IframelyResponse {
	response := IframelyResponse{
		URL:   preview.CanonicalURL,
		Meta:  metaOf(preview),
		Links: map[string][]IframelyLink{},
		Rel:   []string{},
	}
	for _, link := range preview.Links {
		converted := linkOf(link)
		for _, rel := range functionalRels {
			if slices.Contains(link.Rel, rel) {
				response.Links[rel] = append(response.Links[rel], converted)
			}
		}
		if isPrimary(link) && response.HTML == "" {
			response.Rel, response.HTML = converted.Rel, cmp.Or(converted.HTML, picture(link.Href))
		}
	}
	return response
}

// isPrimary is the embed a client shows in place of the card: the provider's own HTML, or the file the URL serves.
func isPrimary(link card.Link) bool {
	return link.HTML != "" || slices.Contains(link.Rel, card.RelImage) && slices.Contains(link.Rel, card.RelFile)
}

// picture is the HTML of an image the URL serves.
func picture(href string) string {
	return `<img src="` + html.EscapeString(href) + `">`
}

func metaOf(preview card.Preview) IframelyMeta {
	meta := IframelyMeta{
		Title: preview.Title, Description: preview.Description, Author: preview.Author, AuthorURL: preview.AuthorURL,
		Site: preview.SiteName, Canonical: preview.CanonicalURL, Keywords: preview.Keywords, Medium: mediumOf(preview),
	}
	if !preview.PublishedAt.IsZero() {
		meta.Date = preview.PublishedAt.UTC().Format(isoMillis)
	}
	return meta
}

func mediumOf(preview card.Preview) string {
	switch preview.Type {
	case card.TypeVideo:
		return "video"
	case card.TypePhoto:
		return "image"
	case card.TypeRich:
		return ""
	case card.TypeLink:
	}
	if len(preview.Links) == 1 && slices.Equal(preview.Links[0].Rel, []string{card.RelFile}) {
		return "file"
	}
	return "link"
}

func linkOf(link card.Link) IframelyLink {
	converted := IframelyLink{Href: link.Href, Type: link.Type, Rel: relsOf(link)}
	isPlayer := slices.Contains(link.Rel, card.RelPlayer)
	switch {
	case isPlayer && link.Type == "text/html":
		converted.Rel = append(converted.Rel, "html5")
		converted.Media = IframelyMedia{AspectRatio: link.Media.AspectRatio}
		// The frame takes its proportions from the exact size; the media keeps the rounded ratio.
		if link.Media.AspectRatio == 0 {
			converted.Media.Height = link.Media.Height
		} else {
			converted.Rel = append(converted.Rel, "responsive")
		}
		converted.HTML = frame(link.Href, link.Media)
	case slices.Contains(link.Rel, card.RelApp):
		// An app is its HTML; its href would only repeat the page's address.
		converted.Href = ""
		converted.Rel = append(converted.Rel, "html5")
		converted.HTML = link.HTML
		converted.Media = IframelyMedia{MaxWidth: link.Media.Width}
	default:
		converted.Media = IframelyMedia{Width: link.Media.Width, Height: link.Media.Height}
		if converted.Type == "" && !isPlayer {
			converted.Type = "image"
		}
	}
	return converted
}

// relsOf are a link's rels in Iframely's names: its roles, the sources Iframely also knows, and ssl.
func relsOf(link card.Link) []string {
	var rels []string
	for _, rel := range link.Rel {
		if slices.Contains(functionalRels, rel) {
			rels = append(rels, rel)
		} else if renamed, ok := sourceRels[rel]; ok {
			rels = append(rels, renamed)
		}
	}
	if strings.HasPrefix(link.Href, "https://") {
		rels = append(rels, "ssl")
	}
	return rels
}

// iframelyStatuses are the failures Iframely's clients know by another status than the catalogue's: a page too slow
// is 408, and one that cannot be fetched is one Iframely cannot find.
var iframelyStatuses = map[card.Code]int{
	card.CodeTimeout:     http.StatusRequestTimeout,
	card.CodeTooLarge:    http.StatusRequestEntityTooLarge,
	card.CodeFetchFailed: http.StatusNotFound,
}

// statusOf is the HTTP status of a failure in Iframely's terms: the page's own error status when it answered with
// one, else the catalogue's as Iframely's clients know it.
func statusOf(failed *card.Error) int {
	if failed.Status >= http.StatusBadRequest {
		return failed.Status
	}
	if status, ok := iframelyStatuses[failed.Code]; ok {
		return status
	}
	return failed.HTTPStatus()
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
