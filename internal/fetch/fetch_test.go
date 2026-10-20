package fetch_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/fetch"
)

var loopback = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")}

func newClient(allowed []netip.Prefix) *fetch.Client {
	return fetch.New(fetch.Options{
		Timeout: time.Second, MaxBody: 1 << 10, MaxRedirects: 3, UserAgent: "embisto-test", Allowed: allowed,
	})
}

func get(t *testing.T, client *fetch.Client, raw string) (card.Response, error) {
	t.Helper()
	target, err := url.Parse(raw)
	require.NoError(t, err)
	return client.Fetch(context.Background(), target, "text/html")
}

func serve(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func TestGetDecodesTheCharsetTheMetaTagNames(t *testing.T) {
	server := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// "Привет" in windows-1251.
		_, _ = w.Write([]byte("<meta charset=\"windows-1251\"><title>\xcf\xf0\xe8\xe2\xe5\xf2</title>"))
	})

	page, err := get(t, newClient(loopback), server.URL)

	require.NoError(t, err)
	assert.Equal(t, `<meta charset="windows-1251"><title>Привет</title>`, string(page.Body))
}

// TestGetDecodesWhatTheFirstKilobyteDoesNotTell: the HTML standard's sniff reads the first kilobyte, and a body that
// is ASCII there was taken as windows-1252, turning JSON and late UTF-8 into mojibake.
func TestGetDecodesWhatTheFirstKilobyteDoesNotTell(t *testing.T) {
	head := strings.Repeat("<!-- padding -->", 100)
	tests := []struct {
		name, contentType, body, want string
	}{
		{"JSON without a charset is UTF-8", "application/json", `{"pad":"` + head + `","title":"Café"}`, `{"pad":"` + head + `","title":"Café"}`},
		{"HTML valid as UTF-8 after an ASCII head", "text/html", head + "<title>Café</title>", head + "<title>Café</title>"},
		{"HTML naming its charset late", "text/html", head + "<meta charset=\"windows-1251\"><title>\xcf\xf0\xe8\xe2\xe5\xf2</title>", head + `<meta charset="windows-1251"><title>Привет</title>`},
		{"the header's charset wins", "text/html; charset=windows-1251", head + "<title>\xcf\xf0\xe8\xe2\xe5\xf2</title>", head + "<title>Привет</title>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				_, _ = w.Write([]byte(tt.body))
			})
			client := fetch.New(fetch.Options{Timeout: time.Second, MaxBody: 1 << 20, MaxRedirects: 3, UserAgent: "embisto-test", Allowed: loopback})

			page, err := get(t, client, server.URL)

			require.NoError(t, err)
			assert.Equal(t, tt.want, string(page.Body))
		})
	}
}

func TestGetReportsTheURLTheLastRedirectServed(t *testing.T) {
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})

	page, err := get(t, newClient(loopback), server.URL)

	require.NoError(t, err)
	assert.Equal(t, card.Response{URL: mustParse(t, server.URL+"/final"), MediaType: "text/html", Body: []byte("ok")}, page)
}

func TestGetSkipsTheBodyOfMediaThatIsNotText(t *testing.T) {
	server := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(strings.Repeat("x", 4<<10)))
	})

	page, err := get(t, newClient(loopback), server.URL)

	require.NoError(t, err)
	assert.Equal(t, card.Response{URL: mustParse(t, server.URL), MediaType: "image/png"}, page)
}

// TestGetTellsTheMediaTypeOfAHeaderGoCannotRead: an unreadable Content-Type once made every page a bare file, with no
// title.
func TestGetTellsTheMediaTypeOfAHeaderGoCannotRead(t *testing.T) {
	const page = "<!doctype html><title>A page</title>"
	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 16)
	tests := []struct {
		name          string
		contentType   []string
		body          string
		wantMediaType string
		wantBody      string
	}{
		{"a parameter without a value", []string{"text/html;charset="}, page, "text/html", page},
		{"a parameter without an equals sign", []string{"text/html; charset:utf-8"}, page, "text/html", page},
		{"conflicting parameters", []string{"text/html; charset=utf-8; charset=latin1"}, page, "text/html", page},
		{"no Content-Type on a page", nil, page, "text/html", page},
		{"no Content-Type on a picture", nil, png, "image/png", ""},
		{"the oEmbed media type", []string{"application/json+oembed"}, `{"title":"A post"}`, "application/json+oembed", `{"title":"A post"}`},
		{"the oEmbed media type in XML", []string{"text/xml+oembed"}, "<oembed/>", "text/xml+oembed", "<oembed/>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				// A nil value keeps the server from sniffing one of its own.
				w.Header()["Content-Type"] = tt.contentType
				_, _ = w.Write([]byte(tt.body))
			})

			page, err := get(t, newClient(loopback), server.URL)

			require.NoError(t, err)
			assert.Equal(t, []string{tt.wantMediaType, tt.wantBody}, []string{page.MediaType, string(page.Body)})
		})
	}
}

