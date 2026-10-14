package fixture_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/internal/fixture"
)

// root is the repository's testdata, which this test reads from internal/fixture.
var root = filepath.Join("..", "..", "testdata")

// tracking are markers of a visitor, a session or a request that a saved page must not keep: GitHub's visitor
// payload and its HMAC, request ids, nonces, analytics ids, a logged-in user.
var tracking = regexp.MustCompile(`(?i)visitor-payload|visitor-hmac|name="request-id"|html-safe-nonce|\bnonce="|octolytics|csrf|name="user-login" content="[^"]`)

var linkedDataBlock = regexp.MustCompile(`(?is)<script[^>]*application/ld\+json[^>]*>(.*?)</script>`)

// TestTestdataHoldsNoPersonalData scans every saved file for what a visitor or a commenter left in a page: tracking
// markers, and in JSON-LD any key the trimmer does not keep, such as a video's comments with their authors' handles.
func TestTestdataHoldsNoPersonalData(t *testing.T) {
	var found []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if marker := tracking.Find(raw); marker != nil {
			found = append(found, path+": "+string(marker))
		}
		for _, block := range linkedDataBlock.FindAllSubmatch(raw, -1) {
			for _, key := range unreadKeys(block[1]) {
				found = append(found, path+": JSON-LD key "+key)
			}
		}
		return nil
	})

	require.NoError(t, err)
	assert.Empty(t, found, "run `embisto pin -retrim` to trim saved pages again")
}

// unreadKeys are the keys of a JSON-LD block the sources do not read, each once.
func unreadKeys(block []byte) []string {
	var value any
	if len(bytes.TrimSpace(block)) == 0 {
		return nil
	}
	if json.Unmarshal(block, &value) != nil {
		return []string{"(not JSON)"}
	}
	var unread []string
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				if !slices.Contains(fixture.LinkedDataKeys, key) && !slices.Contains(unread, key) {
					unread = append(unread, key)
				}
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	slices.Sort(unread)
	return unread
}

func TestTheScanFindsWhatTheReviewFound(t *testing.T) {
	page := `<meta name="visitor-payload" content="eyJ"><script type="application/ld+json">{"comment":[{"author":{"name":"@someone"}}]}</script>`

	marker := tracking.FindString(page)
	keys := unreadKeys([]byte(linkedDataBlock.FindStringSubmatch(page)[1]))

	assert.Equal(t, []any{"visitor-payload", []string{"comment"}}, []any{marker, keys})
}
