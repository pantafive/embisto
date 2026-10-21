package cache_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/cache"
	"github.com/pantafive/embisto/card"
)

// counted answers every request after release closes, counting the requests that reached it. Its origin says the
// card may be kept ten minutes.
type counted struct {
	calls   atomic.Int32
	purges  atomic.Int32
	release chan struct{}
	err     error
}

func (c *counted) Extract(_ context.Context, req card.Request) (card.Result, error) {
	c.calls.Add(1)
	<-c.release
	if c.err != nil {
		return card.Result{}, c.err
	}
	return card.Result{Preview: card.Preview{URL: req.URL, Type: card.TypeLink, Title: "cached"}, Freshness: card.Freshness{MaxAge: 10 * time.Minute, Known: true}}, nil
}

func (c *counted) Purge(context.Context, card.Request) (int, error) {
	c.purges.Add(1)
	return 0, nil
}

func released(err error) *counted {
	c := &counted{release: make(chan struct{}), err: err}
	close(c.release)
	return c
}

var now = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

// options keep a preview at least five minutes and at most a day, an hour when the origin says nothing.
var options = cache.Options{
	TTL: time.Hour, MinTTL: 5 * time.Minute, MaxTTL: 24 * time.Hour, FailureTTL: time.Minute, Now: func() time.Time { return now },
}

func memory() *cache.Memory {
	return cache.NewMemory(1<<20, func() time.Time { return now })
}

// recorder hears what a layer does.
type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) LookedUp(result string)  { r.add("looked up " + result) }
func (r *recorder) Coalesced(scope string)  { r.add("coalesced " + scope) }
func (r *recorder) Locked(outcome string)   { r.add("locked " + outcome) }
func (r *recorder) TTLChosen(source string) { r.add("ttl " + source) }

func (r *recorder) add(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func TestLayerServesARepeatedRequestFromTheStore(t *testing.T) {
	next := released(nil)
	previews := cache.New(next, memory(), options)
	req := card.Request{URL: "https://example.com"}
	_, err := previews.Extract(context.Background(), req)
	require.NoError(t, err)

	result, err := previews.Extract(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, []any{card.Preview{URL: req.URL, Type: card.TypeLink, Title: "cached"}, now.Add(10 * time.Minute), int32(1)},
		[]any{result.Preview, result.Expires, next.calls.Load()})
}

func TestLayerServesARepeatedFailureWithItsExpiry(t *testing.T) {
	next := released(&card.Error{Code: card.CodeTimeout, Err: errors.New("slow")})
	previews := cache.New(next, memory(), options)
	req := card.Request{URL: "https://example.com"}
	_, err := previews.Extract(context.Background(), req)
	require.Error(t, err)

	result, err := previews.Extract(context.Background(), req)

	assert.Equal(t, []any{card.CodeTimeout, "the page did not answer in time", now.Add(time.Minute), int32(1)},
		[]any{card.CodeOf(err), err.Error(), result.Expires, next.calls.Load()})
}

func TestLayerKeepsAPreviewAsLongAsItsOriginAllows(t *testing.T) {
	tests := []struct {
		name      string
		freshness card.Freshness
		want      time.Duration
	}{
		{"what the origin says", card.Freshness{MaxAge: 10 * time.Minute, Known: true}, 10 * time.Minute},
		{"no shorter than the minimum", card.Freshness{Known: true}, 5 * time.Minute},
		{"no longer than the maximum", card.Freshness{MaxAge: 1000 * time.Hour, Known: true}, 24 * time.Hour},
		{"the default when it says nothing", card.Freshness{}, time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previews := cache.New(said(tt.freshness), memory(), options)

			result, err := previews.Extract(context.Background(), card.Request{URL: "https://example.com/post"})

			require.NoError(t, err)
			assert.Equal(t, now.Add(tt.want), result.Expires)
		})
	}
}

// said is an extractor whose origin said the freshness.
type said card.Freshness

func (s said) Extract(_ context.Context, req card.Request) (card.Result, error) {
	return card.Result{Preview: card.Preview{URL: req.URL, Title: "said"}, Freshness: card.Freshness(s)}, nil
}

func (said) Purge(context.Context, card.Request) (int, error) {
	return 0, nil
}

func TestLayerTreatsAnEntryPastItsExpiryAsStale(t *testing.T) {
	clock := now
	opts := options
	opts.Now = func() time.Time { return clock }
	observer := &recorder{}
	opts.Observer = observer
	previews := cache.New(released(nil), cache.NewMemory(1<<20, opts.Now), opts)
	req := card.Request{URL: "https://example.com/post"}
	_, err := previews.Extract(context.Background(), req)
	require.NoError(t, err)
	clock = clock.Add(10*time.Minute + time.Second)
	observer.events = nil

	_, err = previews.Extract(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, []string{"looked up stale", "ttl origin"}, observer.events)
}

