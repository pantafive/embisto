package meta_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/meta"
)

// TestMicrodataInTheBodyIsNotThePagesOwn: a comment's author and date, marked up in the body, were read as the page's.
func TestMicrodataInTheBodyIsNotThePagesOwn(t *testing.T) {
	page := meta.Parse(card.Response{URL: &url.URL{Scheme: "https", Host: "example.com", Path: "/post"}, MediaType: "text/html", Body: []byte(`
<head><meta itemprop="name" content="The post"></head>
<body><div itemscope itemtype="https://schema.org/Comment">
<meta itemprop="author" content="A commenter"><meta itemprop="datePublished" content="2020-01-01">
<meta itemprop="description" content="A comment"></div></body>`)})

	assert.Equal(t, []string{"The post", "", "", ""}, []string{page.Value("name"), page.Value("author"), page.Value("datepublished"), page.Value("description")})
}

// TestADateNoCardCanCarryIsDropped: a date its offset pushed past 9999 failed the JSON card with 500 and was never
// cached.
func TestADateNoCardCanCarryIsDropped(t *testing.T) {
	page := meta.Parse(card.Response{
		URL: &url.URL{Scheme: "https", Host: "example.com", Path: "/post"}, MediaType: "text/html",
		Body: []byte(`<meta property="article:published_time" content="9999-12-31T23:00:00-05:00">`),
	})

	facts, err := meta.OpenGraph{}.Read(context.Background(), card.Input{Page: page})

	require.NoError(t, err)
	assert.True(t, facts.PublishedAt.IsZero())
	_, err = json.Marshal(facts.PublishedAt)
	require.NoError(t, err)
}
