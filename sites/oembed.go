// Package sites holds the site providers: each knows one site, or a family of them, and outranks the generic sources
// there.
package sites

import (
	"context"
	"net/url"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/oembed"
)

// OEmbed covers every site of the oembed.com provider list without code of its own: the list is data, and a page
// that did not link to its oEmbed endpoint is asked about at the endpoint the list names for its URL.
type OEmbed struct{}

func (OEmbed) Name() string {
	return card.SourceOEmbed
}

func (OEmbed) Match(u *url.URL) card.Match {
	if _, ok := oembed.Known().Endpoint(u); ok {
		return card.MatchedWithPage
	}
	return card.Unmatched
}

func (OEmbed) Provide(ctx context.Context, in card.Input) (card.Facts, error) {
	for _, facts := range in.Facts {
		if facts.Source == card.SourceOEmbed && !facts.IsEmpty() {
			return card.Facts{}, nil
		}
	}
	// Standing in for a page that failed or was a wall, there is no page: the requested URL is asked about.
	target := in.URL
	if in.Page.URL != nil {
		target = in.Page.URL
	}
	base, ok := oembed.Known().Endpoint(target)
	if !ok {
		return card.Facts{}, nil
	}
	endpoint, err := oembed.Endpoint(base, target, in.Options)
	if err != nil {
		return card.Facts{}, err //nolint:wrapcheck // it names the endpoint
	}
	return oembed.Get(ctx, in.Fetcher, endpoint, base) //nolint:wrapcheck // it says which step failed
}
