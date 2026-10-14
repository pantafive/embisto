// Package iframely answers in the formats of Iframely's API, so a client of Iframely can switch to embisto by changing
// the address: the response with links grouped by rel, and the shape of a failure.
package iframely

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

// Format is the name clients ask for the Renderer's format by.
const Format = "iframely"

// Renderer writes the preview the way Iframely's API does, links grouped by rel, so a client of Iframely can switch by
// changing the address. docs/iframely-compat.md lists where the two differ.
type Renderer struct{}

// Response is Iframely's answer: the page's meta, its links by rel, and the primary embed.
type Response struct {
	URL   string            `json:"url,omitempty" doc:"The canonical URL of the page"`
	Meta  Meta              `json:"meta"`
	Links map[string][]Link `json:"links" doc:"Links by their primary rel: player, app, image, file, thumbnail, icon"`
	Rel   []string          `json:"rel" doc:"The rels of the primary embed; empty when the page has none"`
	HTML  string            `json:"html,omitempty" doc:"The primary embed's HTML"`
}

// Meta is what the page says about itself, in Iframely's names.
type Meta struct {
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

// Link is one resource with its rels, its MIME type, its size and, for a player or an app, its HTML.
type Link struct {
	Href  string   `json:"href,omitempty"`
	Rel   []string `json:"rel"`
	Type  string   `json:"type,omitempty"`
	Media Media    `json:"media,omitzero"`
	HTML  string   `json:"html,omitempty"`
}

// Media is a link's size, or for a responsive player its proportions.
type Media struct {
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	AspectRatio float64 `json:"aspect-ratio,omitempty"` //nolint:tagliatelle // Iframely's own name
	MaxWidth    int     `json:"max-width,omitempty"`    //nolint:tagliatelle // Iframely's own name
}

// Error is Iframely's shape of a failure.
type Error struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody names the failure by its HTTP status, in the catalogue's words.
type ErrorBody struct {
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

func (Renderer) Name() string {
	return Format
}

func (Renderer) ContentType() string {
	return contentTypeJSON
}

func (Renderer) Render(w io.Writer, preview card.Preview) error {
	return encode(w, Of(preview))
}

func (Renderer) RenderError(w io.Writer, failed *card.Error) (int, error) {
	status := statusOf(failed)
	return status, encode(w, Error{Error: ErrorBody{Source: "embisto", Code: status, Message: failed.Error()}})
}

// Of is the preview in Iframely's shape.
func Of(preview card.Preview) Response {
	response := Response{
		URL:   preview.CanonicalURL,
		Meta:  metaOf(preview),
		Links: map[string][]Link{},
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

func metaOf(preview card.Preview) Meta {
	meta := Meta{
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

func linkOf(link card.Link) Link {
	converted := Link{Href: link.Href, Type: link.Type, Rel: relsOf(link)}
	isPlayer := slices.Contains(link.Rel, card.RelPlayer)
	switch {
	case isPlayer && link.Type == "text/html":
		converted.Rel = append(converted.Rel, "html5")
		converted.Media = Media{AspectRatio: link.Media.AspectRatio}
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
		converted.Media = Media{MaxWidth: link.Media.Width}
	default:
		converted.Media = Media{Width: link.Media.Width, Height: link.Media.Height}
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

// statuses are the failures Iframely's clients know by another status than the catalogue's: a page too slow
// is 408, and one that cannot be fetched is one Iframely cannot find.
var statuses = map[card.Code]int{
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
	if status, ok := statuses[failed.Code]; ok {
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
