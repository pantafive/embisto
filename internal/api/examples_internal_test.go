package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/card"
)

// TestTheExampleCardIsTheYouTubeFixtures holds the reference's example to the youtube fixture's golden preview: every
// field it shows is the golden one, and each of its links is one of the golden links.
func TestTheExampleCardIsTheYouTubeFixtures(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "youtube", "preview.json"))
	require.NoError(t, err)
	var golden card.Preview
	require.NoError(t, json.Unmarshal(raw, &golden))

	shown := examplePreview
	require.NotEmpty(t, shown.Links)
	for _, link := range shown.Links {
		assert.Contains(t, golden.Links, link)
	}
	shown.Links, golden.Links = nil, nil
	golden.Keywords = ""
	assert.Equal(t, golden, shown)
}
