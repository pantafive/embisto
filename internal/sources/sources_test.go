package sources_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/page"
	"github.com/pantafive/embisto/internal/sources"
)

// TestADateNoCardCanCarryIsDropped: a date its offset pushed past 9999 failed the JSON card with 500 and was never
// cached.
func TestADateNoCardCanCarryIsDropped(t *testing.T) {
	parsed := page.Parse(card.Response{
		URL: &url.URL{Scheme: "https", Host: "example.com", Path: "/post"}, MediaType: "text/html",
		Body: []byte(`<meta property="article:published_time" content="9999-12-31T23:00:00-05:00">`),
	})

	facts, err := sources.OpenGraph{}.Read(context.Background(), card.Input{Page: parsed})

	require.NoError(t, err)
	assert.True(t, facts.PublishedAt.IsZero())
	_, err = json.Marshal(facts.PublishedAt)
	require.NoError(t, err)
}
