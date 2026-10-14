// Package card is the contract between embisto and its modules: the preview a client draws, the parsed page, the
// facts a source finds on it, and the interfaces of every stage of the pipeline. It imports nothing from the module,
// so a site provider or a renderer written outside it needs this package alone.
package card

import (
	"math"
	"time"
)

// Type is what a preview shows.
type Type string

// The types a preview can be.
const (
	TypeLink  Type = "link"
	TypeVideo Type = "video"
	TypePhoto Type = "photo"
	TypeRich  Type = "rich"
)

// Image is a picture with its size in pixels, when known.
type Image struct {
	URL    string `json:"url"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	// Variants are the picture resized and served by embisto at /i/, when the server has image variants on.
	Variants []Variant `json:"variants,omitempty"`
}

// Variant is a picture resized to a preset and served by embisto; its size is known when the original's is.
type Variant struct {
	Preset string `json:"preset" doc:"The preset's name, such as thumb (160x160, cropped), card (600 wide) or large (1200 wide)"`
	URL    string `json:"url" doc:"The picture as JPEG, or PNG when it has transparency"`
	// WebPURL is the same picture as WebP, about half the bytes, for a client that decodes WebP.
	WebPURL string `json:"webp_url,omitempty" doc:"The same picture as WebP, about half the bytes"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
}

// Video is a playable resource: a file with its media type, or a player page with text/html.
type Video struct {
	URL    string `json:"url"`
	Type   string `json:"type,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// Embed is the HTML a provider offers for showing the content inline.
type Embed struct {
	HTML     string `json:"html"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Provider string `json:"provider,omitempty"`
}

// Link is a resource a preview can show: a picture, a player, an icon. Rel says what it is to the page and which
// sources named it.
type Link struct {
	Href  string   `json:"href"`
	Rel   []string `json:"rel"`
	Type  string   `json:"type,omitempty"`
	Media Media    `json:"media,omitzero"`
	// HTML embeds the resource, for a player or an app.
	HTML string `json:"html,omitempty"`
}

// Media is a resource's size in pixels, when known.
type Media struct {
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	AspectRatio float64 `json:"aspect_ratio,omitempty"`
}

// Limit is why a card has little more than its URL.
type Limit string

const (
	// LimitLoginWall is a site that sent the request to a login page.
	LimitLoginWall Limit = "login_wall"
	// LimitConsentWall is a site that sent the request to a cookie or consent page.
	LimitConsentWall Limit = "consent_wall"
	// LimitBlocked is a site that answered with a page about itself, not the item: a block or a wall without a
	// telltale address.
	LimitBlocked Limit = "blocked"
)

// MediaOf is a size with its aspect ratio, rounded to four places; zero for an unknown size.
func MediaOf(width, height int) Media {
	if width <= 0 || height <= 0 {
		return Media{}
	}
	return Media{Width: width, Height: height, AspectRatio: math.Round(float64(width)/float64(height)*aspectPrecision) / aspectPrecision}
}

const aspectPrecision = 10000

// The relations a link has to the page, besides the names of the sources that found it.
const (
	RelIcon      = "icon"
	RelThumbnail = "thumbnail"
	RelImage     = "image"
	RelPlayer    = "player"
	RelApp       = "app"
	RelFile      = "file"
)

// Preview is the card for one URL.
type Preview struct {
	URL          string    `json:"url"`
	CanonicalURL string    `json:"canonical_url,omitempty"`
	Type         Type      `json:"type" enum:"link,video,photo,rich"`
	Title        string    `json:"title,omitempty"`
	Description  string    `json:"description,omitempty"`
	SiteName     string    `json:"site_name,omitempty"`
	Author       string    `json:"author,omitempty"`
	AuthorURL    string    `json:"author_url,omitempty"`
	Keywords     string    `json:"keywords,omitempty"`
	PublishedAt  time.Time `json:"published_at,omitzero"`
	Icon         Image     `json:"icon,omitzero"`
	Image        Image     `json:"image,omitzero"`
	Images       []Image   `json:"images,omitempty"`
	Video        Video     `json:"video,omitzero"`
	Embed        Embed     `json:"embed,omitzero"`
	// Duration is how long the video or audio lasts, in seconds, when the provider says.
	Duration int `json:"duration,omitempty" doc:"How long the video or audio lasts, in seconds, when the provider says"`
	// Limited says why the card is all but empty: the site showed a login or consent wall, or refused the request,
	// instead of the item. Such a card is honest about it rather than showing the wall's own text.
	Limited Limit `json:"limited,omitempty" enum:"login_wall,consent_wall,blocked" doc:"Why the card has little more than its URL: the site answered with a login wall, a consent wall, or a page about itself instead of the item. Such a card carries the site's name and icon, never the wall's text."`
	// Links are every resource the sources found, each with its relations to the page.
	Links []Link `json:"links,omitempty"`
	// Sources name where the fields came from, highest priority first.
	Sources []string `json:"sources,omitempty"`
}
