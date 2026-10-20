package fixture_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/fixture"
	"github.com/pantafive/embisto/internal/imagesize"
)

// pictures answers every peek with the picture its URL names.
type pictures map[string][]byte

func (p pictures) Fetch(context.Context, *url.URL, string) (card.Response, error) {
	return card.Response{}, &card.Error{Code: card.CodeFetchFailed}
}

func (p pictures) Peek(_ context.Context, target *url.URL, _ int64) (card.Response, error) {
	return card.Response{URL: target, MediaType: "image/x-icon", Body: p[target.String()]}, nil
}

func encodedPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height))))
	return buf.Bytes()
}

func TestAPeekIsSavedAsTheSizeOfItsPicture(t *testing.T) {
	// An icon of two pictures, 32 by 32 and 256 by 32, whose width of 256 is written as 0.
	ico := make([]byte, 6+2*16)
	copy(ico, []byte{0, 0, 1, 0, 2, 0, 32, 32})
	ico[6+16+1] = 32
	recorded := pictures{
		"https://example.com/a.png": encodedPNG(t, 640, 360),
		"https://example.com/b.svg": []byte(`<svg viewBox="0 0 24 16"></svg>`),
		"https://example.com/c.ico": ico,
		"https://example.com/d":     []byte("not a picture"),
	}
	recorder := fixture.NewRecorder(recorded)
	for href := range recorded {
		target, err := url.Parse(href)
		require.NoError(t, err)
		_, err = recorder.Peek(context.Background(), target, 1<<16)
		require.NoError(t, err)
	}
	dir := t.TempDir()

	require.NoError(t, recorder.Save(dir, card.Request{URL: "https://example.com/"}, ""))

	saved, replay, err := fixture.Load(dir)
	require.NoError(t, err)
	require.Len(t, saved.Exchanges, len(recorded))
	sizes := map[string][2]int{}
	for _, exchange := range saved.Exchanges {
		assert.Empty(t, exchange.File, exchange.URL)
		assert.Equal(t, "image/x-icon", exchange.MediaType, exchange.URL)
		target, err := url.Parse(exchange.URL)
		require.NoError(t, err)
		resp, err := replay.Peek(context.Background(), target, 1<<16)
		require.NoError(t, err)
		width, height, _ := imagesize.Size(resp.Body)
		sizes[exchange.URL] = [2]int{width, height}
	}
	assert.Equal(t, map[string][2]int{
		"https://example.com/a.png": {640, 360},
		"https://example.com/b.svg": {24, 16},
		"https://example.com/c.ico": {256, 32},
		"https://example.com/d":     {0, 0},
	}, sizes)
}
