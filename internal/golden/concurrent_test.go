package golden_test

import (
	"context"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/card"
)

// kept serves one page of many pictures, and answers every picture with a response the origin lets be kept.
type kept struct{}

func (kept) Fetch(_ context.Context, target *url.URL, _ string) (card.Response, error) {
	page := `<title>Pictures</title><link rel=icon href=/icon.png><meta property=og:image content=/a.png>` +
		`<meta property=og:image content=/b.png><meta property=og:image content=/c.png><meta property=og:image content=/d.png>`
	return card.Response{URL: target, MediaType: "text/html", Body: []byte(page), Freshness: card.Freshness{MaxAge: time.Hour, Known: true}}, nil
}

func (kept) Peek(_ context.Context, target *url.URL, _ int64) (card.Response, error) {
	return card.Response{URL: target, MediaType: "image/png", Freshness: card.Freshness{MaxAge: time.Minute, Known: true}}, nil
}

// TestPicturesMeasuredAtOnceKeepTheirFreshnessSafely: the picture measurer reads several pictures at once, and each
// response's freshness was written to one unguarded map, which ended the program.
func TestPicturesMeasuredAtOnceKeepTheirFreshnessSafely(t *testing.T) {
	opts := embisto.Defaults()
	opts.Fetcher, opts.HomePages = kept{}, nil
	core := embisto.New(opts)

	results, errs := make([]card.Result, 8), make([]error, 8)
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() {
			results[i], errs[i] = core.Extract(context.Background(), card.Request{URL: "https://pictures.example/item"})
		})
	}
	wg.Wait()

	for i, result := range results {
		require.NoError(t, errs[i])
		assert.Equal(t, time.Hour, result.Freshness.MaxAge)
	}
}
