package render

import (
	"cmp"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/pantafive/embisto/card"
)

// maxDescription is how many characters of the description a text card keeps: a sentence or two, enough for a
// notification or a prompt.
const maxDescription = 200

// Text writes the preview as plain lines, for terminals, messages, notifications and prompts: the title with its
// site, the description trimmed to a sentence or two, and the canonical URL.
type Text struct{}

func (Text) Name() string {
	return formatText
}

func (Text) ContentType() string {
	return contentTypeText
}

func (Text) Render(w io.Writer, preview card.Preview) error {
	var lines []string
	switch {
	case preview.Title != "" && preview.SiteName != "" && !strings.Contains(preview.Title, preview.SiteName):
		lines = append(lines, preview.Title+" — "+preview.SiteName)
	case preview.Title != "":
		lines = append(lines, preview.Title)
	case preview.SiteName != "":
		lines = append(lines, preview.SiteName)
	}
	if description := trimmed(preview.Description, maxDescription); description != "" {
		lines = append(lines, description)
	}
	lines = append(lines, cmp.Or(preview.CanonicalURL, preview.URL))
	if _, err := io.WriteString(w, strings.Join(lines, "\n")+"\n"); err != nil {
		return fmt.Errorf("writing the card: %w", err)
	}
	return nil
}

// trimmed cuts text at the last word that fits in limit characters and marks the cut.
func trimmed(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	cut := string([]rune(text)[:limit])
	if space := strings.LastIndex(cut, " "); space > 0 {
		cut = cut[:space]
	}
	return strings.TrimRight(cut, " ,.;:—-") + "…"
}
