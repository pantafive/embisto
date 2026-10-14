package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/pantafive/embisto/card"
)

type previewInput struct {
	Format    string `query:"format" default:"json" doc:"The output format: json is the card; text is plain lines for terminals, messages, notifications and prompts; html is a self-contained HTML card; any other is a template the server loaded."`
	URL       string `query:"url" doc:"The page to preview, URL-encoded. Required. Without a scheme it is taken as https." example:"https://www.youtube.com/watch?v=dQw4w9WgXcQ"`
	MaxWidth  int    `query:"max_width" minimum:"0" doc:"The widest embed the client can show, in pixels; passed on to oEmbed providers."`
	MaxHeight int    `query:"max_height" minimum:"0" doc:"The tallest embed the client can show, in pixels; passed on to oEmbed providers."`
	Language  string `query:"lang" pattern:"^[A-Za-z]{2,3}(-[A-Za-z0-9]{1,8})*$" doc:"The language to ask the page in, as a BCP 47 tag such as de or pt-BR; by default the server's EMBISTO_ACCEPT_LANGUAGE. Previews in different languages are cached apart." example:"de"`
}

// compatibleInput takes every parameter as text, checked by the handler: a malformed one is then refused in
// Iframely's shape, which huma's own checks would answer as problem+json.
type compatibleInput struct {
	URL       string `query:"url" doc:"The page to preview, URL-encoded. Required." example:"https://www.youtube.com/watch?v=dQw4w9WgXcQ"`
	MaxWidth  string `query:"maxwidth" doc:"The widest embed the client can show, in pixels; passed on to oEmbed providers."`
	MaxHeight string `query:"maxheight" doc:"The tallest embed the client can show, in pixels; passed on to oEmbed providers."`
	Language  string `query:"lang" doc:"The language to ask the page in, as a BCP 47 tag such as de or pt-BR; by default the server's EMBISTO_ACCEPT_LANGUAGE. Previews in different languages are cached apart." example:"de"`
}

