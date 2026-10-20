package render

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"io"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	texttemplate "text/template"
	"time"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/iframely"
)

// ErrTemplate is a template file that cannot serve: a bad name, a parse error, or output that breaks its format.
var ErrTemplate = errors.New("template")

// templateSuffix ends a template file's name: <name>.<ext>.tmpl.
const templateSuffix = ".tmpl"

var templateName = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// builtIn are the formats embisto has of its own, which no template may take.
var builtIn = []string{formatJSON, formatText, formatHTML, iframely.Format, formatOEmbed}

// The names of embisto's own formats.
const (
	formatJSON   = "json"
	formatText   = "text"
	formatHTML   = "html"
	formatOEmbed = "oembed"
)

// contentTypeText is plain text's media type.
const contentTypeText = "text/plain; charset=utf-8"

// extensionJSON is the extension of a template whose output must be JSON.
const extensionJSON = "json"

// contentTypes are the media types of the extensions templates are written in; any other is plain text. The table is
// embisto's own, so a format answers the same wherever it runs.
var contentTypes = map[string]string{
	"html":         "text/html; charset=utf-8",
	"htm":          "text/html; charset=utf-8",
	extensionXHTML: "application/xhtml+xml; charset=utf-8",
	extensionSVG:   "image/svg+xml; charset=utf-8",
	extensionJSON:  "application/json",
	"txt":          contentTypeText,
	"md":           "text/markdown; charset=utf-8",
	"dart":         contentTypeText,
	"rfwtxt":       contentTypeText,
	extensionXML:   "application/xml; charset=utf-8",
	"csv":          "text/csv; charset=utf-8",
}

// markup are the extensions whose output a browser renders as markup: their templates run as html/template, which
// escapes every value for where it appears, since a page's text there could otherwise be a script.
var markup = []string{formatHTML, "htm", extensionXHTML, extensionXML, extensionSVG}

// The extensions of the XML formats, whose output must be well-formed.
const (
	extensionXML   = "xml"
	extensionSVG   = "svg"
	extensionXHTML = "xhtml"
)

// executor is a parsed template of either kind.
type executor interface {
	Execute(w io.Writer, data any) error
}

// Template is an output format a template file defines.
type Template struct {
	name        string
	extension   string
	contentType string
	parsed      executor
}

func (t Template) Name() string {
	return t.name
}

func (t Template) ContentType() string {
	return t.contentType
}

// Render runs the template on the card. A failure leaves nothing written: the output is built first.
func (t Template) Render(w io.Writer, preview card.Preview) error {
	var out bytes.Buffer
	if err := t.parsed.Execute(&out, preview); err != nil {
		return fmt.Errorf("%w %s: %w", ErrTemplate, t.name, err)
	}
	if _, err := w.Write(out.Bytes()); err != nil {
		return fmt.Errorf("writing %s: %w", t.name, err)
	}
	return nil
}

// LoadTemplates reads every <name>.<ext>.tmpl at the top of the file system as the output format <name>. A .html
// template runs as html/template, which escapes by context; any other as text/template, whose author escapes with
// the helpers. In the template, . is the card, with card.Preview's Go field names.
//
// Every template is run at load on cards made to break it, an empty one and one whose text holds quotes, newlines,
// markup and $; it must not fail, and a .json template must give JSON. A file that fails is named in the error.
func LoadTemplates(fsys fs.FS) ([]card.Renderer, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("reading the templates: %w", err)
	}
	var loaded []card.Renderer
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), templateSuffix) {
			continue
		}
		template, err := load(fsys, entry.Name())
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(loaded, func(other card.Renderer) bool { return other.Name() == template.name }) {
			return nil, fmt.Errorf("%w %s: another template is also named %q", ErrTemplate, entry.Name(), template.name)
		}
		loaded = append(loaded, template)
	}
	return loaded, nil
}

func load(fsys fs.FS, file string) (Template, error) {
	name, extension, ok := strings.Cut(strings.TrimSuffix(file, templateSuffix), ".")
	// Read once, so that the engine, the checks and the media type all go by the same extension: card.HTML.tmpl is html.
	extension = strings.ToLower(extension)
	switch {
	case !ok || strings.Contains(extension, "."):
		return Template{}, fmt.Errorf("%w %s: the name is not <name>.<ext>.tmpl", ErrTemplate, file)
	case !templateName.MatchString(name):
		return Template{}, fmt.Errorf("%w %s: the format's name must be lowercase letters, digits, - and _", ErrTemplate, file)
	case slices.Contains(builtIn, name):
		return Template{}, fmt.Errorf("%w %s: %q is a format of embisto's own", ErrTemplate, file, name)
	}
	source, err := fs.ReadFile(fsys, file)
	if err != nil {
		return Template{}, fmt.Errorf("%w %s: %w", ErrTemplate, file, err)
	}
	template := Template{name: name, extension: extension, contentType: contentTypeOf(extension)}
	if slices.Contains(markup, extension) {
		template.parsed, err = htmltemplate.New(file).Funcs(helpers).Parse(string(source))
	} else {
		template.parsed, err = texttemplate.New(file).Funcs(helpers).Option("missingkey=error").Parse(string(source))
	}
	if err != nil {
		return Template{}, fmt.Errorf("%w %s: %w", ErrTemplate, file, err)
	}
	if err := template.lint(file); err != nil {
		return Template{}, err
	}
	return template, nil
}

