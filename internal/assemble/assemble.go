// Package assemble merges what the sources found into one preview, field by field, by a table of priorities.
package assemble

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/trace"
)

type field int

const (
	title field = iota
	description
	siteName
	author
	authorURL
	keywords
	publishedAt
	canonicalURL
	icon
	images
	video
	embed
	kind
	duration
	limited
)

// priority orders the generic sources for each field. Site providers come before all of them; a source the table
// does not name comes after.
var priority = map[field][]string{
	title:       {card.SourceOEmbed, card.SourceOpenGraph, card.SourceJSONLD, card.SourceTwitter, card.SourceHTML},
	description: {card.SourceOpenGraph, card.SourceJSONLD, card.SourceTwitter, card.SourceHTML},
	siteName:    {card.SourceOpenGraph, card.SourceOEmbed, card.SourceJSONLD, card.SourceHTML},
	// Structured data names the people and the date; Open Graph rarely has the author and pages fill its date
	// carelessly.
	author:       {card.SourceJSONLD, card.SourceOEmbed, card.SourceOpenGraph, card.SourceHTML, card.SourceTwitter},
	authorURL:    {card.SourceOEmbed, card.SourceJSONLD},
	keywords:     {card.SourceHTML, card.SourceJSONLD},
	publishedAt:  {card.SourceJSONLD, card.SourceOpenGraph, card.SourceHTML},
	canonicalURL: {card.SourceHTML, card.SourceOpenGraph},
	icon:         {card.SourceHTML},
	images:       {card.SourceOpenGraph, card.SourceTwitter, card.SourceJSONLD, card.SourceOEmbed},
	video:        {card.SourceOpenGraph, card.SourceJSONLD, card.SourceTwitter},
	embed:        {card.SourceOEmbed},
	kind:         {card.SourceOEmbed, card.SourceOpenGraph, card.SourceJSONLD},
	duration:     {card.SourceJSONLD, card.SourceOpenGraph},
	limited:      {card.SourceHTML},
}

// Traced builds the preview of the requested URL, served from final, out of the site providers' facts and the
// generic sources'. Each field takes the first facts in its order that have it. Every field's candidates and the one
// that won are recorded into the trace; a nil one records nothing.
func Traced(t *trace.Trace, requested, final string, providers, generic []card.Facts) card.Preview {
	known := boilerplateOf(slices.Concat(providers, generic))
	providers, generic = known.scrubbed(t, providers), known.scrubbed(t, generic)
	m := merger{providers: providers, generic: generic, trace: t}
	preview := card.Preview{
		URL:          requested,
		CanonicalURL: first(&m, canonicalURL, func(f card.Facts) string { return f.CanonicalURL }),
		Title:        first(&m, title, func(f card.Facts) string { return f.Title }),
		Description:  first(&m, description, func(f card.Facts) string { return f.Description }),
		SiteName:     first(&m, siteName, func(f card.Facts) string { return f.SiteName }),
		Author:       first(&m, author, func(f card.Facts) string { return f.Author }),
		AuthorURL:    first(&m, authorURL, func(f card.Facts) string { return f.AuthorURL }),
		Keywords:     first(&m, keywords, func(f card.Facts) string { return f.Keywords }),
		PublishedAt:  first(&m, publishedAt, func(f card.Facts) time.Time { return f.PublishedAt }),
		Icon:         firstOf(&m, icon, func(f card.Facts) []card.Image { return f.Icons }),
		Images:       m.images(),
		Video:        firstOf(&m, video, func(f card.Facts) []card.Video { return f.Videos }),
		Embed:        first(&m, embed, func(f card.Facts) card.Embed { return f.Embed }),
	}
	if preview.CanonicalURL == "" {
		preview.CanonicalURL = final
	}
	if len(preview.Images) > 0 {
		preview.Image = preview.Images[0]
	}
	preview.Type = typeOf(preview, first(&merger{providers: providers, generic: generic, trace: t}, kind, func(f card.Facts) card.Type { return f.Type }))
	preview.Duration = first(&merger{providers: providers, generic: generic, trace: t}, duration, func(f card.Facts) int { return f.Duration })
	preview.Limited = first(&merger{providers: providers, generic: generic, trace: t}, limited, func(f card.Facts) card.Limit { return f.Limited })
	preview.Sources = m.sources()
	preview.Links = links(preview.CanonicalURL, slices.Concat(providers, generic))
	return preview
}

