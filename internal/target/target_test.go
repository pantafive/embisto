package target_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/target"
)

func TestParseTakesAURLWithoutASchemeAsHTTPS(t *testing.T) {
	tests := []struct {
		raw, want string
		code      card.Code
	}{
		{"example.com/post", "https://example.com/post", ""},
		{"example.com/share?u=https://other.test/a", "https://example.com/share?u=https://other.test/a", ""},
		{"example.com:8443/post", "https://example.com:8443/post", ""},
		{"HTTP://Example.com/Post#top", "http://example.com/Post", ""},
		{"gopher://example.com/", "", card.CodeInvalidURL},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			parsed, err := target.Parse(tt.raw)

			got := ""
			if parsed != nil {
				got = parsed.String()
			}
			assert.Equal(t, []any{tt.want, tt.code}, []any{got, card.CodeOf(err)})
		})
	}
}
