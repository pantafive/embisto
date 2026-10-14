package thumbs_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/thumbs"
)

var key = []byte(strings.Repeat("k", thumbs.MinKeyLength))

func signer(t *testing.T) *thumbs.Signer {
	t.Helper()
	signer, err := thumbs.NewSigner(key, thumbs.DefaultPresets)
	require.NoError(t, err)
	return signer
}

// segments are the signature, the size and the source of a proxy path.
func segments(t *testing.T, path string) (string, string, string) {
	t.Helper()
	parts := strings.Split(strings.TrimPrefix(path, "/i/"), "/")
	require.Len(t, parts, 3)
	return parts[0], parts[1], parts[2]
}

func TestVerifyReadsWhatTheSignerSigned(t *testing.T) {
	signature, size, source := segments(t, signer(t).Path("card", "https://example.com/a.jpg"))

	request, err := signer(t).Verify(signature, size, source)

	require.NoError(t, err)
	assert.Equal(t, []any{"https://example.com/a.jpg", thumbs.Box{Width: 600, Mode: thumbs.Fit}},
		[]any{request.Source.String(), request.Box})
}

func TestVerifyRefusesATamperedURL(t *testing.T) {
	signature, size, source := segments(t, signer(t).Path("card", "https://example.com/a.jpg"))
	_, _, other := segments(t, signer(t).Path("card", "https://example.com/b.jpg"))
	forged, err := thumbs.NewSigner([]byte(strings.Repeat("x", thumbs.MinKeyLength)), thumbs.DefaultPresets)
	require.NoError(t, err)
	forgedSignature, _, _ := segments(t, forged.Path("card", "https://example.com/a.jpg"))
	tests := []struct {
		name, signature, size, source string
	}{
		{"another source", signature, size, other},
		{"another size", signature, "large", source},
		{"another key", forgedSignature, size, source},
		{"no signature", "", size, source},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := signer(t).Verify(tt.signature, tt.size, tt.source)

			require.ErrorIs(t, err, thumbs.ErrSignature)
		})
	}
}

func TestVerifyTakesOnlyAllowedSizes(t *testing.T) {
	tests := []struct {
		size    string
		isValid bool
	}{
		{"thumb", true},
		{"320x0-fit", true},
		{"160x160-fill", true},
		{"321x0-fit", false},
		{"320x0-fill", false},
		{"0x0-fit", false},
		{"320x0-stretch", false},
		{"huge", false},
	}
	for _, tt := range tests {
		t.Run(tt.size, func(t *testing.T) {
			signature, size, source := segments(t, signer(t).Path(tt.size, "https://example.com/a.jpg"))

			_, err := signer(t).Verify(signature, size, source)

			assert.Equal(t, tt.isValid, err == nil, err)
		})
	}
}

func TestNewSignerRefusesAShortKey(t *testing.T) {
	_, err := thumbs.NewSigner([]byte("short"), thumbs.DefaultPresets)

	require.Error(t, err)
}

func TestEnricherGivesEachPictureItsVariants(t *testing.T) {
	enricher := thumbs.Enricher{Signer: signer(t), BaseURL: "https://api.example.com"}
	path := func(size, source string) string { return "https://api.example.com" + signer(t).Path(size, source) }

	enriched, err := enricher.Enrich(context.Background(), card.Preview{
		Icon:  card.Image{URL: "https://example.com/favicon.svg", Width: 32, Height: 32},
		Image: card.Image{URL: "https://example.com/cover.jpg", Width: 1280, Height: 720},
	}, nil)

	require.NoError(t, err)
	assert.Equal(t, card.Preview{
		Icon: card.Image{URL: "https://example.com/favicon.svg", Width: 32, Height: 32},
		Image: card.Image{URL: "https://example.com/cover.jpg", Width: 1280, Height: 720, Variants: []card.Variant{
			{Preset: "thumb", URL: path("thumb", "https://example.com/cover.jpg"), WebPURL: path("thumb.webp", "https://example.com/cover.jpg"), Width: 160, Height: 160},
			{Preset: "card", URL: path("card", "https://example.com/cover.jpg"), WebPURL: path("card.webp", "https://example.com/cover.jpg"), Width: 600, Height: 338},
			{Preset: "large", URL: path("large", "https://example.com/cover.jpg"), WebPURL: path("large.webp", "https://example.com/cover.jpg"), Width: 1200, Height: 675},
		}},
	}, enriched)
}

func TestVerifyReadsTheFormatSignedWithTheSize(t *testing.T) {
	signature, size, source := segments(t, signer(t).Path("card.webp", "https://example.com/a.jpg"))

	request, err := signer(t).Verify(signature, size, source)

	require.NoError(t, err)
	assert.Equal(t, thumbs.WebP, request.Format)
}

func TestVerifyRefusesAFormatChangedAfterSigning(t *testing.T) {
	signature, _, source := segments(t, signer(t).Path("card.webp", "https://example.com/a.jpg"))

	_, err := signer(t).Verify(signature, "card", source)

	require.ErrorIs(t, err, thumbs.ErrSignature)
}
