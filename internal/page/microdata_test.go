package page_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/page"
)

// TestMicrodataInTheBodyIsNotThePagesOwn: a comment's author and date, marked up in the body, were read as the page's.
func TestMicrodataInTheBodyIsNotThePagesOwn(t *testing.T) {
	page := page.Parse(card.Response{URL: &url.URL{Scheme: "https", Host: "example.com", Path: "/post"}, MediaType: "text/html", Body: []byte(`
<head><meta itemprop="name" content="The post"></head>
<body><div itemscope itemtype="https://schema.org/Comment">
<meta itemprop="author" content="A commenter"><meta itemprop="datePublished" content="2020-01-01">
<meta itemprop="description" content="A comment"></div></body>`)})

	assert.Equal(t, []string{"The post", "", "", ""}, []string{page.Value("name"), page.Value("author"), page.Value("datepublished"), page.Value("description")})
}
