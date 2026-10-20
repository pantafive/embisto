package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/cache"
	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/iframely"
	"github.com/pantafive/embisto/internal/api"
	"github.com/pantafive/embisto/render"
)

var renderers = render.Defaults()

func newServer(t *testing.T, allowed ...netip.Prefix) *httptest.Server {
	t.Helper()
	opts := embisto.Defaults()
	opts.AllowedNetworks = allowed
	server := httptest.NewServer(api.NewServer(api.Deps{Previews: embisto.New(opts), Renderers: renderers}))
	t.Cleanup(server.Close)
	return server
}

// upstream serves the page at every path; its home page, at the root, says nothing the page says.
func upstream(t *testing.T, page string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/" {
			_, _ = io.WriteString(w, "<title>Home</title>")
			return
		}
		_, _ = io.WriteString(w, page)
	}))
	t.Cleanup(server.Close)
	return server
}

func get(t *testing.T, target string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func preview(t *testing.T, server *httptest.Server, target string) *http.Response {
	t.Helper()
	resp := get(t, server.URL+"/v1/preview?url="+url.QueryEscape(target))
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestPreviewAnswersWithTheCard(t *testing.T) {
	site := upstream(t, `<title>Hello</title><meta property="og:image" content="/cover.png">`)
	server := newServer(t, netip.MustParsePrefix("127.0.0.0/8"))

	resp := preview(t, server, site.URL+"/post")

	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got card.Preview
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, card.Preview{
		URL: site.URL + "/post", CanonicalURL: site.URL + "/post", Type: card.TypeLink, Title: "Hello",
		Icon:   card.Image{URL: site.URL + "/favicon.ico"},
		Image:  card.Image{URL: site.URL + "/cover.png"},
		Images: []card.Image{{URL: site.URL + "/cover.png"}},
		Links: []card.Link{
			{Href: site.URL + "/cover.png", Rel: []string{card.RelThumbnail, card.SourceOpenGraph}, Type: "image/png"},
			{Href: site.URL + "/favicon.ico", Rel: []string{card.RelIcon, card.SourceHTML}, Type: "image/x-icon"},
		},
		Sources: []string{card.SourceOpenGraph, card.SourceHTML},
	}, got)
}

// TestEveryRenderedAnswerIsConfined: the policy once came only with text/html, which a template's .xml escaped.
func TestEveryRenderedAnswerIsConfined(t *testing.T) {
	site := upstream(t, `<title>Hello</title>`)
	server := newServer(t, netip.MustParsePrefix("127.0.0.0/8"))
	for _, format := range []string{"json", "text", "html", "iframely", "iframely refusal"} {
		t.Run(format, func(t *testing.T) {
			path := "/v1/preview?format=" + format + "&url="
			switch format {
			case "iframely":
				path = "/iframely?url="
			case "iframely refusal":
				path = "/iframely?maxwidth=-1&url="
			}

			resp := get(t, server.URL+path+url.QueryEscape(site.URL+"/post"))

			assert.Equal(t, []string{"default-src 'none'", "nosniff"},
				[]string{strings.SplitN(resp.Header.Get("Content-Security-Policy"), ";", 2)[0], resp.Header.Get("X-Content-Type-Options")})
		})
	}
}

func TestPreviewFailsAsAProblemWithACode(t *testing.T) {
	site := upstream(t, "<title>Private</title>")
	tests := []struct {
		name, query  string
		status       int
		code, detail string
	}{
		{"missing url", "", http.StatusBadRequest, "invalid_url", "the url is empty"},
		{"not http", "?url=" + url.QueryEscape("gopher://example.com"), http.StatusBadRequest, "invalid_url", "the scheme is not http or https"},
		// The detail never names the address the host resolved to.
		{"private address", "?url=" + url.QueryEscape(site.URL), http.StatusForbidden, "blocked_url", "the URL resolves to an address that is not public"},
	}
	server := newServer(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := get(t, server.URL+"/v1/preview"+tt.query)

			var problem struct {
				Status int    `json:"status"`
				Code   string `json:"code"`
				Detail string `json:"detail"`
			}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&problem))
			assert.Equal(t, []any{tt.status, "application/problem+json", tt.status, tt.code, tt.detail},
				[]any{resp.StatusCode, resp.Header.Get("Content-Type"), problem.Status, problem.Code, problem.Detail})
		})
	}
}

