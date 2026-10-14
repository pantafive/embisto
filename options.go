package embisto

import (
	"log/slog"
	"net/netip"
	"time"

	"github.com/pantafive/embisto/cache"
	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/imagesize"
	"github.com/pantafive/embisto/internal/meta"
	"github.com/pantafive/embisto/internal/oembed"
	"github.com/pantafive/embisto/sites"
)

// Options configure the pipeline. A zero number takes its default; a nil list takes the default modules, an empty one
// runs none.
type Options struct {
	// Timeout bounds one preview, every request it makes included.
	Timeout time.Duration
	// MaxBodySize is the most bytes read of one response; a larger one fails with too_large.
	MaxBodySize  int64
	MaxRedirects int
	UserAgent    string
	// DefaultAcceptLanguage is the Accept-Language a page is asked in, unless a request names its own language; sites
	// otherwise answer in the language of wherever the server runs.
	DefaultAcceptLanguage string
	// AllowedNetworks are reachable although not public, for a deployment that previews its own intranet.
	AllowedNetworks []netip.Prefix
	// Fetcher replaces the client the fields above configure, and with it the guard against non-public addresses.
	Fetcher card.Fetcher
	Sources []card.Source
	// HomePages remembers what each site's home page says, so that a description or a picture a site puts on every
	// page is not taken for one page's own. Nil reads no home page. A store shared between instances shares what
	// they learn; purging a URL forgets its site.
	HomePages cache.Store
	// Providers know particular sites and are asked first, in order: the first that knows a URL is the only one asked.
	Providers []card.Provider
	// Fallbacks are asked only about a URL no provider knows, such as the provider for every site on the oEmbed
	// list; so a provider added to Providers always wins over them.
	Fallbacks []card.Provider
	Enrichers []card.Enricher
	// Log hears about sources and providers that failed; the preview goes on without them. Nil logs nothing.
	Log *slog.Logger
	// Observer hears what the pipeline does, for metrics; nil hears nothing.
	Observer Observer
}

// Observer hears what the pipeline does, for metrics. It is told names and codes, never URLs.
type Observer interface {
	// Fetched is one request a stage made, with its outcome: ok, a card code, or canceled. The stage is page, or the
	// name of the source, provider or enricher that asked.
	Fetched(stage, outcome string, took time.Duration)
	// Matched is a provider asked about a URL it knows.
	Matched(provider string)
	// Made is a card made afresh, with its class (full, partial, minimal, wall or error) and the provider that shaped
	// it, or generic.
	Made(class, source string)
}

// Defaults are the options embisto ships with; the home pages are kept in this process's memory.
func Defaults() Options {
	return Options{
		Timeout:               5 * time.Second,
		MaxBodySize:           5 << 20,
		MaxRedirects:          5,
		UserAgent:             "embisto/" + Version + " (+https://github.com/pantafive/embisto)",
		DefaultAcceptLanguage: "en",
		Sources: []card.Source{
			oembed.Discovery{Providers: oembed.Known()},
			meta.OpenGraph{},
			meta.JSONLD{},
			meta.Twitter{},
			meta.HTML{},
		},
		HomePages: cache.NewMemory(defaultHomePages, nil),
		Providers: []card.Provider{sites.YouTube{}, sites.X{}, sites.Reddit{}, sites.TikTok{}, sites.Walled{}},
		Fallbacks: []card.Provider{sites.OEmbed{}},
		Enrichers: []card.Enricher{imagesize.Enricher{
			Concurrency: 4, Timeout: 2 * time.Second, MaxBytes: 64 << 10, MaxImages: 8,
		}},
	}
}

// defaultHomePages is how many bytes of what home pages say the defaults remember: tens of thousands of sites.
const defaultHomePages = 16 << 20
