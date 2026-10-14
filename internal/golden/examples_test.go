package golden_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/fixture"
	"github.com/pantafive/embisto/render"
	"github.com/pantafive/embisto/thumbnails"
)

// examples are the example templates, at the module's root.
var examples = filepath.Join("..", "..", "examples", "templates")

// exampleCards are real cards, replayed from fixtures with thumbnails, and one of hostile text.
func exampleCards(t *testing.T) map[string]card.Preview {
	t.Helper()
	signer, err := thumbnails.NewSigner([]byte(strings.Repeat("k", thumbnails.MinKeyLength)), thumbnails.DefaultPresets)
	require.NoError(t, err)
	cards := map[string]card.Preview{"hostile": {
		URL: "https://example.com/x?a=1&b=2", Type: card.TypeLink, SiteName: `Site "$x" <b>`,
		Title: "Say \"hi\" \\ and\nthen </script> <!channel> <https://x.test|y> ${env} it's done 🚀", Description: "*bold* [link](x)   next",
	}}
	for _, name := range []string{"youtube", "github", "guardian-article", "instagram-post"} {
		saved, replay, err := fixture.Load(filepath.Join(fixtures, name))
		require.NoError(t, err)
		opts := embisto.Defaults()
		opts.Fetcher, opts.Enrichers = replay, append(opts.Enrichers, thumbnails.Enricher{Signer: signer, BaseURL: "https://embisto.example"})
		result, err := embisto.New(opts).Extract(context.Background(), saved.Request)
		require.NoError(t, err)
		cards[name] = result.Preview
	}
	return cards
}

// TestExampleTemplatesMatchTheirGoldens renders every example template on every example card.
func TestExampleTemplatesMatchTheirGoldens(t *testing.T) {
	templates, err := render.LoadTemplates(os.DirFS(examples))
	require.NoError(t, err)
	require.NotEmpty(t, templates)
	cards := exampleCards(t)
	for _, template := range templates {
		for name, preview := range cards {
			t.Run(template.Name()+"/"+name, func(t *testing.T) {
				var out bytes.Buffer
				require.NoError(t, template.Render(&out, preview))
				golden := filepath.Join(examples, "testdata", template.Name(), name+".golden")
				if *update {
					require.NoError(t, os.MkdirAll(filepath.Dir(golden), 0o750))
					require.NoError(t, os.WriteFile(golden, out.Bytes(), 0o600))
				}
				want, err := os.ReadFile(golden)
				require.NoError(t, err)
				assert.Equal(t, string(want), out.String())
			})
		}
	}
}
