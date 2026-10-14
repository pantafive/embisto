package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/cache"
	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/api"
)

// scripted answers every page with its page function and every oEmbed endpoint with its endpoint function; a peek
// at a picture runs peek, when set.
type scripted struct {
	page, endpoint func(target *url.URL) (card.Response, error)
	peek           func()
}

func (s scripted) Fetch(_ context.Context, target *url.URL, _ string) (card.Response, error) {
	if strings.Contains(target.Path, "oembed") {
		return s.endpoint(target)
	}
	return s.page(target)
}

func (s scripted) Peek(context.Context, *url.URL, int64) (card.Response, error) {
	if s.peek != nil {
		s.peek()
	}
	return card.Response{}, &card.Error{Code: card.CodeFetchFailed, Err: errors.New("no pictures here")}
}

func failing(code card.Code, status int, text string) func(*url.URL) (card.Response, error) {
	return func(*url.URL) (card.Response, error) {
		return card.Response{}, &card.Error{Code: code, Status: status, Err: errors.New(text)}
	}
}

func consentWall(target *url.URL) (card.Response, error) {
	final, err := url.Parse("https://consent." + target.Hostname() + "/?continue=" + url.QueryEscape(target.String()))
	return card.Response{URL: final, MediaType: "text/html", Body: []byte("<title>Before you continue</title>")}, err
}

// panicking is a site provider with a bug.
type panicking struct{}

func (panicking) Name() string { return "panicking" }

func (panicking) Match(*url.URL) card.Match { return card.MatchedAlone }

func (panicking) Provide(context.Context, card.Input) (card.Facts, error) {
	var page *card.Page
	return card.Facts{Title: page.URL.String()}, nil
}

func servedThroughCache(t *testing.T, opts embisto.Options) *httptest.Server {
	t.Helper()
	previews := cache.New(embisto.New(opts), cache.NewMemory(1<<20, nil), cache.Options{TTL: time.Hour, FailureTTL: time.Minute})
	server := httptest.NewServer(api.NewServer(api.Deps{Previews: previews, Renderers: renderers}))
	t.Cleanup(server.Close)
	return server
}

// TestOneRequestCannotStopTheServer sends the URLs that once killed the process: a URL that only the oEmbed list
// knows, whose page fails or is a wall, so that the list's provider stands in without a page.
func TestOneRequestCannotStopTheServer(t *testing.T) {
	endpointDown := failing(card.CodeFetchFailed, http.StatusNotFound, "status 404")
	tests := []struct {
		name, url string
		page      func(*url.URL) (card.Response, error)
		status    int
	}{
		{"a wildcard scheme on a subdomain that does not resolve", "https://no-such-sub.assemblrworld.com/creation/1", failing(card.CodeFetchFailed, 0, "lookup no-such-sub.assemblrworld.com: no such host"), http.StatusBadGateway},
		{"a listed URL answering 403", "https://vimeo.com/76979871", failing(card.CodeFetchFailed, http.StatusForbidden, "status 403"), http.StatusBadGateway},
		{"a listed URL answering 503", "https://vimeo.com/76979871", failing(card.CodeFetchFailed, http.StatusServiceUnavailable, "status 503"), http.StatusBadGateway},
		{"a listed URL that times out", "https://vimeo.com/76979871", failing(card.CodeTimeout, 0, "deadline exceeded"), http.StatusGatewayTimeout},
		{"a listed URL redirected to a consent wall", "https://vimeo.com/76979871", consentWall, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := embisto.Defaults()
			opts.Fetcher = scripted{page: tt.page, endpoint: endpointDown}
			server := servedThroughCache(t, opts)

			resp := get(t, server.URL+"/v1/preview?url="+url.QueryEscape(tt.url))
			health := get(t, server.URL+"/healthz")

			assert.Equal(t, []int{tt.status, http.StatusOK}, []int{resp.StatusCode, health.StatusCode})
		})
	}
}

