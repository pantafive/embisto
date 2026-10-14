// Package debug serves a page that shows how a preview was made: the card, every fetch, source, provider and
// enricher with its time and outcome, which source won each field and why, and the card's image variants. It is for
// operators, on the internal listener only.
package debug

import (
	"bytes"
	"cmp"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/quality"
	"github.com/pantafive/embisto/internal/trace"
	"github.com/pantafive/embisto/thumbs"
)

//go:embed page.html
var pageHTML string

// page is parsed with html/template: every value from a page, embed HTML included, is shown escaped, never run.
var page = template.Must(template.New("debug").Funcs(template.FuncMap{
	"ms": func(d time.Duration) string { return d.Round(100 * time.Microsecond).String() },
}).Parse(pageHTML))

// Page makes previews afresh, without a cache, and shows how.
type Page struct {
	Core card.Extractor
	// Images, when the server has image variants on, also resizes the card's image into the card preset.
	Images thumbs.Processor
	Log    *slog.Logger
}

type view struct {
	URL       string
	Language  string
	Error     string
	Card      card.Preview
	Class     quality.Class
	JSON      string
	Took      time.Duration
	Steps     []trace.Step
	Decisions []trace.Decision
	Pictures  []picture
}

// picture is the card's image variant in one format.
type picture struct {
	Format string
	Type   string
	Bytes  int
	Took   time.Duration
	Error  string
}

func (p Page) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	shown := view{URL: r.URL.Query().Get("url"), Language: r.URL.Query().Get("lang")}
	if shown.URL != "" {
		p.explain(r.Context(), &shown)
	}
	var out bytes.Buffer
	if err := page.Execute(&out, shown); err != nil {
		p.Log.ErrorContext(r.Context(), "the debug page was not rendered", slog.Any("error", err))
		http.Error(w, "the debug page was not rendered", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(out.Bytes()); err != nil {
		p.Log.WarnContext(r.Context(), "the debug page was not written", slog.Any("error", err))
	}
}

func (p Page) explain(ctx context.Context, shown *view) {
	recorded := &trace.Trace{}
	started := time.Now()
	result, err := p.Core.Extract(trace.With(ctx, recorded), card.Request{URL: shown.URL, Options: card.Options{Language: shown.Language}})
	preview := result.Preview
	shown.Took = time.Since(started)
	shown.Steps, shown.Decisions = recorded.Steps(), recorded.Decisions()
	shown.Class = quality.Classify(preview, err)
	if err != nil {
		// This page is for the operator: it shows the cause a client is never told.
		shown.Error = err.Error()
		if cause := errors.Unwrap(err); cause != nil {
			shown.Error += ": " + cause.Error()
		}
		return
	}
	shown.Card = preview
	raw, err := json.MarshalIndent(preview, "", "  ")
	if err != nil {
		shown.Error = err.Error()
	}
	shown.JSON = string(raw)
	shown.Pictures = p.pictures(ctx, preview)
}

// pictures resizes the card's image into the card preset in each format, as /i/ would.
func (p Page) pictures(ctx context.Context, preview card.Preview) []picture {
	source, err := urlOf(preview.Image.URL)
	if p.Images == nil || err != nil {
		return nil
	}
	box := thumbs.DefaultPresets[1].Box
	var made []picture
	for _, format := range []thumbs.Format{thumbs.Classic, thumbs.WebP} {
		started := time.Now()
		result, err := p.Images.Process(ctx, source, box, format)
		made = append(made, picture{Format: cmp.Or(string(format), "jpeg/png"), Type: result.ContentType, Bytes: len(result.Body), Took: time.Since(started), Error: errorText(err)})
	}
	return made
}