func observedServer(t *testing.T, requests api.RequestObserver) *httptest.Server {
	t.Helper()
	opts := embisto.Defaults()
	opts.AllowedNetworks = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	server := httptest.NewServer(api.NewServer(api.Deps{Previews: embisto.New(opts), Renderers: renderers, Requests: requests}))
	t.Cleanup(server.Close)
	return server
}

type recordedRequests []string

func (r *recordedRequests) Observed(route string, status int, _ time.Duration) {
	*r = append(*r, fmt.Sprintf("%s %d", route, status))
}

func TestServerReportsRequestsByRouteNotByURL(t *testing.T) {
	site := upstream(t, "<title>Hello</title>")
	var observed recordedRequests
	server := observedServer(t, &observed)

	get(t, server.URL+"/v1/preview?url="+url.QueryEscape(site.URL))
	get(t, server.URL+"/nowhere")

	assert.Equal(t, recordedRequests{"/v1/preview 200", "unmatched 404"}, observed)
}

// leftBehind makes previews for a client that has gone: every one fails with the client's cancellation.
type leftBehind struct{}

func (leftBehind) Extract(context.Context, card.Request) (card.Result, error) {
	return card.Result{}, context.Canceled
}

func (leftBehind) Purge(context.Context, card.Request) (int, error) {
	return 0, nil
}

// TestAPreviewItsClientLeftIsCanceledNotInternal: a client that disconnected was answered and counted as a 500.
func TestAPreviewItsClientLeftIsCanceledNotInternal(t *testing.T) {
	var observed recordedRequests
	server := httptest.NewServer(api.NewServer(api.Deps{Previews: leftBehind{}, Renderers: renderers, Requests: &observed}))
	t.Cleanup(server.Close)

	resp := get(t, server.URL+"/v1/preview?url="+url.QueryEscape("https://example.com/post"))

	var problem struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&problem))
	assert.Equal(t, []any{http.StatusServiceUnavailable, "canceled"}, []any{resp.StatusCode, problem.Code})
	assert.Equal(t, recordedRequests{"/v1/preview 503"}, observed)
}

func TestCompatiblePathsAnswerInIframelysFormats(t *testing.T) {
	site := upstream(t, `<title>Hello</title><meta property="og:image" content="/cover.png">`)
	server := newServer(t, netip.MustParsePrefix("127.0.0.0/8"))
	target := url.QueryEscape(site.URL + "/post")

	compatible := get(t, server.URL+"/iframely?url="+target)
	oembed := get(t, server.URL+"/oembed?url="+target)

	var gotIframely iframely.Response
	require.NoError(t, json.NewDecoder(compatible.Body).Decode(&gotIframely))
	var gotOEmbed render.OEmbedResponse
	require.NoError(t, json.NewDecoder(oembed.Body).Decode(&gotOEmbed))
	assert.Equal(t, []any{"Hello", []string{"thumbnail", "og"}, "link", "Hello"},
		[]any{gotIframely.Meta.Title, gotIframely.Links["thumbnail"][0].Rel, gotOEmbed.Type, gotOEmbed.Title})
}

func TestCompatiblePathsFailInIframelysShape(t *testing.T) {
	missing := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(missing.Close)
	server := newServer(t, netip.MustParsePrefix("127.0.0.0/8"))

	resp := get(t, server.URL+"/iframely?url="+url.QueryEscape(missing.URL+"/gone"))

	var got iframely.Error
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, []any{http.StatusNotFound, iframely.Error{Error: iframely.ErrorBody{Source: "embisto", Code: 404, Message: "the page or the item does not exist, or was removed"}}},
		[]any{resp.StatusCode, got})
}

