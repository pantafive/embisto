package embisto

import (
	"cmp"
	"context"
	"log/slog"
	"time"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/fetch"
	"github.com/pantafive/embisto/internal/homepage"
	"github.com/pantafive/embisto/internal/pipeline"
)

// Version is the library's release.
const Version = "0.1.0"

// How long what a home page says is remembered, how long a home page that said nothing is, and how long reading one
// may take of a preview's time.
const (
	homePageTTL        = 24 * time.Hour
	homePageFailureTTL = time.Hour
	homePageTimeout    = time.Second
)

// Core is the pipeline. It keeps nothing between previews but what Options.HomePages stores.
type Core struct {
	pipeline *pipeline.Pipeline
}

// New builds the pipeline.
func New(opts Options) *Core {
	defaults := Defaults()
	opts.Timeout = cmp.Or(opts.Timeout, defaults.Timeout)
	opts.MaxBodySize = cmp.Or(opts.MaxBodySize, defaults.MaxBodySize)
	opts.MaxRedirects = cmp.Or(opts.MaxRedirects, defaults.MaxRedirects)
	opts.UserAgent = cmp.Or(opts.UserAgent, defaults.UserAgent)
	if opts.Fetcher == nil {
		opts.Fetcher = NewFetcher(opts)
	}
	log := cmp.Or(opts.Log, slog.New(slog.DiscardHandler))
	var homePages *homepage.Source
	if opts.HomePages != nil {
		homePages = homepage.New(opts.HomePages, homepage.Options{TTL: homePageTTL, FailureTTL: homePageFailureTTL, Timeout: homePageTimeout, Log: log})
	}
	var observer pipeline.Observer = unobserved{}
	if opts.Observer != nil {
		observer = opts.Observer
	}
	return &Core{pipeline: pipeline.New(pipeline.Options{
		Timeout:   opts.Timeout,
		Language:  cmp.Or(opts.DefaultAcceptLanguage, defaults.DefaultAcceptLanguage),
		Fetcher:   opts.Fetcher,
		Sources:   orDefault(opts.Sources, defaults.Sources),
		HomePages: homePages,
		Providers: orDefault(opts.Providers, defaults.Providers),
		Fallbacks: orDefault(opts.Fallbacks, defaults.Fallbacks),
		Enrichers: orDefault(opts.Enrichers, defaults.Enrichers),
		Log:       log,
		Observer:  observer,
	})}
}

// Extract makes the preview of the request's URL. It fails with a *card.Error, or with the context's error when the
// caller gave up; a module that panics fails its preview as internal, never the program.
func (c *Core) Extract(ctx context.Context, req card.Request) (card.Result, error) {
	return c.pipeline.Extract(ctx, req) //nolint:wrapcheck // the *card.Error is the contract
}

// Purge forgets what the core remembers of the URL's site: what its home page says.
func (c *Core) Purge(ctx context.Context, req card.Request) (int, error) {
	return c.pipeline.Purge(ctx, req) //nolint:wrapcheck // the *card.Error is the contract
}

// NewFetcher is the client the options configure, which refuses addresses that are not public; a fetcher of your own
// can wrap it, such as to record or to route through a proxy.
func NewFetcher(opts Options) card.Fetcher {
	defaults := Defaults()
	return fetch.New(fetch.Options{
		Timeout:      cmp.Or(opts.Timeout, defaults.Timeout),
		MaxBody:      cmp.Or(opts.MaxBodySize, defaults.MaxBodySize),
		MaxRedirects: cmp.Or(opts.MaxRedirects, defaults.MaxRedirects),
		UserAgent:    cmp.Or(opts.UserAgent, defaults.UserAgent),
		Allowed:      opts.AllowedNetworks,
	})
}

func orDefault[T any](list, fallback []T) []T {
	if list == nil {
		return fallback
	}
	return list
}

type unobserved struct{}

func (unobserved) Fetched(string, string, time.Duration) {}
func (unobserved) Matched(string)                        {}
func (unobserved) Made(string, string)                   {}