// Media is the preview of a URL that serves an image or a video file rather than a page.
func Media(requested, final, mediaType string) card.Preview {
	preview := card.Preview{URL: requested, CanonicalURL: final, Type: card.TypeLink}
	switch {
	case strings.HasPrefix(mediaType, "image/"):
		preview.Type, preview.Image = card.TypePhoto, card.Image{URL: final}
		preview.Images = []card.Image{preview.Image}
		preview.Links = []card.Link{{Href: final, Rel: []string{card.RelImage, card.RelFile}, Type: mediaType}}
	case strings.HasPrefix(mediaType, "video/"):
		preview.Type, preview.Video = card.TypeVideo, card.Video{URL: final, Type: mediaType}
		preview.Links = []card.Link{{Href: final, Rel: []string{card.RelPlayer, card.RelFile}, Type: mediaType}}
	default:
		preview.Links = []card.Link{{Href: final, Rel: []string{card.RelFile}, Type: mediaType}}
	}
	return preview
}

// typeOf trusts the type a source declared, except a declared link that turns out to carry a video.
func typeOf(preview card.Preview, declared card.Type) card.Type {
	switch {
	case declared != "" && declared != card.TypeLink:
		return declared
	case preview.Video.URL != "":
		return card.TypeVideo
	default:
		return card.TypeLink
	}
}

// merger walks the facts in each field's order and remembers which sources a preview took a field from.
type merger struct {
	providers []card.Facts
	generic   []card.Facts
	used      []string
	trace     *trace.Trace
}

func (m *merger) order(f field) []card.Facts {
	named := priority[f]
	ordered := slices.Clone(m.providers)
	for _, name := range named {
		if at := slices.IndexFunc(m.generic, func(facts card.Facts) bool { return facts.Source == name }); at >= 0 {
			ordered = append(ordered, m.generic[at])
		}
	}
	for _, facts := range m.generic {
		if !isBuiltIn(facts.Source) {
			ordered = append(ordered, facts)
		}
	}
	return ordered
}

func isBuiltIn(source string) bool {
	return slices.Contains([]string{card.SourceOEmbed, card.SourceOpenGraph, card.SourceTwitter, card.SourceHTML, card.SourceJSONLD}, source)
}

func (m *merger) use(source string) {
	if !slices.Contains(m.used, source) {
		m.used = append(m.used, source)
	}
}

// sources are the used ones, providers first, then the generic sources in the order they ran.
func (m *merger) sources() []string {
	var ordered []string
	for _, facts := range slices.Concat(m.providers, m.generic) {
		if slices.Contains(m.used, facts.Source) && !slices.Contains(ordered, facts.Source) {
			ordered = append(ordered, facts.Source)
		}
	}
	return ordered
}

func first[T comparable](m *merger, f field, get func(card.Facts) T) T {
	var zero T
	order := m.order(f)
	for i, facts := range order {
		if value := get(facts); value != zero {
			m.use(facts.Source)
			m.decide(f, order, i, func(facts card.Facts) any { return get(facts) }, zero)
			return value
		}
	}
	m.decide(f, order, -1, func(facts card.Facts) any { return get(facts) }, zero)
	return zero
}

func firstOf[T any](m *merger, f field, get func(card.Facts) []T) T {
	order := m.order(f)
	listed := func(facts card.Facts) any {
		if values := get(facts); len(values) > 0 {
			return values[0]
		}
		return nil
	}
	for i, facts := range order {
		if values := get(facts); len(values) > 0 {
			m.use(facts.Source)
			m.decide(f, order, i, listed, nil)
			return values[0]
		}
	}
	m.decide(f, order, -1, listed, nil)
	var zero T
	return zero
}

// images are every source's pictures, each URL once: the first source to name it places it, a later one only fills
// in a size the first did not know.
func (m *merger) images() []card.Image {
	var all []card.Image
	seen := map[string]int{}
	decision := trace.Decision{Field: fieldNames[images], Reason: "every source's pictures, each URL once, in the field's order"}
	defer func() { m.trace.Decide(decision) }()
	for _, facts := range m.order(images) {
		for _, image := range facts.Images {
			m.use(facts.Source)
			decision.Candidates = append(decision.Candidates, trace.Candidate{Source: facts.Source, Value: shown(image)})
			decision.Winner = cmp.Or(decision.Winner, facts.Source)
			at, ok := seen[image.URL]
			switch {
			case !ok:
				seen[image.URL] = len(all)
				all = append(all, image)
			case all[at].Width == 0 && image.Width != 0:
				all[at].Width, all[at].Height = image.Width, image.Height
			}
		}
	}
	return all
}
