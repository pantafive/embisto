package card

import (
	"context"
	"io"
	"net/url"
)

// The pipeline runs its stages in a fixed order: fetch the page, run every generic source over it, run the site
// providers that match, merge the facts field by field, enrich the preview, render it.

// Request is one URL to preview with the options that shape its preview.
type Request struct {
	URL     string  `json:"url"`
	Options Options `json:"options,omitzero"`
}

// Options shape one preview; none is read from the target URL's query.
type Options struct {
	// MaxWidth and MaxHeight bound the embed a provider offers; zero leaves it to the provider.
	MaxWidth  int `json:"max_width,omitempty"`
	MaxHeight int `json:"max_height,omitempty"`
	// Language is the BCP 47 tag of the language the page is asked in; empty leaves it to the server.
	Language string `json:"language,omitempty"`
}

type languageKey struct{}

// WithLanguage is a context whose fetches ask for pages in the language: an Accept-Language value.
func WithLanguage(ctx context.Context, language string) context.Context {
	return context.WithValue(ctx, languageKey{}, language)
}

// LanguageOf is the language a context's fetches ask for; empty for none.
func LanguageOf(ctx context.Context) string {
	language, _ := ctx.Value(languageKey{}).(string)
	return language
}

// Fetcher gets URLs over the network, in the language the context carries (LanguageOf); it fails with an *Error.
type Fetcher interface {
	// Fetch gets a URL, asking for the media types in accept; only a text body comes with the response.
	Fetch(ctx context.Context, target *url.URL, accept string) (Response, error)
	// Peek gets at most the first limit bytes of a URL of any media type, such as the header of an image.
	Peek(ctx context.Context, target *url.URL, limit int64) (Response, error)
}

// Input is what a source or a provider reads: the requested URL, the page and the fetcher for anything else it needs.
type Input struct {
	URL *url.URL
	// Page is the fetched page; zero for a provider that answers from the URL alone.
	Page    Page
	Options Options
	Fetcher Fetcher
	// Facts are the generic sources' findings, given to site providers; empty for the sources themselves.
	Facts []Facts
}

// Source is a generic extractor that reads any page: Open Graph, Twitter Cards, oEmbed, HTML.
type Source interface {
	Extract(ctx context.Context, in Input) (Facts, error)
}

// Match is how a provider answers for a URL.
type Match int

const (
	// Unmatched is a URL the provider does not know.
	Unmatched Match = iota
	// MatchedWithPage is a URL the provider answers for after reading its page and the generic sources' facts.
	MatchedWithPage
	// MatchedAlone is a URL the provider answers for by itself, such as from the site's API, so the page is not
	// fetched. When the provider fails or finds nothing, the page is fetched after all.
	MatchedAlone
)

// Provider knows one site: its facts outrank every generic source's.
type Provider interface {
	// Name is the provider's name in a preview's sources.
	Name() string
	// Match says whether and how the provider answers for a URL: the requested one before the page is fetched, the
	// one the page was served from after.
	Match(u *url.URL) Match
	Provide(ctx context.Context, in Input) (Facts, error)
}

// Enricher improves an assembled preview, such as by measuring its images.
type Enricher interface {
	Enrich(ctx context.Context, preview Preview, fetcher Fetcher) (Preview, error)
}

// ErrorRenderer is a renderer whose format has its own shape for a failed preview; a renderer without one fails as
// RFC 9457 problem+json.
type ErrorRenderer interface {
	// RenderError writes the failure and says the HTTP status it goes with.
	RenderError(w io.Writer, failed *Error) (int, error)
}

// Renderer writes a preview in one output format.
type Renderer interface {
	// Name is the format's name a client asks for.
	Name() string
	ContentType() string
	Render(w io.Writer, preview Preview) error
}