func TestPurgeDropsEveryVariantOfAURLOrJustOne(t *testing.T) {
	next := released(nil)
	previews := cache.New(next, memory(), options)
	german := card.Request{URL: "https://example.com/post", Options: card.Options{Language: "de"}}
	for _, req := range []card.Request{{URL: "https://example.com/post"}, german, {URL: "example.com/post", Options: card.Options{Language: "fr"}}} {
		_, err := previews.Extract(context.Background(), req)
		require.NoError(t, err)
	}

	one, err := previews.Purge(context.Background(), card.Request{URL: "https://example.com/post", Options: card.Options{Language: "DE"}})
	require.NoError(t, err)
	rest, err := previews.Purge(context.Background(), card.Request{URL: "https://example.com/post#top"})
	require.NoError(t, err)
	none, err := previews.Purge(context.Background(), card.Request{URL: "https://example.com/post"})

	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 0, 3}, []int{one, rest, none, int(next.purges.Load())})
}

// panicky is an extractor with a bug, as a layer of a library user's own may have.
type panicky struct{}

func (panicky) Extract(context.Context, card.Request) (card.Result, error) {
	panic("a layer's bug")
}

func (panicky) Purge(context.Context, card.Request) (int, error) {
	return 0, nil
}

func TestLayerTurnsAPanicIntoThatPreviewsFailure(t *testing.T) {
	previews := cache.New(panicky{}, memory(), options)
	req := card.Request{URL: "https://example.com"}

	_, first := previews.Extract(context.Background(), req)
	_, again := previews.Extract(context.Background(), req)

	assert.Equal(t, []card.Code{card.CodeInternal, card.CodeInternal}, []card.Code{card.CodeOf(first), card.CodeOf(again)})
}

func TestStackedLayersKeepAPreviewNoLongerThanItsOriginAllows(t *testing.T) {
	clock := now
	opts := options
	opts.Now = func() time.Time { return clock }
	next := released(nil)
	inner := cache.New(next, cache.NewMemory(1<<20, opts.Now), opts)
	outer := cache.New(inner, cache.NewMemory(1<<20, opts.Now), opts)
	req := card.Request{URL: "https://example.com/post"}

	made, err := outer.Extract(context.Background(), req)
	require.NoError(t, err)
	clock = clock.Add(4 * time.Minute)
	// A second outer layer, empty, meets the inner one's entry four minutes old.
	later, err := cache.New(inner, cache.NewMemory(1<<20, opts.Now), opts).Extract(context.Background(), req)
	require.NoError(t, err)
	purged, err := outer.Purge(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, []any{now.Add(10 * time.Minute), 6 * time.Minute, now.Add(10 * time.Minute), 2, int32(1)},
		[]any{made.Expires, later.Freshness.MaxAge, later.Expires, purged, next.calls.Load()})
}

// homeSite serves a post and a home page, counting the times its home page is read.
func homeSite(t *testing.T, homes *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/" {
			homes.Add(1)
			_, _ = io.WriteString(w, `<meta name="description" content="Everything about everything.">`)
			return
		}
		_, _ = io.WriteString(w, `<title>A post</title><meta name="description" content="Everything about everything.">`)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestPurgeReachesWhatTheCoreRemembersOfTheSite(t *testing.T) {
	var homes atomic.Int32
	site := homeSite(t, &homes)
	core := embisto.Defaults()
	core.AllowedNetworks = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	previews := cache.New(embisto.New(core), memory(), options)
	req := card.Request{URL: site.URL + "/post"}
	_, err := previews.Extract(context.Background(), req)
	require.NoError(t, err)

	purged, err := previews.Purge(context.Background(), req)
	require.NoError(t, err)
	_, err = previews.Extract(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, []any{2, int32(2)}, []any{purged, homes.Load()})
}

// TestTheBoundsHoldForAPageThatSaysNothing: the default for a page without caching headers once escaped them.
func TestTheBoundsHoldForAPageThatSaysNothing(t *testing.T) {
	tests := []struct {
		name     string
		min, max time.Duration
		want     time.Duration
	}{
		{"a maximum below the default", 0, 30 * time.Minute, 30 * time.Minute},
		{"a minimum above the default", 2 * time.Hour, 0, 2 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := options
			opts.MinTTL, opts.MaxTTL = tt.min, tt.max
			previews := cache.New(said(card.Freshness{}), memory(), opts)

			result, err := previews.Extract(context.Background(), card.Request{URL: "https://example.com/post"})

			require.NoError(t, err)
			assert.Equal(t, now.Add(tt.want), result.Expires)
		})
	}
}
