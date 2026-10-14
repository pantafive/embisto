package meta

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pantafive/embisto/card"
)

// appleTouchSize is the size iOS asks of an apple-touch-icon that does not state one.
const appleTouchSize = 180

// OpenGraph reads the og: and article: properties.
type OpenGraph struct{}

func (OpenGraph) Extract(_ context.Context, in card.Input) (card.Facts, error) {
	doc := in.Page
	fields := card.Facts{
		Source:       card.SourceOpenGraph,
		Title:        doc.Value("og:title"),
		Description:  doc.Value("og:description"),
		SiteName:     doc.Value("og:site_name"),
		CanonicalURL: doc.Resolve(doc.Value("og:url")),
		PublishedAt:  parseTime(doc.Value("article:published_time")),
	}
	// The property is meant to link to a profile, but many pages put the name there.
	if author := doc.Value("article:author"); !strings.Contains(author, "://") {
		fields.Author = author
	}
	if strings.HasPrefix(doc.Value("og:type"), "video") {
		fields.Type = card.TypeVideo
	}
	for _, t := range doc.Tags {
		openGraphMedia(doc, &fields, t)
	}
	return fields, nil
}

// openGraphMedia applies one og:image or og:video property: the URL starts a new one, the structured properties after
// it describe the last one started.
func openGraphMedia(doc card.Page, fields *card.Facts, t card.Tag) {
	switch t.Key {
	case "og:image", "og:image:url":
		if u := doc.Resolve(t.Value); u != "" {
			fields.Images = append(fields.Images, card.Image{URL: u})
		}
	case "og:video", "og:video:url":
		if u := doc.Resolve(t.Value); u != "" {
			fields.Videos = append(fields.Videos, card.Video{URL: u})
		}
	}
	if len(fields.Images) > 0 {
		openGraphImage(doc, &fields.Images[len(fields.Images)-1], t)
	}
	if len(fields.Videos) > 0 {
		openGraphVideo(doc, &fields.Videos[len(fields.Videos)-1], t)
	}
}

func openGraphImage(doc card.Page, image *card.Image, t card.Tag) {
	switch t.Key {
	case "og:image:secure_url":
		image.URL = cmp.Or(doc.Resolve(t.Value), image.URL)
	case "og:image:width":
		image.Width = number(t.Value)
	case "og:image:height":
		image.Height = number(t.Value)
	}
}

func openGraphVideo(doc card.Page, video *card.Video, t card.Tag) {
	switch t.Key {
	case "og:video:secure_url":
		video.URL = cmp.Or(doc.Resolve(t.Value), video.URL)
	case "og:video:type":
		video.Type = t.Value
	case "og:video:width":
		video.Width = number(t.Value)
	case "og:video:height":
		video.Height = number(t.Value)
	}
}

// Twitter reads the twitter: card properties.
type Twitter struct{}

func (Twitter) Extract(_ context.Context, in card.Input) (card.Facts, error) {
	doc := in.Page
	fields := card.Facts{
		Source:      card.SourceTwitter,
		Title:       doc.Value("twitter:title"),
		Description: doc.Value("twitter:description"),
		Author:      doc.Value("twitter:creator"),
	}
	if u := doc.Resolve(doc.Value("twitter:image", "twitter:image:src")); u != "" {
		fields.Images = []card.Image{{
			URL: u, Width: number(doc.Value("twitter:image:width")), Height: number(doc.Value("twitter:image:height")),
		}}
	}
	if u := doc.Resolve(doc.Value("twitter:player")); u != "" {
		fields.Videos = []card.Video{{
			URL: u, Type: "text/html",
			Width: number(doc.Value("twitter:player:width")), Height: number(doc.Value("twitter:player:height")),
		}}
	}
	return fields, nil
}

// HTML reads what any page has without a metadata vocabulary: the title, the description and author meta tags, the
// canonical link and the icons, falling back to /favicon.ico.
type HTML struct{}

func (HTML) Extract(_ context.Context, in card.Input) (card.Facts, error) {
	doc := in.Page
	fields := card.Facts{
		Source:      card.SourceHTML,
		Title:       doc.Title,
		Description: doc.Value("description"),
		SiteName:    doc.Value("application-name"),
		Author:      doc.Value("author"),
		Keywords:    doc.Value("keywords"),
		PublishedAt: parseTime(doc.Value("datepublished", "uploaddate", "date")),
		Icons:       icons(doc),
	}
	for _, l := range doc.Links {
		if l.HasRel("canonical") {
			fields.CanonicalURL = l.Href
			break
		}
	}
	return fields, nil
}

// icons are the page's icons, the largest first; an SVG last, since not every client draws one.
func icons(doc card.Page) []card.Image {
	var found []rankedIcon
	for _, l := range doc.Links {
		if icon, ok := iconOf(l); ok {
			found = append(found, icon)
		}
	}
	if len(found) == 0 {
		return []card.Image{{URL: doc.Resolve("/favicon.ico")}}
	}
	slices.SortStableFunc(found, func(a, b rankedIcon) int { return b.rank - a.rank })
	icons := make([]card.Image, len(found))
	for i, r := range found {
		icons[i] = r.image
	}
	return icons
}

type rankedIcon struct {
	image card.Image
	rank  int
}

func iconOf(l card.PageLink) (rankedIcon, bool) {
	isAppleTouch := l.HasRel("apple-touch-icon") || l.HasRel("apple-touch-icon-precomposed")
	if !isAppleTouch && !l.HasRel("icon") {
		return rankedIcon{}, false
	}
	width, height := size(l.Sizes)
	if width == 0 && isAppleTouch {
		width, height = appleTouchSize, appleTouchSize
	}
	rank := width
	if l.Type == "image/svg+xml" || strings.HasSuffix(strings.ToLower(l.Href), ".svg") {
		rank = -1
	}
	return rankedIcon{card.Image{URL: l.Href, Width: width, Height: height}, rank}, true
}

// size reads the first WxH of a sizes attribute.
func size(sizes string) (int, int) {
	first, _, _ := strings.Cut(strings.TrimSpace(sizes), " ")
	width, height, ok := strings.Cut(first, "x")
	if !ok {
		return 0, 0
	}
	return number(width), number(height)
}

func number(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// parseTime reads the date forms pages use; zero for anything else, and for a date outside the years 1 to 9999,
// which no card can carry: 9999-12-31T23:00:00-05:00 is in the year 10000 once in UTC.
func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z0700", "2006-01-02T15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, s); err == nil {
			if year := parsed.UTC().Year(); year < 1 || year > 9999 {
				return time.Time{}
			}
			return parsed.UTC()
		}
	}
	return time.Time{}
}