func contentTypeOf(extension string) string {
	if known, ok := contentTypes[extension]; ok {
		return known
	}
	return contentTypes["txt"]
}

// lint runs the template on the hostile cards: it must not fail on any, and its output must parse as its type.
func (t Template) lint(file string) error {
	for _, sample := range hostileCards {
		var out bytes.Buffer
		if err := t.Render(&out, sample.preview); err != nil {
			return fmt.Errorf("%w %s: on %s: %w", ErrTemplate, file, sample.name, err)
		}
		if err := parses(t.extension, out.Bytes()); err != nil {
			return fmt.Errorf("%w %s: on %s, %w: %q", ErrTemplate, file, sample.name, err, truncate(200, out.String()))
		}
	}
	return nil
}

var (
	errNotJSON = errors.New("the output is not JSON; escape text with json")
	errNotXML  = errors.New("the output is not well-formed XML")
	errNotCSV  = errors.New("the output is not CSV")
)

// parses says whether output parses as its extension's type, for the types a client parses rather than shows.
func parses(extension string, output []byte) error {
	switch extension {
	case extensionJSON:
		if !json.Valid(output) {
			return errNotJSON
		}
	case extensionXML, extensionSVG, extensionXHTML:
		decoder := xml.NewDecoder(bytes.NewReader(output))
		for {
			_, err := decoder.Token()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("%w: %w", errNotXML, err)
			}
		}
	case "csv":
		if _, err := csv.NewReader(bytes.NewReader(output)).ReadAll(); err != nil {
			return fmt.Errorf("%w: %w", errNotCSV, err)
		}
	}
	return nil
}

// sampleSite names the sample cards' site.
const sampleSite = "Example"

// hostile is the text a page may put in a card to break a template that does not escape it.
const hostile = "Say \"hi\" \\ and\nthen </script><script>alert(1)</script> <https://x.test|y> ${env} it's done 🚀"

var hostileCards = []struct {
	name    string
	preview card.Preview
}{
	{"an empty card", card.Preview{}},
	{"a full card", card.Preview{
		URL: "https://example.com/post", CanonicalURL: "https://example.com/post", Type: card.TypeVideo,
		Title: "A post", Description: "What it says.", SiteName: sampleSite, Author: "Someone",
		PublishedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Icon:        card.Image{URL: "https://example.com/icon.png", Width: 32, Height: 32},
		Image: card.Image{URL: "https://example.com/cover.jpg", Width: 1200, Height: 630, Variants: []card.Variant{
			{Preset: "thumb", URL: "https://embisto.test/i/a/thumb/b", WebPURL: "https://embisto.test/i/a/thumb.webp/b", Width: 160, Height: 160},
			{Preset: "card", URL: "https://embisto.test/i/a/card/b", WebPURL: "https://embisto.test/i/a/card.webp/b", Width: 600, Height: 315},
		}},
		Video: card.Video{URL: "https://example.com/player", Type: "text/html", Width: 640, Height: 360},
		Embed: card.Embed{HTML: `<iframe src="https://example.com/player"></iframe>`, Width: 640, Height: 360, Provider: sampleSite},
	}},
	{"a walled card", card.Preview{URL: "https://example.com/p/1", CanonicalURL: "https://example.com/p/1", Type: card.TypeLink, SiteName: sampleSite, Limited: card.LimitLoginWall}},
	{"a card of hostile text", card.Preview{
		URL: "https://example.com/x?a=1&b=\"2\"", CanonicalURL: "https://example.com/x?a=1&b=\"2\"", Type: card.TypeLink,
		Title: hostile, Description: hostile, SiteName: hostile, Author: hostile,
		Image: card.Image{URL: "https://example.com/a.jpg?x=\"><script>"},
	}},
}

// Names are the formats' names, for a listing.
func Names(renderers []card.Renderer) []string {
	names := make([]string, len(renderers))
	for i, renderer := range renderers {
		names[i] = renderer.Name()
	}
	return names
}