// language is a BCP 47 tag as the paths take it.
var language = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{1,8})*$`)

// request is the preview the compatible paths ask for; a malformed parameter is invalid_request.
func (in *compatibleInput) request() (card.Request, *card.Error) {
	maxWidth, isWidth := size(in.MaxWidth)
	maxHeight, isHeight := size(in.MaxHeight)
	if !isWidth || !isHeight || in.Language != "" && !language.MatchString(in.Language) {
		return card.Request{}, &card.Error{Code: card.CodeInvalidRequest}
	}
	return card.Request{URL: in.URL, Options: card.Options{MaxWidth: maxWidth, MaxHeight: maxHeight, Language: in.Language}}, nil
}

// size is a size in pixels, zero when absent; false for anything but a whole number from zero.
func size(raw string) (int, bool) {
	if raw == "" {
		return 0, true
	}
	parsed, err := strconv.Atoi(raw)
	return parsed, err == nil && parsed >= 0
}

type renderedOutput struct {
	Status       int
	ContentType  string `header:"Content-Type"`
	CacheControl string `header:"Cache-Control"`
	// CSP and NoSniff confine an answer a browser renders: no script, pictures from anywhere, nothing loaded besides.
	CSP     string `header:"Content-Security-Policy"`
	NoSniff string `header:"X-Content-Type-Options"`
	Body    []byte
}

// noSniff keeps a browser to the answer's own media type.
const noSniff = "nosniff"

// htmlPolicy lets an HTML answer show its inline styles and its pictures, and nothing else.
const htmlPolicy = "default-src 'none'; img-src https: http: data:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"

type purgedOutput struct {
	Status       int
	CacheControl string `header:"Cache-Control"`
}

// cacheNone keeps an answer out of every cache downstream.
const cacheNone = "no-store"

// cacheControl lets a cache downstream keep an answer exactly as long as embisto keeps it, so both expire together.
func (deps Deps) cacheControl(expires time.Time) string {
	remaining := time.Until(expires)
	if expires.IsZero() || remaining <= 0 {
		return cacheNone
	}
	return fmt.Sprintf("public, max-age=%d", int(remaining.Seconds()))
}

func (deps Deps) preview(ctx context.Context, in *previewInput) (*renderedOutput, error) {
	// huma has checked the format against the ones listed in the spec, which are these.
	renderer := deps.formats[in.Format]
	result, err := deps.Previews.Extract(ctx, card.Request{URL: in.URL, Options: card.Options{MaxWidth: in.MaxWidth, MaxHeight: in.MaxHeight, Language: in.Language}})
	if err != nil {
		failed, err := deps.failed(ctx, in.URL, err)
		if err != nil {
			return nil, err
		}
		refused := problemOf(failed)
		if refused.headers == nil {
			refused.headers = http.Header{}
		}
		refused.headers.Set("Cache-Control", deps.cacheControl(result.Expires))
		return nil, refused
	}
	return deps.rendered(ctx, renderer, result)
}

// purge drops what GET with the same parameters would serve from the cache; with only url, every variant of it.
func (deps Deps) purge(ctx context.Context, in *previewInput) (*purgedOutput, error) {
	dropped, err := deps.Previews.Purge(ctx, card.Request{URL: in.URL, Options: card.Options{MaxWidth: in.MaxWidth, MaxHeight: in.MaxHeight, Language: in.Language}})
	var failed *card.Error
	switch {
	case errors.As(err, &failed):
		refused := problemOf(failed)
		if refused.headers == nil {
			refused.headers = http.Header{}
		}
		refused.headers.Set("Cache-Control", cacheNone)
		return nil, refused
	case err != nil:
		deps.Log.WarnContext(ctx, "the cache did not purge", slog.Any("error", err))
		return nil, refusal(card.CodeCacheUnavailable)
	case dropped == 0:
		return nil, refusal(card.CodeNotCached)
	default:
		return &purgedOutput{Status: http.StatusNoContent, CacheControl: cacheNone}, nil
	}
}

// compatible answers in a format that also shapes its own failures.
func (deps Deps) compatible(format string) func(context.Context, *compatibleInput) (*renderedOutput, error) {
	return func(ctx context.Context, in *compatibleInput) (*renderedOutput, error) {
		renderer := deps.formats[format]
		req, refused := in.request()
		if refused != nil {
			return deps.refusedAs(renderer, refused, time.Time{})
		}
		result, err := deps.Previews.Extract(ctx, req)
		if err == nil {
			return deps.rendered(ctx, renderer, result)
		}
		failed, err := deps.failed(ctx, in.URL, err)
		if err != nil {
			return nil, err
		}
		return deps.refusedAs(renderer, failed, result.Expires)
	}
}

// refusedAs answers a failure in the renderer's own shape, or as problem+json for a renderer without one.
func (deps Deps) refusedAs(renderer card.Renderer, failed *card.Error, expires time.Time) (*renderedOutput, error) {
	refuser, ok := renderer.(card.ErrorRenderer)
	if !ok {
		return nil, problemOf(failed)
	}
	var body bytes.Buffer
	status, err := refuser.RenderError(&body, failed)
	if err != nil {
		return nil, fmt.Errorf("rendering the failure: %w", err)
	}
	return &renderedOutput{Status: status, ContentType: renderer.ContentType(), CacheControl: deps.cacheControl(expires), CSP: htmlPolicy, NoSniff: noSniff, Body: body.Bytes()}, nil
}

// failed logs a preview that failed and gives its card error; an error without a code fails the request.
func (deps Deps) failed(ctx context.Context, target string, err error) (*card.Error, error) {
	var failed *card.Error
	if !errors.As(err, &failed) {
		// Not a refusal the contract names, such as a module that panicked: its text is for the log, not the client.
		level := slog.LevelError
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			level = slog.LevelDebug // the client left
		}
		deps.Log.Log(ctx, level, "the preview failed", slog.String("url", target), slog.Any("error", err))
		return nil, refusal(card.CodeInternal)
	}
	deps.Log.InfoContext(ctx, "no preview", slog.String("url", target), slog.String("code", string(failed.Code)), slog.Any("error", err))
	return failed, nil
}

func (deps Deps) rendered(ctx context.Context, renderer card.Renderer, result card.Result) (*renderedOutput, error) {
	var body bytes.Buffer
	if err := renderer.Render(&body, result.Preview); err != nil {
		// The error names the operator's template and where it failed: that is for the log, not the client.
		deps.Log.ErrorContext(ctx, "the format failed", slog.String("format", renderer.Name()), slog.String("url", result.Preview.URL), slog.Any("error", err))
		return nil, refusal(card.CodeInternal)
	}
	answer := &renderedOutput{Status: http.StatusOK, ContentType: renderer.ContentType(), CacheControl: deps.cacheControl(result.Expires), Body: body.Bytes()}
	// Every answer, not only HTML: a format's media type is a template author's choice, and a browser may render
	// more than HTML.
	answer.CSP, answer.NoSniff = htmlPolicy, noSniff
	return answer, nil
}
