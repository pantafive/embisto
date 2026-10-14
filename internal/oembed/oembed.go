// Package oembed reads oEmbed: the endpoint a page links to, or the one the oembed.com provider list names for a URL.
package oembed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/pantafive/embisto/card"
)

// MediaType is the type of the alternate link a page names its JSON oEmbed endpoint with.
const MediaType = "application/json+oembed"

// Discovery is the generic source: the oEmbed endpoint the page itself links to. An endpoint the provider list does
// not name gives its facts without its HTML, since a page can name any endpoint and the HTML would run in clients.
type Discovery struct {
	Providers *Providers
}

func (Discovery) Name() string {
	return card.SourceOEmbed
}

func (d Discovery) Extract(ctx context.Context, in card.Input) (card.Facts, error) {
	href := in.Page.Alternate(MediaType)
	if href == "" {
		return card.Facts{}, nil
	}
	endpoint, err := url.Parse(href)
	if err != nil {
		return card.Facts{}, fmt.Errorf("parsing the oEmbed link: %w", err)
	}
	return Get(ctx, in.Fetcher, withSize(endpoint, in.Options), d.Providers.Endpoints()...)
}

// Get fetches an oEmbed endpoint's answer. Its HTML is kept only when the answer came from under one of the trusted
// endpoints; see trusts.
func Get(ctx context.Context, fetcher card.Fetcher, endpoint *url.URL, trusted ...string) (card.Facts, error) {
	resp, err := fetcher.Fetch(ctx, endpoint, "application/json")
	if err != nil {
		return card.Facts{}, fmt.Errorf("fetching oEmbed: %w", err)
	}
	var answer response
	if err := json.Unmarshal(resp.Body, &answer); err != nil {
		return card.Facts{}, fmt.Errorf("decoding oEmbed: %w", err)
	}
	return answer.facts(endpoint, trusts(endpoint, resp, trusted)), nil
}

// trusts says whether an answer's HTML, which clients run, may be kept: it was asked for and answered over https,
// as JSON, on the host asked, from under one of the trusted endpoints. A listed host that redirects elsewhere, or an
// answer anyone on the path could have rewritten, would otherwise put a stranger's script into the card.
func trusts(asked *url.URL, resp card.Response, trusted []string) bool {
	final := resp.URL
	if final == nil || !isSecure(asked) || !sameHost(final, asked) || !isJSON(resp.MediaType) {
		return false
	}
	for _, endpoint := range trusted {
		if listed, err := url.Parse(endpoint); err == nil && isSecure(listed) && sameHost(final, listed) && strings.HasPrefix(final.EscapedPath(), listed.EscapedPath()) {
			return true
		}
	}
	return false
}

func isSecure(u *url.URL) bool {
	return u.Scheme == "https"
}

// sameHost compares the scheme too: the same host over http is another, unauthenticated origin.
func sameHost(a, b *url.URL) bool {
	return a.Scheme == b.Scheme && strings.EqualFold(a.Host, b.Host)
}

func isJSON(mediaType string) bool {
	return mediaType == "application/json" || mediaType == MediaType || mediaType == "text/json" || strings.HasSuffix(mediaType, "+json")
}

// Endpoint is the provider's endpoint asked about the URL in JSON, within the options' size.
func Endpoint(base string, target *url.URL, opts card.Options) (*url.URL, error) {
	endpoint, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("parsing the endpoint %q: %w", base, err)
	}
	query := endpoint.Query()
	query.Set("url", target.String())
	query.Set("format", "json")
	endpoint.RawQuery = query.Encode()
	return withSize(endpoint, opts), nil
}

func withSize(endpoint *url.URL, opts card.Options) *url.URL {
	query := endpoint.Query()
	if opts.MaxWidth > 0 {
		query.Set("maxwidth", strconv.Itoa(opts.MaxWidth))
	}
	if opts.MaxHeight > 0 {
		query.Set("maxheight", strconv.Itoa(opts.MaxHeight))
	}
	sized := *endpoint
	sized.RawQuery = query.Encode()
	return &sized
}

type response struct {
	Type            string    `json:"type"`
	Title           string    `json:"title"`
	AuthorName      string    `json:"author_name"`
	AuthorURL       string    `json:"author_url"`
	ProviderName    string    `json:"provider_name"`
	URL             string    `json:"url"`
	HTML            string    `json:"html"`
	Width           dimension `json:"width"`
	Height          dimension `json:"height"`
	ThumbnailURL    string    `json:"thumbnail_url"`
	ThumbnailWidth  dimension `json:"thumbnail_width"`
	ThumbnailHeight dimension `json:"thumbnail_height"`
}

func (r response) facts(endpoint *url.URL, isTrusted bool) card.Facts {
	facts := card.Facts{
		Source: card.SourceOEmbed, Title: r.Title, Author: r.AuthorName, AuthorURL: resolve(endpoint, r.AuthorURL), SiteName: r.ProviderName,
	}
	switch r.Type {
	case "photo":
		facts.Type = card.TypePhoto
		if photo := resolve(endpoint, r.URL); photo != "" {
			facts.Images = append(facts.Images, card.Image{URL: photo, Width: int(r.Width), Height: int(r.Height)})
		}
	case "video":
		facts.Type = card.TypeVideo
	}
	if thumbnail := resolve(endpoint, r.ThumbnailURL); thumbnail != "" {
		facts.Images = append(facts.Images, card.Image{URL: thumbnail, Width: int(r.ThumbnailWidth), Height: int(r.ThumbnailHeight)})
	}
	if isTrusted && r.HTML != "" {
		facts.Embed = card.Embed{HTML: r.HTML, Width: int(r.Width), Height: int(r.Height), Provider: r.ProviderName}
		// Rich content is its HTML: without it, the page is a plain link.
		if r.Type == "rich" {
			facts.Type = card.TypeRich
		}
	}
	return facts
}

// resolve makes an http or https reference absolute against the endpoint; empty for anything else.
func resolve(endpoint *url.URL, ref string) string {
	if ref == "" {
		return ""
	}
	parsed, err := endpoint.Parse(ref)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return parsed.String()
}

// dimension is a size some providers write as a number and others as a string.
type dimension int

func (d *dimension) UnmarshalJSON(raw []byte) error {
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil //nolint:nilerr // null or a shape no client could use: the size stays unknown
		}
		number = json.Number(text)
	}
	value, err := number.Float64()
	if err != nil || value < 0 {
		return nil //nolint:nilerr // an unreadable size stays unknown rather than failing the whole answer
	}
	*d = dimension(value)
	return nil
}