func TestSpecNamesThePublicURLAsItsServer(t *testing.T) {
	server := httptest.NewServer(api.NewServer(api.Deps{PublicURL: "https://api.example.com", Renderers: renderers}))
	t.Cleanup(server.Close)

	resp := get(t, server.URL+"/openapi.json")

	var spec struct {
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&spec))
	assert.Equal(t, "https://api.example.com", spec.Servers[0].URL)
}

func TestReferenceLoadsAPinnedScalarCheckedByHash(t *testing.T) {
	server := newServer(t)

	page := get(t, server.URL+"/docs")

	html, err := io.ReadAll(page.Body)
	require.NoError(t, err)
	assert.Regexp(t, `<script src="https://cdn\.jsdelivr\.net/npm/@scalar/api-reference@[0-9]+\.[0-9]+\.[0-9]+/[^"]+"\s+integrity="sha384-[^"]+"`, string(html))
}

func TestPreviewAsksThePageInTheRequestedLanguage(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<title>"+r.Header.Get("Accept-Language")+"</title>")
	}))
	t.Cleanup(site.Close)
	server := newServer(t, netip.MustParsePrefix("127.0.0.0/8"))
	titles := map[string]string{}

	for _, query := range []string{"", "&lang=de"} {
		var got card.Preview
		require.NoError(t, json.NewDecoder(get(t, server.URL+"/v1/preview?url="+url.QueryEscape(site.URL)+query).Body).Decode(&got))
		titles[query] = got.Title
	}

	assert.Equal(t, map[string]string{"": "en", "&lang=de": "de"}, titles)
}

func TestPreviewAnswersInTextWhenAsked(t *testing.T) {
	site := upstream(t, `<title>Hello</title><meta property="og:site_name" content="Example"><meta name="description" content="A page.">`)
	server := newServer(t, netip.MustParsePrefix("127.0.0.0/8"))

	resp := get(t, server.URL+"/v1/preview?format=text&url="+url.QueryEscape(site.URL+"/post"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, []any{"text/plain; charset=utf-8", "Hello — Example\nA page.\n" + site.URL + "/post\n"},
		[]any{resp.Header.Get("Content-Type"), string(body)})
}

// cachedServer is a site, the API previewing it through a cache, and the purge of that cache.
func cachedServer(t *testing.T) (*httptest.Server, *httptest.Server, *httptest.Server) {
	t.Helper()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Cache-Control", "max-age=600")
		if r.URL.Path == "/gone" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, "<title>Hello</title>")
	}))
	t.Cleanup(site.Close)
	opts := embisto.Defaults()
	opts.AllowedNetworks = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	previews := cache.New(embisto.New(opts), cache.NewMemory(1<<20, nil), cache.Options{TTL: time.Hour, MinTTL: time.Minute, MaxTTL: 24 * time.Hour, FailureTTL: time.Minute})
	server := httptest.NewServer(api.NewServer(api.Deps{Previews: previews, Renderers: renderers}))
	t.Cleanup(server.Close)
	purger := httptest.NewServer(api.NewPurger(api.Deps{Previews: previews}))
	t.Cleanup(purger.Close)
	return site, server, purger
}

func TestPreviewSaysHowLongItMayBeKept(t *testing.T) {
	site, server, _ := cachedServer(t)

	resp := get(t, server.URL+"/v1/preview?url="+url.QueryEscape(site.URL+"/post"))

	assert.Regexp(t, `^public, max-age=(599|600)$`, resp.Header.Get("Cache-Control"))
}

func TestARefusalServedFromTheCacheSaysHowLongItMayBeKept(t *testing.T) {
	site, server, _ := cachedServer(t)
	target := server.URL + "/v1/preview?url=" + url.QueryEscape(site.URL+"/gone")
	get(t, target)

	resp := get(t, target)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Regexp(t, `^public, max-age=(59|60)$`, resp.Header.Get("Cache-Control"))
}

