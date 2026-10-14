package golden_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/card"
)

// hostile serves one page for every URL, and no picture.
type hostile struct {
	page []byte
}

func (h hostile) Fetch(ctx context.Context, target *url.URL, _ string) (card.Response, error) {
	if err := ctx.Err(); err != nil {
		return card.Response{}, err
	}
	return card.Response{URL: target, MediaType: "text/html", Body: h.page}, nil
}

func (hostile) Peek(context.Context, *url.URL, int64) (card.Response, error) {
	return card.Response{}, &card.Error{Code: card.CodeFetchFailed, Err: errors.New("no picture")}
}

// TestAPageCannotMakeTheCardAsLargeAsItLikes is the review's page: 120,000 pictures and a description of megabytes,
// under the body limit, which once took a minute of CPU and made a card of megabytes.
func TestAPageCannotMakeTheCardAsLargeAsItLikes(t *testing.T) {
	var page strings.Builder
	page.WriteString(`<title>Evil</title><meta property="og:description" content="` + strings.Repeat("words ", 200_000) + `">`)
	for i := range 120_000 {
		page.WriteString(`<meta property=og:image content=/` + strconv.Itoa(i) + `>`)
	}
	opts := embisto.Defaults()
	opts.Fetcher, opts.HomePages = hostile{page: []byte(page.String())}, nil

	started := time.Now()
	result, err := embisto.New(opts).Extract(context.Background(), card.Request{URL: "https://evil.example/item"})
	took := time.Since(started)

	require.NoError(t, err)
	raw, err := json.Marshal(result.Preview)
	require.NoError(t, err)
	assert.Less(t, took, 3*time.Second)
	assert.LessOrEqual(t, len(result.Preview.Images), 16)
	assert.LessOrEqual(t, len(raw), 64<<10)
}

// slow is a source that finishes after the preview's deadline and ignores it.
type slow struct{}

func (slow) Read(context.Context, card.Input) (card.Facts, error) {
	time.Sleep(300 * time.Millisecond)
	return card.Facts{Source: "slow", Title: "Too late"}, nil
}

func TestNoCardIsMadePastTheDeadline(t *testing.T) {
	opts := embisto.Defaults()
	opts.Fetcher, opts.HomePages, opts.Timeout = hostile{page: []byte(`<title>A page</title>`)}, nil, 100*time.Millisecond
	opts.Sources = append(opts.Sources, slow{})

	_, err := embisto.New(opts).Extract(context.Background(), card.Request{URL: "https://example.com/post"})

	assert.Equal(t, card.CodeTimeout, card.CodeOf(err))
}
