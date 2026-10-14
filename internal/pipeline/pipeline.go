// Package pipeline makes a preview: fetch the page, run the generic sources over it, ask the site provider that
// knows it, merge their facts, enrich the result. It keeps nothing between previews; what it remembers of home pages
// is in the store the home-page source is given.
package pipeline

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/assemble"
	"github.com/pantafive/embisto/internal/homepage"
	"github.com/pantafive/embisto/internal/meta"
	"github.com/pantafive/embisto/internal/quality"
	"github.com/pantafive/embisto/internal/target"
	"github.com/pantafive/embisto/internal/trace"
)

const acceptHTML = "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8"

// pageShare is the part of a preview's time its page may take, so that a provider can still stand in for a page
// that did not answer.
const pageShare = 0.6

// Observer hears what the pipeline does, for metrics. It is told names and codes, never URLs.
type Observer interface {
	Fetched(stage, outcome string, took time.Duration)
	Matched(provider string)
	Made(class, source string)
}

// Options are the pipeline's modules and bounds, every one already set.
type Options struct {
	Timeout  time.Duration
	Language string
	Fetcher  card.Fetcher
	Sources  []card.Source
	// HomePages, when set, offers what a site's home page says as boilerplate; nil reads no home page.
	HomePages *homepage.Source
	// Providers are asked first, in order; Fallbacks only for a URL no provider knows.
	Providers []card.Provider
	Fallbacks []card.Provider
	Enrichers []card.Enricher
	Log       *slog.Logger
	Observer  Observer
}

// Pipeline makes previews.
type Pipeline struct {
	opts Options
}

// New builds the pipeline.
func New(opts Options) *Pipeline {
	return &Pipeline{opts: opts}
}

// run is one preview being made: which stage made the card, and how long each stage's responses may be kept.
type run struct {
	stages  map[string]card.Freshness
	decider string
}

// note keeps the shortest freshness a stage's responses allowed.
func (r *run) note(stage string, said card.Freshness) {
	if !said.Known {
		return
	}
	if known, ok := r.stages[stage]; !ok || said.MaxAge < known.MaxAge {
		r.stages[stage] = said
	}
}

// Extract makes the preview of the request's URL. It fails with a *card.Error, or with the context's error when the
// caller gave up. A module that panics fails its preview as internal, never the program serving it.
func (p *Pipeline) Extract(ctx context.Context, req card.Request) (result card.Result, failed error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			p.opts.Log.ErrorContext(ctx, "a module panicked; the preview fails", slog.Any("panic", recovered), slog.String("stack", string(debug.Stack())))
			result, failed = card.Result{}, &card.Error{Code: card.CodeInternal, Err: fmt.Errorf("panic: %v", recovered)} //nolint:err113 // the panic's own value
		}
	}()
	parsed, err := target.Parse(req.URL)
	if err != nil {
		return card.Result{}, err //nolint:wrapcheck // the *card.Error is the contract
	}
	language := cmp.Or(target.Language(req.Options.Language), p.opts.Language)
	ctx, cancel := context.WithTimeout(card.WithLanguage(ctx, language), p.opts.Timeout)
	defer cancel()
	r := &run{stages: map[string]card.Freshness{}}
	preview, provider, err := p.make(ctx, r, card.Input{URL: parsed, Options: req.Options})
	if err == nil && ctx.Err() != nil {
		// A card finished past the deadline is no answer: the caller has stopped waiting for it.
		err = &card.Error{Code: card.CodeTimeout, Err: ctx.Err()}
	}
	p.judge(ctx, parsed, preview, provider, err)
	if err != nil {
		return card.Result{}, err
	}
	return card.Result{Preview: preview, Freshness: r.stages[r.decider], Provider: provider}, nil
}

// Purge forgets what the home-page source remembers of the URL's site.
func (p *Pipeline) Purge(ctx context.Context, req card.Request) (int, error) {
	parsed, err := target.Parse(req.URL)
	if err != nil {
		return 0, err //nolint:wrapcheck // the *card.Error is the contract
	}
	if p.opts.HomePages == nil {
		return 0, nil
	}
	return p.opts.HomePages.Forget(ctx, parsed) //nolint:wrapcheck // it names the site
}

