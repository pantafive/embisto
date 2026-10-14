package oembed_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/oembed"
)

// answering answers every request as if it was served from final, as the fetcher reports a redirected response.
type answering struct {
	final     string
	mediaType string
}

func (a answering) Fetch(_ context.Context, target *url.URL, _ string) (card.Response, error) {
	final := target
	if a.final != "" {
		var err error
		if final, err = url.Parse(a.final); err != nil {
			return card.Response{}, err
		}
	}
	body := `{"type":"rich","title":"A post","html":"<script src=\"https://cdn.example/widget.js\"></script>","width":500,"height":300}`
	return card.Response{URL: final, MediaType: a.mediaType, Body: []byte(body)}, nil
}

func (answering) Peek(context.Context, *url.URL, int64) (card.Response, error) {
	return card.Response{}, nil
}

func TestEmbedHTMLIsKeptOnlyFromATrustedEndpoint(t *testing.T) {
	tests := []struct {
		name, endpoint, final, mediaType string
		isKept                           bool
	}{
		{"a listed https endpoint answering JSON", "https://www.youtube.com/oembed?url=x", "", "application/json", true},
		{"the oEmbed media type", "https://www.youtube.com/oembed?url=x", "", oembed.MediaType, true},
		{"a listed host redirecting to an unlisted one", "https://www.youtube.com/oembed?url=x", "https://attacker.example/oembed", "application/json", false},
		{"a listed host redirecting outside its endpoint", "https://www.youtube.com/oembed?url=x", "https://www.youtube.com/redirect?q=1", "application/json", false},
		{"a listed http endpoint", "http://www.23hq.com/23/oembed?url=x", "", "application/json", false},
		{"a listed https endpoint redirected to http", "https://www.youtube.com/oembed?url=x", "http://www.youtube.com/oembed?url=x", "application/json", false},
		{"a listed host on another port", "https://www.youtube.com/oembed?url=x", "https://www.youtube.com:8443/oembed?url=x", "application/json", false},
		{"an answer that is not JSON", "https://www.youtube.com/oembed?url=x", "", "text/html", false},
		{"an unlisted endpoint", "https://attacker.example/oembed?url=x", "", "application/json", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, err := url.Parse(tt.endpoint)
			require.NoError(t, err)

			facts, err := oembed.Get(context.Background(), answering{final: tt.final, mediaType: tt.mediaType}, endpoint, oembed.Known().Endpoints()...)

			require.NoError(t, err)
			assert.Equal(t, []any{"A post", tt.isKept}, []any{facts.Title, facts.Embed.HTML != ""})
		})
	}
}