func TestTheListsProviderStandsInForAFailedPage(t *testing.T) {
	opts := embisto.Defaults()
	opts.Fetcher = scripted{
		page: failing(card.CodeFetchFailed, http.StatusForbidden, "status 403"),
		endpoint: func(target *url.URL) (card.Response, error) {
			return card.Response{URL: target, MediaType: "application/json", Body: []byte(`{"type":"video","title":"A film"}`)}, nil
		},
	}
	server := servedThroughCache(t, opts)

	resp := get(t, server.URL+"/v1/preview?url="+url.QueryEscape("https://vimeo.com/76979871"))

	var got card.Preview
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, []any{http.StatusOK, "A film"}, []any{resp.StatusCode, got.Title})
}

func TestAPanickingModuleFailsOnlyItsPreview(t *testing.T) {
	opts := embisto.Defaults()
	opts.Providers = append([]card.Provider{panicking{}}, opts.Providers...)
	server := servedThroughCache(t, opts)

	resp := get(t, server.URL+"/v1/preview?url="+url.QueryEscape("https://example.com/post"))
	health := get(t, server.URL+"/healthz")

	var refused struct {
		Detail string `json:"detail"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&refused))
	assert.Equal(t, []any{http.StatusInternalServerError, "application/problem+json", "the request failed", http.StatusOK},
		[]any{resp.StatusCode, resp.Header.Get("Content-Type"), refused.Detail, health.StatusCode})
}

func TestAPanicWhileMeasuringAPictureFailsOnlyTheMeasure(t *testing.T) {
	opts := embisto.Defaults()
	opts.Fetcher = scripted{
		page: func(target *url.URL) (card.Response, error) {
			body := `<title>A page</title><meta property="og:image" content="https://example.com/a.png">`
			return card.Response{URL: target, MediaType: "text/html", Body: []byte(body)}, nil
		},
		peek: func() { panic("a decoder's bug") },
	}
	server := servedThroughCache(t, opts)

	resp := get(t, server.URL+"/v1/preview?url="+url.QueryEscape("https://example.com/post"))
	health := get(t, server.URL+"/healthz")

	var got card.Preview
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, []any{http.StatusOK, "A page", 0, http.StatusOK}, []any{resp.StatusCode, got.Title, got.Image.Width, health.StatusCode})
}

// unresolvedPage fails as a page on a host whose name does not resolve.
func unresolvedPage(target *url.URL) (card.Response, error) {
	return card.Response{}, &card.Error{Code: card.CodeFetchFailed, Err: &url.Error{Op: "Get", URL: target.String(), Err: &net.DNSError{Err: "no such host", Name: target.Hostname(), IsNotFound: true}}}
}

func amtraker(answer string) func(*url.URL) (card.Response, error) {
	return func(target *url.URL) (card.Response, error) {
		return card.Response{URL: target, MediaType: "application/json", Body: []byte(answer)}, nil
	}
}

// deadHostPlayer is what the list's endpoint for a wildcard scheme answers about a made-up subdomain: its name and a
// player of the URL, nothing of an item.
const deadHostPlayer = `"provider_name":"Amtraker","html":"<iframe src=\"https://nonexistent-x.amtraker.com/trains/1?oembed\"></iframe>","width":464,"height":788`

func TestAMadeUpSubdomainIsAFetchFailureNotACard(t *testing.T) {
	tests := []struct {
		name, answer string
		status       int
		kind         card.Type
	}{
		{"the provider says nothing of an item", `{"type":"rich","title":"",` + deadHostPlayer + `}`, http.StatusBadGateway, ""},
		{"the provider names an item but plays the dead host", `{"type":"rich","title":"Train 1","thumbnail_url":"https://amtraker.com/t.png",` + deadHostPlayer + `}`, http.StatusOK, card.TypeLink},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := embisto.Defaults()
			opts.Fetcher = scripted{page: unresolvedPage, endpoint: amtraker(tt.answer)}
			server := servedThroughCache(t, opts)

			resp := get(t, server.URL+"/v1/preview?url="+url.QueryEscape("https://nonexistent-x.amtraker.com/trains/1"))

			var got card.Preview
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
			assert.Equal(t, []any{tt.status, false, tt.kind}, []any{resp.StatusCode, got.Embed.HTML != "", got.Type})
		})
	}
}