// make runs the pipeline: one provider for a URL it answers by itself; otherwise the page, its generic sources and
// the one provider that knows its address. A page that fails, or turns out to be a wall, is stood in for by the
// provider that knows the requested URL, when there is one. It says which provider shaped the card; empty is none.
func (p *Pipeline) make(ctx context.Context, r *run, in card.Input) (card.Preview, string, error) {
	requested := in.URL.String()
	provider := p.providerFor(in.URL)
	if provider != nil && provider.Match(in.URL) == card.MatchedAlone {
		facts, err := p.ask(ctx, r, provider, in)
		if isDefinitive(err) {
			return card.Preview{}, provider.Name(), err
		}
		if err == nil && !facts.IsEmpty() {
			r.decider = provider.Name()
			return p.enrich(ctx, r, assemble.Traced(trace.From(ctx), requested, requested, []card.Facts{facts}, nil)), provider.Name(), nil
		}
	}
	// The page's own response decides how long the card may be kept.
	r.decider = stagePage
	pageCtx, cancelPage := context.WithTimeout(ctx, time.Duration(float64(p.opts.Timeout)*pageShare))
	resp, err := p.fetcherFor(r, stagePage).Fetch(pageCtx, in.URL, acceptHTML)
	cancelPage()
	if err != nil {
		return p.standIn(ctx, r, provider, in, card.Preview{}, notFound(err))
	}
	if resp.MediaType != "text/html" && resp.MediaType != "application/xhtml+xml" {
		return p.enrich(ctx, r, assemble.Media(requested, resp.URL.String(), resp.MediaType)), "", nil
	}
	in.Page = meta.Parse(resp)
	return p.fromPage(ctx, r, provider, in)
}

// fromPage makes the card of a fetched page: its generic sources, then the provider that knows the address it was
// served from; or the stand-in for a wall.
func (p *Pipeline) fromPage(ctx context.Context, r *run, provider card.Provider, in card.Input) (card.Preview, string, error) {
	requested := in.URL.String()
	in.Facts = p.extract(ctx, r, in)
	if ctx.Err() != nil {
		return card.Preview{}, "", &card.Error{Code: card.CodeTimeout, Err: ctx.Err()}
	}
	if wall := meta.Wall(in.URL, in.Page); wall != "" {
		trace.From(ctx).Step(trace.Step{Kind: trace.KindWall, Name: string(wall), Detail: "the page is a wall: " + in.Page.URL.String()})
		return p.standIn(ctx, r, provider, in, walled(ctx, requested, in.Facts, wall), nil)
	}
	var provided []card.Facts
	source := ""
	if found, asked, ok := p.servedProvider(in); ok {
		facts, err := p.ask(ctx, r, found, asked)
		if isDefinitive(err) {
			return card.Preview{}, found.Name(), err
		}
		if err == nil && !facts.IsEmpty() {
			provided, source = []card.Facts{facts}, found.Name()
		}
	}
	return p.enrich(ctx, r, assemble.Traced(trace.From(ctx), requested, in.Page.URL.String(), provided, in.Facts)), source, nil
}

// servedProvider is the provider that knows the address the page was served from, and what it is asked. A provider
// that also knows the requested URL is asked about it, which may say more, such as a short or a start time; one that
// knows only the address a link such as bit.ly or t.co redirected to is asked about that address, even one that
// answers from the URL alone.
func (p *Pipeline) servedProvider(in card.Input) (card.Provider, card.Input, bool) {
	found := p.providerFor(in.Page.URL)
	if found == nil {
		return nil, card.Input{}, false
	}
	knowsRequested := found.Match(in.URL) != card.Unmatched
	switch found.Match(in.Page.URL) {
	case card.MatchedWithPage:
		if !knowsRequested {
			in.URL = in.Page.URL
		}
		return found, in, true
	case card.MatchedAlone:
		// One that knows the requested URL was asked about it before the page was fetched.
		if !knowsRequested {
			return found, card.Input{URL: in.Page.URL, Options: in.Options}, true
		}
	case card.Unmatched:
	}
	return nil, card.Input{}, false
}

