package render_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/render"
)

func htmlOf(t *testing.T, preview card.Preview) string {
	t.Helper()
	var out strings.Builder
	require.NoError(t, render.HTML{}.Render(&out, preview))
	return out.String()
}

func TestHTMLCardEscapesWhatAPageSaysAndRunsNothing(t *testing.T) {
	out := htmlOf(t, card.Preview{
		URL: "javascript:alert(1)", Title: "<script>alert(1)</script>", Description: `"><img src=x onerror=alert(2)>`,
		Embed: card.Embed{HTML: "<script>alert(3)</script>"},
	})

	assert.Equal(t, []bool{false, false, false, true},
		[]bool{strings.Contains(out, "<script>alert"), strings.Contains(out, "<img src=x"), strings.Contains(out, "javascript:"), strings.Contains(out, "&lt;script&gt;alert(1)")})
}

func TestHTMLCardSizesThePictureAndOffersWebP(t *testing.T) {
	out := htmlOf(t, card.Preview{
		URL: "https://example.com/post", Title: "A post",
		Image: card.Image{URL: "https://example.com/cover.jpg", Width: 1280, Height: 720, Variants: []card.Variant{
			{Preset: "card", URL: "https://embisto.test/i/s/card/x", WebPURL: "https://embisto.test/i/s/card.webp/x", Width: 600, Height: 338},
		}},
	})

	assert.Equal(t, []bool{true, true, true}, []bool{
		strings.Contains(out, `<source type="image/webp" srcset="https://embisto.test/i/s/card.webp/x">`),
		strings.Contains(out, `src="https://embisto.test/i/s/card/x" alt="" width="600" height="338"`),
		strings.Contains(out, "aspect-ratio: 600 / 338"),
	})
}

func TestHTMLCardNamesAWalledOrEmptyCardBySite(t *testing.T) {
	walled := htmlOf(t, card.Preview{URL: "https://www.instagram.com/p/1/", SiteName: "Instagram", Limited: card.LimitLoginWall})
	empty := htmlOf(t, card.Preview{URL: "https://www.example.com/p/1"})

	assert.Equal(t, []bool{true, true, true}, []bool{
		strings.Contains(walled, "<title>Instagram</title>"),
		strings.Contains(walled, "only to visitors who sign in"),
		strings.Contains(empty, "<title>example.com</title>"),
	})
}
