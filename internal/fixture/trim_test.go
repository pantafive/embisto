package fixture_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pantafive/embisto/internal/fixture"
)

func TestTrimKeepsOnlyWhatTheSourcesRead(t *testing.T) {
	page := `<!doctype html><html><head><title>T</title><style>p{}</style><script>var x = "<meta>";</script>` +
		`<!-- note --><meta property="og:title" content="O"></head>` +
		`<body><div><p>Text</p><svg><title>S</title></svg></div><meta itemprop="name" content="N">` +
		`<script>track()</script><script type="application/ld+json">{"@type":"Article"}</script></body></html>`

	trimmed := fixture.Trim([]byte(page))

	assert.Equal(t, `<!doctype html><html><head><title>T</title>`+
		`<meta property="og:title" content="O"></head>`+
		`<body><meta itemprop="name" content="N"><script type="application/ld+json">{"@type":"Article"}</script></body></html>`,
		string(trimmed))
}

// TestTrimKeepsNothingOfVisitorsOrCommenters: saved pages once kept a visitor's payload and the handles, names and
// text of the people who commented on a video.
func TestTrimKeepsNothingOfVisitorsOrCommenters(t *testing.T) {
	page := `<html><head><meta name="visitor-payload" content="eyJ2aXNpdG9yX2lkIjoiNTA1In0="><meta name="visitor-hmac" content="e811bf">` +
		`<meta name="request-id" content="3B16:6167D"><script nonce="RqfN"></script><link rel="preload" href="/a.js?token=1">` +
		`<meta name="description" content="D"><link rel="icon" href="/favicon.ico">` +
		`<script type="application/ld+json">{"@type":"VideoObject","name":"V","author":{"name":"Channel"},` +
		`"comment":[{"@type":"Comment","author":{"name":"@commenter","url":"https://example.com/@commenter"},"text":"Hi"}],` +
		`"interactionStatistic":{"userInteractionCount":5}}</script></head></html>`

	trimmed := fixture.Trim([]byte(page))

	assert.Equal(t, `<html><head><meta name="description" content="D"><link rel="icon" href="/favicon.ico">`+
		`<script type="application/ld+json">{"@type":"VideoObject","author":{"name":"Channel"},"name":"V"}</script></head></html>`,
		string(trimmed))
}