// standIn asks the provider that knows the requested URL to stand in for a page that failed or was a wall, without
// the page; nothing found leaves the failure, or the wall's card, as it was.
func (p *Pipeline) standIn(ctx context.Context, r *run, provider card.Provider, in card.Input, wall card.Preview, failure error) (card.Preview, string, error) {
	code := card.CodeOf(failure)
	if provider != nil && (failure == nil || code == card.CodeFetchFailed || code == card.CodeTimeout) {
		facts, err := p.ask(ctx, r, provider, card.Input{URL: in.URL, Options: in.Options})
		if isDefinitive(err) {
			return card.Preview{}, provider.Name(), err
		}
		if err == nil && standsFor(&facts, in.URL, failure) {
			r.decider = provider.Name()
			// Only the provider speaks: what the page said is a wall's text or nothing.
			return p.enrich(ctx, r, assemble.Traced(trace.From(ctx), in.URL.String(), in.URL.String(), []card.Facts{facts}, nil)), provider.Name(), nil
		}
	}
	if failure != nil {
		return card.Preview{}, "", failure
	}
	return p.enrich(ctx, r, wall), "", nil
}

// standsFor says whether a provider's facts may stand in for the page. For a wall, anything will. For a page that
// failed, the provider must say something of the item itself: a title, a picture, or why the site walls it off; a card
// of only what the URL already says, such as the provider's name and a player of the URL, would hide that the page is
// not there. After a host's name did not resolve, a player of that host is dropped: it would show an empty frame.
func standsFor(facts *card.Facts, requested *url.URL, failure error) bool {
	if failure != nil && unresolved(failure) && embedsHost(facts.Embed.HTML, requested.Hostname()) {
		facts.Embed = card.Embed{}
		// Rich content is its player: without it, the card is a link.
		if facts.Type == card.TypeRich {
			facts.Type = card.TypeLink
		}
	}
	if facts.IsEmpty() {
		return false
	}
	return failure == nil || facts.Title != "" || len(facts.Images) > 0 || facts.Limited != ""
}

// unresolved says whether a fetch failed because the host's name does not resolve.
func unresolved(err error) bool {
	_, ok := errors.AsType[*net.DNSError](err)
	return ok
}

// embedsHost says whether the embed's player is served from the host.
func embedsHost(fragment, host string) bool {
	source, err := url.Parse(assemble.IframeSource(fragment))
	return fragment != "" && err == nil && strings.EqualFold(source.Hostname(), host)
}

// walled is the card of a page that was a wall: the URL, the site's name and its icon, and nothing of the wall's own
// text or pictures.
func walled(ctx context.Context, requested string, generic []card.Facts, wall card.Limit) card.Preview {
	kept := card.Facts{Source: card.SourceHTML, Limited: wall}
	for _, facts := range generic {
		kept.SiteName = cmp.Or(kept.SiteName, facts.SiteName)
		if facts.Source == card.SourceHTML {
			kept.Icons = facts.Icons
		}
	}
	for _, facts := range generic {
		// A page that names only its site is titled with the site's name.
		if facts.Source == card.SourceHTML && wall == card.LimitBlocked {
			kept.SiteName = cmp.Or(kept.SiteName, facts.Title)
		}
	}
	preview := assemble.Traced(trace.From(ctx), requested, requested, nil, []card.Facts{kept})
	preview.Sources = nil
	return preview
}

// notFound is a page that answered 404 or 410: the item is gone, which is no fetch failure.
func notFound(err error) error {
	var failed *card.Error
	if errors.As(err, &failed) && (failed.Status == http.StatusNotFound || failed.Status == http.StatusGone) {
		return &card.Error{Code: card.CodeNotFound, Status: failed.Status, Err: failed.Err}
	}
	return err
}

// isDefinitive is a provider's answer that the item does not exist, which no other source can overrule.
func isDefinitive(err error) bool {
	return err != nil && card.CodeOf(err) == card.CodeNotFound
}

