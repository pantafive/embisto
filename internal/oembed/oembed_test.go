package oembed_test

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/oembed"
	"github.com/pantafive/embisto/internal/page"
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
		{"a listed endpoint without its trailing slash", "https://www.flickr.com/services/oembed?url=x", "", "application/json", true},
		{"a listed endpoint with its trailing slash", "https://www.flickr.com/services/oembed/?url=x", "", "application/json", true},
		{"a listed endpoint's format filled in", "https://vimeo.com/api/oembed.json?url=x", "", "application/json", true},
		{"a listed endpoint with a format extension", "https://codepen.io/api/oembed.json?url=x", "", "application/json", true},
		{"a path beneath a listed endpoint", "https://codepen.io/api/oembed/json?url=x", "", "application/json", true},
		{"a path that only starts like a listed endpoint", "https://codepen.io/api/oembedX/anything.json", "", "application/json", false},
		{"escaped dot segments out of a listed endpoint", "https://codepen.io/api/oembed/%2e%2e/%2E%2e/someone/x.json", "", "application/json", false},
		{"dot segments out of a listed endpoint", "https://codepen.io/api/oembed/../../someone/x.json", "", "application/json", false},
		{"a dot segment with a parameter", "https://codepen.io/api/oembed/..;/someone/x.json", "", "application/json", false},
		{"an escaped slash", "https://codepen.io/api/oembed%2F..%2Fsomeone", "", "application/json", false},
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

// silent never answers, until the caller gives up.
type silent struct{}

func (silent) Fetch(ctx context.Context, _ *url.URL, _ string) (card.Response, error) {
	<-ctx.Done()
	return card.Response{}, ctx.Err()
}

func (silent) Peek(ctx context.Context, _ *url.URL, _ int64) (card.Response, error) {
	<-ctx.Done()
	return card.Response{}, ctx.Err()
}

// TestDiscoveryGivesUpOnAnEndpointThatDoesNotAnswer: discovery had no time of its own, so an endpoint that hung took
// the whole preview's.
func TestDiscoveryGivesUpOnAnEndpointThatDoesNotAnswer(t *testing.T) {
	target := &url.URL{Scheme: "https", Host: "blog.example", Path: "/post"}
	parsed := page.Parse(card.Response{
		URL: target, MediaType: "text/html", Body: []byte(`<link rel="alternate" type="application/json+oembed" href="/oembed">`),
	})

	_, err := oembed.Discovery{Providers: oembed.Known(), Timeout: 50 * time.Millisecond}.Read(context.Background(), card.Input{URL: target, Page: parsed, Fetcher: silent{}})

	require.ErrorIs(t, err, context.DeadlineExceeded)
}
