package golden_test

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/card"
)

// shortener serves a short link as the page it redirects to, and the sites' oEmbed endpoints.
type shortener struct {
	to string
}

func (s shortener) Fetch(_ context.Context, target *url.URL, _ string) (card.Response, error) {
	switch {
	case strings.Contains(target.Path, "oembed"):
		answer := `{"type":"video","title":"From the provider","author_name":"Someone","provider_name":"Site","html":"<iframe src=\"https://player.example/1\"></iframe>","width":560,"height":315}`
		return card.Response{URL: target, MediaType: "application/json", Body: []byte(answer)}, nil
	case target.Host == "bit.ly" || target.Host == "t.co":
		served, err := url.Parse(s.to)
		return card.Response{URL: served, MediaType: "text/html", Body: []byte(`<title>Served page</title>`)}, err
	}
	return card.Response{}, &card.Error{Code: card.CodeFetchFailed, Err: errors.New("not served")}
}

func (shortener) Peek(context.Context, *url.URL, int64) (card.Response, error) {
	return card.Response{}, &card.Error{Code: card.CodeFetchFailed, Err: errors.New("no pictures")}
}

// TestAShortLinkGetsTheProviderOfWhereItLeads: a provider was matched on the address served but given the short
// link, and one that answers from the URL alone was never asked after a redirect.
func TestAShortLinkGetsTheProviderOfWhereItLeads(t *testing.T) {
	tests := []struct {
		short, to, provider string
	}{
		{"https://bit.ly/abc", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "youtube"},
		{"https://t.co/abc", "https://x.com/jack/status/20", "x"},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			opts := embisto.Defaults()
			opts.Fetcher, opts.HomePages = shortener{to: tt.to}, nil

			result, err := embisto.New(opts).Extract(context.Background(), card.Request{URL: tt.short})

			require.NoError(t, err)
			assert.Equal(t, []any{tt.provider, true}, []any{result.Provider, result.Preview.Embed.HTML != ""})
		})
	}
}
