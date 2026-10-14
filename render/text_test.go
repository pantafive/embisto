package render_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/render"
)

func TestTextTrimsALongDescriptionAtAWord(t *testing.T) {
	var out strings.Builder

	err := render.Text{}.Render(&out, card.Preview{
		URL: "https://example.com", Title: "A title", Description: strings.Repeat("word ", 60),
	})

	require.NoError(t, err)
	assert.Equal(t, "A title\n"+strings.TrimSpace(strings.Repeat("word ", 40))+"…\nhttps://example.com\n", out.String())
}
