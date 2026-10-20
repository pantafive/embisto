package sources_test

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
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

// TestJSONLDSkipsBlocksTooLargeToParse: a block parses to some twenty-five times its size, so a page of a few
// megabytes of JSON-LD took over a hundred megabytes per preview. An oversize block is skipped, and so is one past
// what a page's blocks may take together; the blocks within both are read.
func TestJSONLDSkipsBlocksTooLargeToParse(t *testing.T) {
	block := func(kind, headline string, size int) string {
		padding := strings.Repeat("x", size)
		return `<script type="application/ld+json">{"@type":"` + kind + `","headline":"` + headline + `","text":"` + padding + `"}</script>`
	}
	hostile := block("NewsArticle", "Too large", 600<<10) +
		block("Organization", "", 400<<10) + block("Organization", "", 400<<10) + block("NewsArticle", "Past the page", 400<<10) +
		block("NewsArticle", "Read", 0)
	parsed := page.Parse(card.Response{
		URL: &url.URL{Scheme: "https", Host: "example.com", Path: "/post"}, MediaType: "text/html", Body: []byte(hostile),
	})

	facts, err := sources.JSONLD{}.Read(context.Background(), card.Input{Page: parsed})

	require.NoError(t, err)
	assert.Equal(t, "Read", facts.Title)
}

// TestJSONLDDecodesTheEntitiesOfItsTextsAlone: WordPress writes them in a headline, and a URL's query may hold a key
// such as &reg that reads as one.
func TestJSONLDDecodesTheEntitiesOfItsTextsAlone(t *testing.T) {
	parsed := page.Parse(card.Response{
		URL: &url.URL{Scheme: "https", Host: "example.com", Path: "/post"}, MediaType: "text/html",
		Body: []byte(`<script type="application/ld+json">{"@type":"BlogPosting","headline":"WordPress 6.5 &#8220;Regina&#8221;","author":{"name":"Dan O&#039;Brien"},"image":{"url":"https://example.com/a.jpg?w=600&reg=eu"}}</script>`),
	})

	facts, err := sources.JSONLD{}.Read(context.Background(), card.Input{Page: parsed})

	require.NoError(t, err)
	require.Len(t, facts.Images, 1)
	assert.Equal(t, []string{"WordPress 6.5 “Regina”", "Dan O'Brien", "https://example.com/a.jpg?w=600&reg=eu"}, []string{facts.Title, facts.Author, facts.Images[0].URL})
}
