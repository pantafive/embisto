package pipeline

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/trace"
)

// Outcomes of a fetch besides the card codes.
const (
	OutcomeOK       = "ok"
	OutcomeCanceled = "canceled"
	stagePage       = "page"
)

// observedFetcher tells the observer about every request a stage makes through it.
type observedFetcher struct {
	next     card.Fetcher
	stage    string
	observer Observer
	run      *run
}

func (o observedFetcher) Fetch(ctx context.Context, target *url.URL, accept string) (card.Response, error) {
	started := time.Now()
	resp, err := o.next.Fetch(ctx, target, accept)
	o.observe(ctx, "GET", target, resp, err, time.Since(started))
	return resp, err //nolint:wrapcheck // the observed fetcher's own answer
}

func (o observedFetcher) Peek(ctx context.Context, target *url.URL, limit int64) (card.Response, error) {
	started := time.Now()
	resp, err := o.next.Peek(ctx, target, limit)
	o.observe(ctx, "PEEK", target, resp, err, time.Since(started))
	return resp, err //nolint:wrapcheck // the observed fetcher's own answer
}

func (o observedFetcher) observe(ctx context.Context, method string, target *url.URL, resp card.Response, err error, took time.Duration) {
	o.observer.Fetched(o.stage, outcomeOf(err), took)
	if err == nil {
		o.run.note(o.stage, resp.Freshness)
	}
	detail := fmt.Sprintf("%s %s", method, target)
	if err == nil {
		detail += fmt.Sprintf(" → %s, %d bytes", resp.MediaType, len(resp.Body))
		if resp.URL != nil && resp.URL.String() != target.String() {
			detail += ", from " + resp.URL.String()
		}
	}
	trace.From(ctx).Step(trace.Step{Kind: trace.KindFetch, Name: o.stage, Detail: detail, Outcome: outcomeOf(err), Took: took})
}

// sourceName names a source in a trace: its own name, or the one its facts carry.
func sourceName(source card.Source, facts card.Facts) string {
	return stageOf(source, cmp.Or(facts.Source, fmt.Sprintf("%T", source)))
}

// summary is what a source or a provider found, by field, for a trace.
func summary(facts card.Facts) string {
	var found []string
	for name, isSet := range map[string]bool{
		"title": facts.Title != "", "description": facts.Description != "", "site": facts.SiteName != "",
		"author": facts.Author != "", "date": !facts.PublishedAt.IsZero(), "canonical": facts.CanonicalURL != "",
		"type": facts.Type != "", "embed": facts.Embed.HTML != "", "limited": facts.Limited != "",
	} {
		if isSet {
			found = append(found, name)
		}
	}
	for name, count := range map[string]int{"icons": len(facts.Icons), "images": len(facts.Images), "videos": len(facts.Videos)} {
		if count > 0 {
			found = append(found, fmt.Sprintf("%d %s", count, name))
		}
	}
	slices.Sort(found)
	if texts := len(facts.Boilerplate.Texts) + len(facts.Boilerplate.Images); texts > 0 {
		found = append(found, fmt.Sprintf("%d boilerplate", texts))
	}
	if len(found) == 0 {
		return "nothing"
	}
	return strings.Join(found, ", ")
}

func outcomeOf(err error) string {
	switch {
	case err == nil:
		return OutcomeOK
	case errors.Is(err, context.Canceled):
		return OutcomeCanceled
	default:
		return string(card.CodeOf(err))
	}
}

// stageOf is a module's name, for one that has it.
func stageOf(module any, fallback string) string {
	if named, ok := module.(interface{ Name() string }); ok {
		return named.Name()
	}
	return fallback
}