func deleteStatus(t *testing.T, target string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, target, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestPurgeDropsWhatWasCached(t *testing.T) {
	site, server, purger := cachedServer(t)
	query := "/v1/preview?url=" + url.QueryEscape(site.URL+"/post")
	get(t, server.URL+query)

	first, second := deleteStatus(t, purger.URL+query), deleteStatus(t, purger.URL+query)

	assert.Equal(t, []int{http.StatusNoContent, http.StatusNotFound}, []int{first, second})
}

// TestTheAPIServesNoPurge: anyone who reaches the API could empty the cache and make every preview afresh.
func TestTheAPIServesNoPurge(t *testing.T) {
	site, server, _ := cachedServer(t)

	purged := deleteStatus(t, server.URL+"/v1/preview?url="+url.QueryEscape(site.URL+"/post"))
	spec := get(t, server.URL+"/openapi.json")

	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	require.NoError(t, json.NewDecoder(spec.Body).Decode(&document))
	_, listed := document.Paths["/v1/preview"]["delete"]
	assert.Equal(t, []any{http.StatusMethodNotAllowed, false}, []any{purged, listed})
}

func TestPreviewAnswersInATemplatesFormatAndListsIt(t *testing.T) {
	templates, err := render.LoadTemplates(fstest.MapFS{"note.md.tmpl": {Data: []byte(`**{{md .Title}}**`)}})
	require.NoError(t, err)
	site := upstream(t, "<title>Hello</title>")
	opts := embisto.Defaults()
	opts.AllowedNetworks = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	server := httptest.NewServer(api.NewServer(api.Deps{Previews: embisto.New(opts), Renderers: append(slices.Clone(renderers), templates...)}))
	t.Cleanup(server.Close)

	resp := get(t, server.URL+"/v1/preview?format=note&url="+url.QueryEscape(site.URL+"/post"))
	spec := get(t, server.URL+"/openapi.json")
	unknown := get(t, server.URL+"/v1/preview?format=nope&url="+url.QueryEscape(site.URL+"/post"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var document struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name   string `json:"name"`
				Schema struct {
					Enum []string `json:"enum"`
				} `json:"schema"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	require.NoError(t, json.NewDecoder(spec.Body).Decode(&document))
	var formats []string
	for _, parameter := range document.Paths["/v1/preview"]["get"].Parameters {
		if parameter.Name == "format" {
			formats = parameter.Schema.Enum
		}
	}
	var refused struct {
		Code   string `json:"code"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	require.NoError(t, json.NewDecoder(unknown.Body).Decode(&refused))
	require.NotEmpty(t, refused.Errors)
	assert.Equal(t, []any{"**Hello**", "text/markdown; charset=utf-8", true, http.StatusBadRequest, "unknown_format", true},
		[]any{string(body), resp.Header.Get("Content-Type"), slices.Contains(formats, "note"), unknown.StatusCode, refused.Code, strings.Contains(refused.Errors[0].Message, "note")})
}

func TestPreviewAnswers500WhenATemplateFailsOnARealCard(t *testing.T) {
	templates, err := render.LoadTemplates(fstest.MapFS{"picky.txt.tmpl": {Data: []byte(`start {{if eq .Title "Hello"}}{{index .Images 5}}{{end}}`)}})
	require.NoError(t, err)
	site := upstream(t, "<title>Hello</title>")
	opts := embisto.Defaults()
	opts.AllowedNetworks = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	server := httptest.NewServer(api.NewServer(api.Deps{Previews: embisto.New(opts), Renderers: append(slices.Clone(renderers), templates...)}))
	t.Cleanup(server.Close)

	resp := get(t, server.URL+"/v1/preview?format=picky&url="+url.QueryEscape(site.URL+"/post"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, []any{http.StatusInternalServerError, "application/problem+json", false, false},
		[]any{resp.StatusCode, resp.Header.Get("Content-Type"), strings.Contains(string(body), "start"), strings.Contains(string(body), "picky.txt.tmpl")})
}
