package fixture_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"unicode/utf8"

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

// TestTestdataHoldsNoPersonalData scans every saved file for the markers in tracking, and in JSON-LD blocks of a page
// for any key the trimmer does not keep, such as a video's comments with their authors' handles. Nothing else is
// looked for: an address, a signed URL or a name in a kept field passes.
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

// TestTestdataHoldsNoPicture checks every saved file is text, and no fixture keeps the body of a peek: a picture is
// saved as its size.
func TestTestdataHoldsNoPicture(t *testing.T) {
	var scan pictureScan
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return scan.file(path, raw)
	})

	require.NoError(t, err)
	require.Positive(t, scan.peeks)
	assert.Equal(t, [][]string{nil, nil}, [][]string{scan.binary, scan.keptBodies})
}

// pictureScan is what TestTestdataHoldsNoPicture found: the files that are not text, and the peeks and the bodies
// they kept.
type pictureScan struct {
	binary, keptBodies []string
	peeks              int
}

func (s *pictureScan) file(path string, raw []byte) error {
	if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		s.binary = append(s.binary, path)
	}
	if filepath.Base(path) != fixture.FixtureFile {
		return nil
	}
	var saved fixture.Fixture
	if err := json.Unmarshal(raw, &saved); err != nil {
		return err
	}
	for _, exchange := range saved.Exchanges {
		if exchange.Method != "peek" {
			continue
		}
		s.peeks++
		if exchange.File != "" {
			s.keptBodies = append(s.keptBodies, path+": "+exchange.File)
		}
	}
	return nil
}

func TestTheScanFindsATrackingMarkerAndAnUnreadKey(t *testing.T) {
	page := `<meta name="visitor-payload" content="eyJ"><script type="application/ld+json">{"comment":[{"author":{"name":"@someone"}}]}</script>`

	marker := tracking.FindString(page)
	keys := unreadKeys([]byte(linkedDataBlock.FindStringSubmatch(page)[1]))

	assert.Equal(t, []any{"visitor-payload", []string{"comment"}}, []any{marker, keys})
}
