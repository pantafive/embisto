package homepage_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/cache"
	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/homepage"
	"github.com/pantafive/embisto/internal/meta"
)

const home = `<meta name="description" content="Everything about everything."><meta property="og:image" content="/logo.png">`

// site serves its home page at /, counting the times it was read.
func site(t *testing.T, homes *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/" {
			homes.Add(1)
			_, _ = io.WriteString(w, home)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func source() *homepage.Source {
	return homepage.New(cache.NewMemory(1<<20, nil), homepage.Options{TTL: time.Hour, FailureTTL: time.Minute, Timeout: time.Minute})
}

func input(t *testing.T, raw string) card.Input {
	t.Helper()
	page, err := url.Parse(raw)
	require.NoError(t, err)
	fetcher := embisto.NewFetcher(embisto.Options{AllowedNetworks: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}})
	return card.Input{URL: page, Page: meta.Parse(card.Response{URL: page, MediaType: "text/html", Body: []byte(home)}), Fetcher: fetcher}
}

func TestSourceOffersWhatTheHomePageSaysAsBoilerplate(t *testing.T) {
	var homes atomic.Int32
	server := site(t, &homes)

	facts, err := source().Extract(context.Background(), input(t, server.URL+"/post"))

	require.NoError(t, err)
	assert.Equal(t, card.Boilerplate{Texts: []string{"Everything about everything."}, Images: []string{server.URL + "/logo.png"}}, facts.Boilerplate)
}

func TestSourceReadsAHomePageOncePerSite(t *testing.T) {
	var homes atomic.Int32
	server := site(t, &homes)
	src := source()
	_, err := src.Extract(context.Background(), input(t, server.URL+"/one"))
	require.NoError(t, err)

	_, err = src.Extract(context.Background(), input(t, server.URL+"/two"))

	require.NoError(t, err)
	assert.Equal(t, int32(1), homes.Load())
}

func TestSourceLeavesTheHomePageItselfAlone(t *testing.T) {
	var homes atomic.Int32
	server := site(t, &homes)
	src := source()
	// Learns the site's home page, served at / for this test's site.
	_, err := src.Extract(context.Background(), input(t, server.URL+"/post"))
	require.NoError(t, err)
	tests := []string{"/", "/?utm_source=news&utm_medium=mail", "/index.html", "/?fbclid=abc"}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			facts, err := src.Extract(context.Background(), input(t, server.URL+path))

			require.NoError(t, err)
			assert.True(t, facts.IsEmpty())
		})
	}
}

func TestSourceTakesThePageTheRootRedirectsToForTheHomePage(t *testing.T) {
	var homes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/en/", http.StatusFound)
			return
		}
		if r.URL.Path == "/en/" {
			homes.Add(1)
		}
		_, _ = io.WriteString(w, home)
	}))
	t.Cleanup(server.Close)
	src := source()
	_, err := src.Extract(context.Background(), input(t, server.URL+"/post"))
	require.NoError(t, err)

	facts, err := src.Extract(context.Background(), input(t, server.URL+"/en/"))

	require.NoError(t, err)
	assert.Equal(t, []any{true, int32(1)}, []any{facts.IsEmpty(), homes.Load()})
}

func TestSourceReadsTheHomePageWithinTheCallersDeadline(t *testing.T) {
	// A home page that never answers.
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	in := input(t, server.URL+"/post")
	slow := homepage.New(cache.NewMemory(1<<20, nil), homepage.Options{TTL: time.Hour, FailureTTL: time.Minute, Timeout: time.Minute})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := slow.Extract(ctx, in)

	require.NoError(t, err)
	assert.Less(t, time.Since(started), time.Second)
}

func TestSourceKeepsNoMoreOfAHomePageThanOfAnyPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<meta name="description" content="`+strings.Repeat("word ", 100_000)+`">`)
	}))
	t.Cleanup(server.Close)

	facts, err := source().Extract(context.Background(), input(t, server.URL+"/post"))

	require.NoError(t, err)
	require.Len(t, facts.Boilerplate.Texts, 1)
	assert.LessOrEqual(t, utf8.RuneCountInString(facts.Boilerplate.Texts[0]), 4096)
}
