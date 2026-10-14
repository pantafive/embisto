package render_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/render"
)

func files(name, source string) fstest.MapFS {
	return fstest.MapFS{name: {Data: []byte(source)}}
}

func renderWith(t *testing.T, renderer card.Renderer, preview card.Preview) string {
	t.Helper()
	var out strings.Builder
	require.NoError(t, renderer.Render(&out, preview))
	return out.String()
}

func TestLoadTemplatesMakesAFormatOfEachFile(t *testing.T) {
	loaded, err := render.LoadTemplates(fstest.MapFS{
		"short.json.tmpl": {Data: []byte(`{"title": {{json .Title}}}`)},
		"note.md.tmpl":    {Data: []byte(`**{{md .Title}}**`)},
		"README.md":       {Data: []byte("not a template")},
	})

	require.NoError(t, err)
	require.Len(t, loaded, 2)
	preview := card.Preview{Title: `A "quoted" *title*`}
	assert.Equal(t, []string{"note", "text/markdown; charset=utf-8", `**A "quoted" \*title\***`, "short", "application/json", `{"title": "A \"quoted\" *title*"}`},
		[]string{loaded[0].Name(), loaded[0].ContentType(), renderWith(t, loaded[0], preview), loaded[1].Name(), loaded[1].ContentType(), renderWith(t, loaded[1], preview)})
}

func TestLoadTemplatesRefusesWhatCannotServe(t *testing.T) {
	tests := []struct {
		name, file, source, says string
	}{
		{"a parse error", "broken.txt.tmpl", `{{.Title`, "broken.txt.tmpl"},
		{"JSON without escaping", "unsafe.json.tmpl", `{"title": "{{.Title}}"}`, "unsafe.json.tmpl: on a card of hostile text, the output is not JSON"},
		{"a field the card lacks", "typo.txt.tmpl", `{{.Headline}}`, "typo.txt.tmpl"},
		{"a format of embisto's own", "json.json.tmpl", `{}`, `"json" is a format of embisto's own`},
		{"a name without an extension", "plain.tmpl", `x`, "plain.tmpl: the name is not <name>.<ext>.tmpl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := render.LoadTemplates(files(tt.file, tt.source))

			require.ErrorIs(t, err, render.ErrTemplate)
			assert.Contains(t, err.Error(), tt.says)
		})
	}
}

func TestHTMLTemplatesEscapeWhatAPageSays(t *testing.T) {
	loaded, err := render.LoadTemplates(files("card.html.tmpl", `<a href="{{.URL}}">{{.Title}}</a>`))
	require.NoError(t, err)

	out := renderWith(t, loaded[0], card.Preview{URL: "javascript:alert(1)", Title: "<script>alert(1)</script>"})

	assert.Equal(t, `<a href="#ZgotmplZ">&lt;script&gt;alert(1)&lt;/script&gt;</a>`, out)
}

func TestEscapers(t *testing.T) {
	loaded, err := render.LoadTemplates(files("all.txt.tmpl", "{{dart .Title}}|{{shell .Title}}|{{json .Title}}|{{truncate 5 .Description}}|{{default \"none\" .Author}}"))
	require.NoError(t, err)

	out := renderWith(t, loaded[0], card.Preview{Title: "it's ${x}\n", Description: "one two three"})

	assert.Equal(t, `'it\'s \${x}\n'|'it'\''s ${x}`+"\n"+`'|"it's ${x}\n"|one…|none`, out)
}

func TestPictureChoosesTheSmallestVariantWideEnough(t *testing.T) {
	loaded, err := render.LoadTemplates(files("pic.txt.tmpl", `{{with picture . 300 true}}{{.URL}} {{.Width}}{{end}}|{{with picture . 2000 false}}{{.URL}}{{end}}`))
	require.NoError(t, err)
	preview := card.Preview{Image: card.Image{URL: "https://e.test/o.jpg", Variants: []card.Variant{
		{Preset: "large", URL: "https://e.test/l", WebPURL: "https://e.test/l.webp", Width: 1200},
		{Preset: "card", URL: "https://e.test/c", WebPURL: "https://e.test/c.webp", Width: 600},
	}}}

	out := renderWith(t, loaded[0], preview)

	assert.Equal(t, "https://e.test/c.webp 600|https://e.test/l", out)
}

func TestTemplatesGoByTheirExtensionWhateverItsCase(t *testing.T) {
	loaded, err := render.LoadTemplates(files("card.HTML.tmpl", `<p>{{.Title}}</p>`))
	require.NoError(t, err)

	out := renderWith(t, loaded[0], card.Preview{Title: "<script>alert(1)</script>"})

	assert.Equal(t, []string{"text/html; charset=utf-8", "<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>"}, []string{loaded[0].ContentType(), out})
}

func TestMarkupTemplatesEscapeAndMustParse(t *testing.T) {
	loaded, err := render.LoadTemplates(files("feed.xml.tmpl", `<item><title>{{.Title}}</title><link>{{.URL}}</link></item>`))
	require.NoError(t, err)

	out := renderWith(t, loaded[0], card.Preview{URL: "https://example.com/?a=1&b=2", Title: `</title><script>alert(1)</script>`})

	assert.Equal(t, []string{"application/xml; charset=utf-8", `<item><title>&lt;/title&gt;&lt;script&gt;alert(1)&lt;/script&gt;</title><link>https://example.com/?a=1&amp;b=2</link></item>`}, []string{loaded[0].ContentType(), out})
}

func TestTemplatesWhoseOutputDoesNotParseAsTheirTypeAreRefused(t *testing.T) {
	tests := []struct {
		file, source, says string
	}{
		{"bot.JSON.tmpl", `{"title": "{{.Title}}"}`, "not JSON"},
		{"broken.xml.tmpl", `<a>{{.Title}}</b>`, "not well-formed XML"},
		{"rows.csv.tmpl", `"{{.Title}}`, "not CSV"},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			_, err := render.LoadTemplates(files(tt.file, tt.source))

			require.ErrorIs(t, err, render.ErrTemplate)
			assert.Contains(t, err.Error(), tt.says)
		})
	}
}
