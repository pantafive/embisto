package imagesize_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/fetch"
	"github.com/pantafive/embisto/internal/imagesize"
)

func encoded(t *testing.T, encode func(*bytes.Buffer, image.Image) error) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, encode(&buf, image.NewRGBA(image.Rect(0, 0, 40, 30))))
	return buf.Bytes()
}

func entry(side byte) []byte {
	return append([]byte{side, side}, make([]byte, 14)...)
}

func TestEnrichMeasuresEveryPictureFormat(t *testing.T) {
	webp, err := base64.StdEncoding.DecodeString("UklGRiQAAABXRUJQVlA4IBgAAAAwAQCdASoBAAEAAwA0JaQAA3AA/vuUAAA=")
	require.NoError(t, err)
	files := map[string][]byte{
		"/a.png":  encoded(t, func(b *bytes.Buffer, i image.Image) error { return png.Encode(b, i) }),
		"/a.gif":  encoded(t, func(b *bytes.Buffer, i image.Image) error { return gif.Encode(b, i, nil) }),
		"/a.jpg":  encoded(t, func(b *bytes.Buffer, i image.Image) error { return jpeg.Encode(b, i, nil) }),
		"/a.webp": webp,
		// Two entries, 16 and 32 pixels square; the rest of each entry is offsets the size does not need.
		"/a.ico": append(append([]byte{0, 0, 1, 0, 2, 0}, entry(16)...), entry(32)...),
		"/a.svg": []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="120px" height="60"></svg>`),
		"/b.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100%" viewBox="0 0 24 12"></svg>`),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(files[r.URL.Path])
	}))
	t.Cleanup(server.Close)
	fetcher := fetch.New(fetch.Options{
		Timeout: time.Second, MaxBody: 1 << 20, MaxRedirects: 1, Allowed: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")},
	})
	images := []card.Image{
		{URL: server.URL + "/a.png"},
		{URL: server.URL + "/a.gif"},
		{URL: server.URL + "/a.jpg"},
		{URL: server.URL + "/a.webp"},
		{URL: server.URL + "/a.svg"},
		{URL: server.URL + "/b.svg"},
		{URL: server.URL + "/missing.png"},
	}
	enricher := imagesize.Enricher{Concurrency: 2, Timeout: time.Second, MaxBytes: 1 << 10, MaxImages: 10}

	enriched, err := enricher.Enrich(context.Background(), card.Preview{
		Icon: card.Image{URL: server.URL + "/a.ico"}, Images: images,
		Links: []card.Link{{Href: server.URL + "/a.png", Rel: []string{card.RelThumbnail}}},
	}, fetcher)

	require.NoError(t, err)
	assert.Equal(t, card.Preview{
		Icon: card.Image{URL: server.URL + "/a.ico", Width: 32, Height: 32},
		Images: []card.Image{
			{URL: server.URL + "/a.png", Width: 40, Height: 30},
			{URL: server.URL + "/a.gif", Width: 40, Height: 30},
			{URL: server.URL + "/a.jpg", Width: 40, Height: 30},
			{URL: server.URL + "/a.webp", Width: 1, Height: 1},
			{URL: server.URL + "/a.svg", Width: 120, Height: 60},
			{URL: server.URL + "/b.svg", Width: 24, Height: 12},
			{URL: server.URL + "/missing.png"},
		},
		Links: []card.Link{{Href: server.URL + "/a.png", Rel: []string{card.RelThumbnail}, Media: card.MediaOf(40, 30)}},
	}, enriched)
}