// providerFor is the provider that knows the URL: the first of the providers, else the first of the fallbacks. One
// URL, one provider.
func (p *Pipeline) providerFor(u *url.URL) card.Provider {
	for _, list := range [][]card.Provider{p.opts.Providers, p.opts.Fallbacks} {
		for _, provider := range list {
			if provider.Match(u) != card.Unmatched {
				return provider
			}
		}
	}
	return nil
}

// fetcherFor is the fetcher a stage makes its requests through, observed under the stage's name.
func (p *Pipeline) fetcherFor(r *run, stage string) card.Fetcher {
	return observedFetcher{next: p.opts.Fetcher, stage: stage, observer: p.opts.Observer, run: r}
}

func (p *Pipeline) extract(ctx context.Context, r *run, in card.Input) []card.Facts {
	sources := p.opts.Sources
	if p.opts.HomePages != nil {
		sources = append(slices.Clip(sources), p.opts.HomePages)
	}
	var found []card.Facts
	for _, source := range sources {
		if ctx.Err() != nil {
			break
		}
		in.Fetcher = p.fetcherFor(r, stageOf(source, "source"))
		started := time.Now()
		facts, err := source.Extract(ctx, in)
		trace.From(ctx).Step(trace.Step{Kind: trace.KindSource, Name: sourceName(source, facts), Detail: summary(facts), Outcome: outcomeOf(err), Took: time.Since(started)})
		if err != nil {
			p.opts.Log.WarnContext(ctx, "a source failed; the preview goes on without it", slog.String("source", fmt.Sprintf("%T", source)), slog.String("url", in.URL.String()), slog.Any("error", err))
			continue
		}
		found = append(found, bound(facts))
	}
	return found
}

func (p *Pipeline) ask(ctx context.Context, r *run, provider card.Provider, in card.Input) (card.Facts, error) {
	p.opts.Observer.Matched(provider.Name())
	in.Fetcher = p.fetcherFor(r, provider.Name())
	started := time.Now()
	facts, err := provider.Provide(ctx, in)
	trace.From(ctx).Step(trace.Step{Kind: trace.KindProvider, Name: provider.Name(), Detail: summary(facts), Outcome: outcomeOf(err), Took: time.Since(started)})
	if err != nil && !isDefinitive(err) {
		p.opts.Log.WarnContext(ctx, "a site provider failed; the preview goes on without it", slog.String("provider", provider.Name()), slog.String("host", in.URL.Hostname()), slog.Any("error", err))
		return card.Facts{}, err //nolint:wrapcheck // the provider's own error, logged here
	}
	facts.Source = provider.Name()
	return bound(facts), err
}

func (p *Pipeline) enrich(ctx context.Context, r *run, preview card.Preview) card.Preview {
	for _, enricher := range p.opts.Enrichers {
		started := time.Now()
		enriched, err := enricher.Enrich(ctx, preview, p.fetcherFor(r, stageOf(enricher, "enricher")))
		trace.From(ctx).Step(trace.Step{Kind: trace.KindEnricher, Name: stageOf(enricher, "enricher"), Outcome: outcomeOf(err), Took: time.Since(started)})
		if err != nil {
			p.opts.Log.WarnContext(ctx, "an enricher failed; the preview goes on without it", slog.String("enricher", fmt.Sprintf("%T", enricher)), slog.String("url", preview.URL), slog.Any("error", err))
			continue
		}
		preview = enriched
	}
	return preview
}

// judge tells the observer how a card came out, and logs a poor one with its host, never its URL, so the domains
// worth a provider can be ranked from the logs.
func (p *Pipeline) judge(ctx context.Context, requested *url.URL, preview card.Preview, provider string, err error) {
	class := quality.Classify(preview, err)
	source := cmp.Or(provider, quality.SourceGeneric)
	p.opts.Observer.Made(string(class), source)
	if class == quality.ClassFull || class == quality.ClassPartial {
		return
	}
	attrs := []any{slog.String("host", requested.Hostname()), slog.String("class", string(class)), slog.String("source", source)}
	if err != nil {
		attrs = append(attrs, slog.String("code", string(card.CodeOf(err))))
	}
	if preview.Limited != "" {
		attrs = append(attrs, slog.String("limited", string(preview.Limited)))
	}
	p.opts.Log.InfoContext(ctx, "poor card", attrs...)
}
