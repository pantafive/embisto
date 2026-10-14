package api_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/gen2brain/vpx/webp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/internal/api"
	"github.com/pantafive/embisto/thumbnails"
)

func opaquePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			picture.Set(x, y, color.RGBA{B: 200, A: 255})
		}
	}
	var out bytes.Buffer
	require.NoError(t, png.Encode(&out, picture))
	return out.Bytes()
}

// imageServer serves pictures and the /i/ handler that resizes them, signed with one key.
func imageServer(t *testing.T, files map[string][]byte) (*httptest.Server, *httptest.Server, *thumbnails.Signer) {
	t.Helper()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(files[r.URL.Path])
	}))
	t.Cleanup(origin.Close)
	signer, err := thumbnails.NewSigner([]byte(strings.Repeat("k", thumbnails.MinKeyLength)), thumbnails.DefaultPresets)
	require.NoError(t, err)
	fetcher := embisto.NewFetcher(embisto.Options{AllowedNetworks: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}})
	server := httptest.NewServer(api.NewServer(api.Deps{Renderers: renderers, Images: &api.Images{
		Signer:    signer,
		Processor: thumbnails.NewBuiltin(fetcher, thumbnails.Limits{MaxBytes: 1 << 20, MaxPixels: 1_000_000, MaxMemory: 64 << 20, Concurrency: 2, Wait: time.Second}),
	}}))
	t.Cleanup(server.Close)
	return origin, server, signer
}

func TestImageServesTheResizedPictureForGood(t *testing.T) {
	origin, server, signer := imageServer(t, map[string][]byte{"/cover.png": opaquePNG(t, 1280, 720)})

	resp := get(t, server.URL+signer.Path("card", origin.URL+"/cover.png"))

	require.Equal(t, http.StatusOK, resp.StatusCode)
	config, err := jpeg.DecodeConfig(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, []any{"image/jpeg", "public, max-age=31536000, immutable", "nosniff", true, 600, 338},
		[]any{
			resp.Header.Get("Content-Type"), resp.Header.Get("Cache-Control"), resp.Header.Get("X-Content-Type-Options"),
			resp.Header.Get("ETag") != "", config.Width, config.Height,
		})
}

func TestImageAnswersACurrentCopyWithNotModified(t *testing.T) {
	origin, server, signer := imageServer(t, map[string][]byte{"/cover.png": opaquePNG(t, 64, 64)})
	address := server.URL + signer.Path("thumb", origin.URL+"/cover.png")
	tag := get(t, address).Header.Get("ETag")
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, address, nil)
	require.NoError(t, err)
	req.Header.Set("If-None-Match", tag)

	resp, err := http.DefaultClient.Do(req)

	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotModified, resp.StatusCode)
}

// hugePNG is a small PNG whose header claims a size far over the limit; /i/ must refuse it from the header.
func hugePNG(t *testing.T) []byte {
	t.Helper()
	file := opaquePNG(t, 4, 4)
	const ihdr = 8
	binary.BigEndian.PutUint32(file[ihdr+8:], 50000)
	binary.BigEndian.PutUint32(file[ihdr+12:], 50000)
	binary.BigEndian.PutUint32(file[ihdr+8+13:], crc32.ChecksumIEEE(file[ihdr+4:ihdr+8+13]))
	return file
}

func TestImageRefusals(t *testing.T) {
	files := map[string][]byte{
		"/cover.png": opaquePNG(t, 64, 64), "/huge.png": hugePNG(t), "/logo.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
	}
	origin, server, signer := imageServer(t, files)
	valid := signer.Path("card", origin.URL+"/cover.png")
	segments := strings.Split(valid, "/")
	tests := []struct {
		name, path string
		status     int
		code       string
	}{
		{"tampered signature", "/i/AAAA" + segments[2][4:] + "/" + segments[3] + "/" + segments[4], http.StatusForbidden, "invalid_signature"},
		{"size changed after signing", "/i/" + segments[2] + "/large/" + segments[4], http.StatusForbidden, "invalid_signature"},
		{"unlisted size, signed", signer.Path("321x0-fit", origin.URL+"/cover.png"), http.StatusBadRequest, "invalid_options"},
		{"too many pixels", signer.Path("card", origin.URL+"/huge.png"), http.StatusRequestEntityTooLarge, "too_many_pixels"},
		{"svg is never served", signer.Path("card", origin.URL+"/logo.svg"), http.StatusUnsupportedMediaType, "unsupported_image"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := get(t, server.URL+tt.path)

			var problem struct {
				Code string `json:"code"`
			}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&problem))
			assert.Equal(t, []any{tt.status, tt.code, "no-store"}, []any{resp.StatusCode, problem.Code, resp.Header.Get("Cache-Control")})
		})
	}
}

func TestImageServesWebPWhenTheSizeSaysSo(t *testing.T) {
	origin, server, signer := imageServer(t, map[string][]byte{"/cover.png": opaquePNG(t, 1280, 720)})

	resp := get(t, server.URL+signer.Path("card.webp", origin.URL+"/cover.png"))

	require.Equal(t, http.StatusOK, resp.StatusCode)
	decoded, err := webp.Decode(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, []any{"image/webp", 600, 338}, []any{resp.Header.Get("Content-Type"), decoded.Bounds().Dx(), decoded.Bounds().Dy()})
}
