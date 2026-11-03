package assemble_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/assemble"
)

func TestStandaloneProviderKeepsItsIconOrFallsBackToThePageOrigin(t *testing.T) {
	for _, tt := range []struct {
		name  string
		icons []card.Image
		want  string
	}{
		{"declared", []card.Image{{URL: "https://cdn.example.com/icon.png"}}, "https://cdn.example.com/icon.png"},
		{"missing", nil, "https://example.com/favicon.ico"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			preview := assemble.Traced(nil, "https://short.example/post", "https://example.com/post", []card.Facts{{Source: "provider", Title: "A post", CanonicalURL: "https://canonical.example/post", Icons: tt.icons}}, nil)
			assert.Equal(t, tt.want, preview.Icon.URL)
			if tt.icons == nil {
				assert.Contains(t, preview.Links, card.Link{Href: tt.want, Rel: []string{card.RelIcon}, Type: "image/x-icon"})
			}
		})
	}
}