func TestGetRefusesAddressesThatAreNotPublic(t *testing.T) {
	server := serve(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("secret")) })
	port := mustParse(t, server.URL).Port()
	tests := []struct {
		name, url string
	}{
		{"loopback by name", "http://localhost:" + port},
		{"loopback by address", server.URL},
		{"IPv4-mapped loopback", "http://[::ffff:127.0.0.1]:" + port},
		{"unspecified", "http://0.0.0.0:" + port},
		{"cloud metadata", "http://169.254.169.254/latest/meta-data/"},
		{"private network", "http://10.0.0.1/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := get(t, newClient(nil), tt.url)

			require.ErrorIs(t, err, fetch.ErrBlocked)
		})
	}
}

func TestGetRefusesARedirectToAnAddressThatIsNotPublic(t *testing.T) {
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	})

	_, err := get(t, newClient(loopback), server.URL)

	require.ErrorIs(t, err, fetch.ErrBlocked)
}

func TestGetRefusesARedirectToASchemeOtherThanHTTP(t *testing.T) {
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	})

	_, err := get(t, newClient(loopback), server.URL)

	require.ErrorIs(t, err, fetch.ErrBlocked)
}

func TestGetStopsFollowingAfterTheRedirectCap(t *testing.T) {
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path+"x", http.StatusFound)
	})

	_, err := get(t, newClient(loopback), server.URL)

	require.ErrorIs(t, err, fetch.ErrFailed)
}

func TestGetRefusesABodyOverTheLimit(t *testing.T) {
	server := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(strings.Repeat("x", 1<<10+1)))
	})

	_, err := get(t, newClient(loopback), server.URL)

	require.ErrorIs(t, err, fetch.ErrTooLarge)
}

func TestGetGivesUpOnASlowServer(t *testing.T) {
	release := make(chan struct{})
	server := serve(t, func(http.ResponseWriter, *http.Request) { <-release })
	defer close(release)
	client := fetch.New(fetch.Options{Timeout: 50 * time.Millisecond, MaxBody: 1 << 10, Allowed: loopback})

	_, err := get(t, client, server.URL)

	require.ErrorIs(t, err, fetch.ErrTimeout)
}

func TestGetFailsOnAnErrorStatusAndKeepsIt(t *testing.T) {
	server := serve(t, http.NotFound)

	_, err := get(t, newClient(loopback), server.URL)

	var failed *card.Error
	require.ErrorAs(t, err, &failed)
	assert.Equal(t, []any{card.CodeFetchFailed, http.StatusNotFound}, []any{failed.Code, failed.Status})
}

func TestIsPublic(t *testing.T) {
	tests := []struct {
		addr     string
		isPublic bool
	}{
		{"8.8.8.8", true},
		{"2606:4700:4700::1111", true},
		{"127.0.0.1", false},
		{"::1", false},
		{"::ffff:10.0.0.1", false},
		{"172.16.0.1", false},
		{"192.168.1.1", false},
		{"169.254.169.254", false},
		{"100.100.100.200", false},
		{"0.0.0.0", false},
		{"224.0.0.1", false},
		{"fd00:ec2::254", false},
		{"fe80::1", false},
		{"64:ff9b::a00:1", false},
		{"2002:a00:1::", false},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			assert.Equal(t, tt.isPublic, fetch.IsPublic(netip.MustParseAddr(tt.addr)))
		})
	}
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed
}

func TestPeekReadsOnlyTheFirstBytesOfAnyMedia(t *testing.T) {
	server := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(strings.Repeat("x", 4<<10)))
	})
	target := mustParse(t, server.URL)

	resp, err := newClient(loopback).Peek(context.Background(), target, 16)

	require.NoError(t, err)
	assert.Equal(t, card.Response{URL: target, MediaType: "image/png", Body: []byte(strings.Repeat("x", 16))}, resp)
}

func TestFetchAsksInTheContextsLanguage(t *testing.T) {
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(r.Header.Get("Accept-Language")))
	})

	resp, err := newClient(loopback).Fetch(card.WithLanguage(context.Background(), "de"), mustParse(t, server.URL), "text/plain")

	require.NoError(t, err)
	assert.Equal(t, "de", string(resp.Body))
}
