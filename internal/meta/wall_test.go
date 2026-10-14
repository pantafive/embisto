package meta_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/meta"
)

func page(t *testing.T, final, html string) card.Page {
	t.Helper()
	served, err := url.Parse(final)
	require.NoError(t, err)
	return meta.Parse(card.Response{URL: served, MediaType: "text/html", Body: []byte(html)})
}

func TestWall(t *testing.T) {
	tests := []struct {
		name, requested, final, html string
		want                         card.Limit
	}{
		{"redirected to a login page", "https://www.linkedin.com/company/x", "https://www.linkedin.com/authwall?trk=x", "<title>Sign Up | LinkedIn</title>", card.LimitLoginWall},
		{"redirected to a consent page", "https://www.youtube.com/watch?v=x", "https://consent.youtube.com/m?continue=x", "<title>Before you continue to YouTube</title>", card.LimitConsentWall},
		{"a post that shows only the site's name", "https://www.instagram.com/p/x/", "https://www.instagram.com/p/x/", "<title>Instagram</title>", card.LimitBlocked},
		{"the site's own home page", "https://www.youtube.com/", "https://www.youtube.com/", "<title>YouTube</title>", ""},
		{"a post titled with the site's name but described", "https://example.com/p/1", "https://example.com/p/1", `<title>Example</title><meta name="description" content="A post.">`, ""},
		{"a login page asked for", "https://example.com/login", "https://example.com/login", "<title>Example</title>", ""},
		{"an ordinary page", "https://example.com/post", "https://example.com/post", "<title>A post</title>", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requested, err := url.Parse(tt.requested)
			require.NoError(t, err)

			assert.Equal(t, tt.want, meta.Wall(requested, page(t, tt.final, tt.html)))
		})
	}
}
