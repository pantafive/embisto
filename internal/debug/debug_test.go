package debug_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/internal/debug"
)

func TestPageShowsWhatAPageSaysWithoutRunningIt(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<title><script>alert(1)</script></title><meta property="og:description" content="&lt;img src=x onerror=alert(2)&gt;">`)
	}))
	t.Cleanup(site.Close)
	core := embisto.New(embisto.Options{AllowedNetworks: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}})
	server := httptest.NewServer(debug.Page{Core: core, Log: slog.New(slog.DiscardHandler)})
	t.Cleanup(server.Close)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/?url="+url.QueryEscape(site.URL+"/post"), nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)

	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, []any{http.StatusOK, false, false}, []any{
		resp.StatusCode, strings.Contains(string(body), "<script>alert(1)"), strings.Contains(string(body), "<img src=x"),
	})
}
