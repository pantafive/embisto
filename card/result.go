package card

import (
	"context"
	"time"
)

// Extractor makes the preview of a URL and forgets what it remembers of one. The core and every layer around it,
// such as a cache, are one, so layers stack in any order.
type Extractor interface {
	// Extract makes the preview of the request's URL. It fails with an *Error, or with the context's error when the
	// caller gave up; a failure a cache served still carries the result's Expires.
	Extract(ctx context.Context, req Request) (Result, error)
	// Purge drops what the request's Extract would be served from memory: with only a URL, every variant of it; with
	// options, the one variant. It says how many entries it dropped, across the layers.
	Purge(ctx context.Context, req Request) (int, error)
}

// Result is a preview with what the layers around the core need to know of it.
type Result struct {
	Preview Preview
	// Freshness is how long the responses the card was made from may still be kept, as their origins said.
	Freshness Freshness
	// Expires is when the cache that served the result lets it go; zero when no cache did.
	Expires time.Time
	// Provider is the site provider that shaped the card; empty for one the generic sources made.
	Provider string
}
