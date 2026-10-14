package render

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pantafive/embisto/card"
)

// Picture is the picture a template shows: a variant of the card's image, or the image itself.
type Picture struct {
	URL    string
	Width  int
	Height int
	// Type is the picture's media type when known: image/webp for a WebP variant.
	Type string
}

// helpers are what every template may call: pure functions of their arguments, with no clock, file, environment or
// network behind them. None returns a type that turns html/template's escaping off.
var helpers = map[string]any{
	"truncate":    truncate,
	"default":     fallback,
	"picture":     pictureFor,
	extensionJSON: jsonString,
	"dart":        dartString,
	"md":          markdown,
	"shell":       shellQuoted,
	"slack":       slackEscaped,
	"ratio":       ratio,
}

// slackEscaped is text for Slack's mrkdwn, where &, < and > would otherwise start an entity, a link or a mention.
func slackEscaped(text string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(text)
}

// ratio is width over height with a decimal point, as a layout that wants a double reads it; 1.9091, a link
// preview's usual shape, when either is unknown.
func ratio(width, height int) string {
	if width <= 0 || height <= 0 {
		return "1.9091"
	}
	return strconv.FormatFloat(float64(width)/float64(height), 'f', 4, 64)
}

// truncate cuts text at the last word that fits in limit characters and marks the cut.
func truncate(limit int, text string) string {
	return trimmed(text, limit)
}

// fallback is the value, or the default when the value is empty: {{default "Untitled" .Title}}.
func fallback(otherwise string, value string) string {
	if strings.TrimSpace(value) == "" {
		return otherwise
	}
	return value
}

// pictureFor is the card's picture for a box minWidth wide: the smallest variant at least that wide, or the largest
// one, or the original image; as WebP when asked and the variant has it. Without the image proxy, it is the original.
func pictureFor(preview card.Preview, minWidth int, isWebP bool) Picture {
	image := preview.Image
	if len(image.Variants) == 0 {
		return Picture{URL: image.URL, Width: image.Width, Height: image.Height}
	}
	variants := slices.Clone(image.Variants)
	slices.SortStableFunc(variants, func(a, b card.Variant) int { return a.Width - b.Width })
	chosen := variants[len(variants)-1]
	if at := slices.IndexFunc(variants, func(v card.Variant) bool { return v.Width >= minWidth }); at >= 0 {
		chosen = variants[at]
	}
	if isWebP && chosen.WebPURL != "" {
		return Picture{URL: chosen.WebPURL, Width: chosen.Width, Height: chosen.Height, Type: "image/webp"}
	}
	return Picture{URL: chosen.URL, Width: chosen.Width, Height: chosen.Height}
}

// jsonString is a complete JSON string literal, quotes included: "title": {{json .Title}}.
func jsonString(text string) string {
	quoted, err := json.Marshal(text)
	if err != nil {
		return `""` // a string always marshals; this keeps the output valid regardless
	}
	return string(quoted)
}

// dartString is a complete single-quoted Dart string literal. $ is escaped too: in Dart it interpolates, and a page's
// title would otherwise become code.
func dartString(text string) string {
	var out strings.Builder
	out.WriteByte('\'')
	for _, r := range text {
		switch r {
		case '\\', '\'', '$':
			out.WriteByte('\\')
			out.WriteRune(r)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if r < ' ' || r == ' ' || r == ' ' {
				fmt.Fprintf(&out, `\u{%x}`, r)
				continue
			}
			out.WriteRune(r)
		}
	}
	out.WriteByte('\'')
	return out.String()
}

// markdownSpecial are the characters CommonMark could read as markup.
const markdownSpecial = "\\`*_{}[]()<>#+-.!|~&"

// markdown escapes text so CommonMark shows it as written, on one line.
func markdown(text string) string {
	var out strings.Builder
	for _, r := range strings.Join(strings.Fields(text), " ") {
		if r < utf8.RuneSelf && strings.ContainsRune(markdownSpecial, r) {
			out.WriteByte('\\')
		}
		out.WriteRune(r)
	}
	return out.String()
}

// shellQuoted is text as one single-quoted POSIX shell word.
func shellQuoted(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}
