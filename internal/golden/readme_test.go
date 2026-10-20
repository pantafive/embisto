package golden_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/card"
)

// TestTheReadmeShowsTheGoBlogCard holds the quickstart's answer to the go-dev-blog fixture's golden preview: it shows
// every field of it but images and links, which its note says it leaves out.
func TestTheReadmeShowsTheGoBlogCard(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	require.NoError(t, err)
	_, sample, ok := strings.Cut(string(readme), "```json\n")
	require.True(t, ok, "README.md has no JSON answer")
	sample, _, _ = strings.Cut(sample, "```")
	decoder := json.NewDecoder(strings.NewReader(sample))
	decoder.DisallowUnknownFields()
	var shown card.Preview
	require.NoError(t, decoder.Decode(&shown))

	raw, err := os.ReadFile(filepath.Join(fixtures, "go-dev-blog", "preview.json"))
	require.NoError(t, err)
	var golden card.Preview
	require.NoError(t, json.Unmarshal(raw, &golden))
	golden.Images, golden.Links = nil, nil
	assert.Equal(t, golden, shown)
}
