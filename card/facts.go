package card

import (
	"reflect"
	"time"
)

// Names of the built-in generic sources.
const (
	SourceOEmbed    = "oembed"
	SourceOpenGraph = "opengraph"
	SourceTwitter   = "twitter"
	SourceHTML      = "html"
	SourceJSONLD    = "jsonld"
)

// Facts are what one source found about a URL; URLs in them are absolute.
type Facts struct {
	// Source names the source in a preview's sources.
	Source string
	// Type is what the source says the page is; empty when it says nothing.
	Type         Type
	Title        string
	Description  string
	SiteName     string
	Author       string
	AuthorURL    string
	Keywords     string
	CanonicalURL string
	PublishedAt  time.Time
	Icons        []Image
	Images       []Image
	Videos       []Video
	Embed        Embed
	Duration     int
	// Limited is a wall the source met instead of the item.
	Limited Limit
	// Boilerplate are texts and pictures the source knows to be the site's own, said of every page rather than of
	// this item: a site-wide description, a default image. No source's value equal to one of them is used.
	Boilerplate Boilerplate
	// Links are resources beyond the ones the fields above describe, such as a download.
	Links []Link
}

// IsEmpty says the source found nothing.
func (f Facts) IsEmpty() bool {
	f.Source, f.Boilerplate = "", Boilerplate{}
	return reflect.ValueOf(f).IsZero()
}

// Boilerplate is what a site says of every page.
type Boilerplate struct {
	Texts  []string `json:"texts,omitempty"`
	Images []string `json:"images,omitempty"`
}
