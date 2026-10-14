package card

import (
	"net/url"
	"slices"
	"strings"
	"time"
)

// Response is what a fetcher got: the URL that finally served it, its media type and, for text, the body decoded to
// UTF-8. Other media types come without a body.
type Response struct {
	URL       *url.URL
	MediaType string
	Body      []byte
	// Freshness is how long the origin lets the response be kept.
	Freshness Freshness
}

// Freshness is how long an origin lets a response be kept, from its Cache-Control, Expires, Date and Age headers.
type Freshness struct {
	// MaxAge is the time left, Age already taken off; zero for a response the origin forbids keeping.
	MaxAge time.Duration `json:"max_age"`
	// Known says the origin said anything at all.
	Known bool `json:"known"`
}

// Page is a fetched HTML page with its metadata parsed: the title, every <meta> with a key and every <link>, in
// document order.
type Page struct {
	Response

	Title string
	Tags  []Tag
	Links []PageLink
	// LinkedData are the page's JSON-LD blocks, unparsed.
	LinkedData []string
	// Base is what relative URLs on the page resolve against: its <base href>, or the URL it was served from.
	Base *url.URL
}

// Tag is a <meta> keyed by its property, name or itemprop, lowercased.
type Tag struct {
	Key   string
	Value string
}

// PageLink is a <link> with an absolute href; rel, type and sizes are lowercased.
type PageLink struct {
	Rel   []string
	Href  string
	Type  string
	Sizes string
}

// HasRel says whether the link has the relation.
func (l PageLink) HasRel(rel string) bool {
	return slices.Contains(l.Rel, rel)
}

// Value is the first tag's value under any of the keys, which are lowercase.
func (p Page) Value(keys ...string) string {
	for _, tag := range p.Tags {
		if slices.Contains(keys, tag.Key) {
			return tag.Value
		}
	}
	return ""
}

// Alternate is the URL of the page's alternate representation in a media type, such as an oEmbed endpoint; empty
// for none.
func (p Page) Alternate(mediaType string) string {
	for _, link := range p.Links {
		if link.Type == mediaType && link.HasRel("alternate") {
			return link.Href
		}
	}
	return ""
}

// Resolve makes an http or https reference absolute against the page's base; empty for anything else.
func (p Page) Resolve(ref string) string {
	if ref == "" || p.Base == nil {
		return ""
	}
	parsed, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return ""
	}
	resolved := p.Base.ResolveReference(parsed)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}
	return resolved.String()
}
