package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/cache"
	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/api"
	"github.com/pantafive/embisto/internal/debug"
	"github.com/pantafive/embisto/internal/metrics"
	"github.com/pantafive/embisto/internal/settings"
	"github.com/pantafive/embisto/render"
	"github.com/pantafive/embisto/sites"
	"github.com/pantafive/embisto/thumbnails"
)

// The limits of thumbnails. Pictures being resized share imageMaxMemory, each admitted by what its header says
// decoding and scaling it will take; with the source files read, thumbnails hold at most imageMaxMemory +
// imageConcurrency × imageMaxBytes: some 300 MB. A picture at the pixel limit takes about half a second to resize.
const (
	imageMaxBytes    = 10 << 20
	imageMaxPixels   = 16_000_000
	imageMaxMemory   = 256 << 20
	imageConcurrency = 4
	imageWait        = 2 * time.Second
	// imageTimeout is longer than a preview's: a large picture over a slow link takes longer than a page's head.
	imageTimeout = 10 * time.Second
)

// providers are the default site providers with the server's settings.
func providers(loaded settings.Settings) []card.Provider {
	configured := slices.Clone(embisto.Defaults().Providers)
	for i, provider := range configured {
		if _, ok := provider.(sites.YouTube); ok {
			configured[i] = sites.YouTube{NoCookie: loaded.YouTubeNoCookie}
		}
	}
	return configured
}

// debugPage serves how previews are made, on the internal listener, from a core of its own: no cache, so every run is
// fresh, and no observer, so it does not count in the metrics.
// debugHomePages is how many bytes of home pages the debug page remembers.
const debugHomePages = 4 << 20

func debugPage(ctx context.Context, opts embisto.Options, served *api.Thumbnails, log *slog.Logger) http.Handler {
	log.WarnContext(ctx, "the debug page is on, on the internal listener", slog.String("path", "/debug"))
	opts.Observer = nil
	// Its own memory of home pages: what it learns must not change what the API serves.
	opts.HomePages = cache.NewMemory(debugHomePages, nil)
	page := debug.Page{Core: embisto.New(opts), Log: log}
	if served != nil {
		page.Thumbnails = served.Processor
	}
	return page
}

// templateFormats are the formats the templates directory defines; none without one.
func templateFormats(ctx context.Context, loaded settings.Settings, log *slog.Logger) ([]card.Renderer, error) {
	if loaded.TemplatesDir == "" {
		return nil, nil
	}
	templates, err := render.LoadTemplates(os.DirFS(loaded.TemplatesDir))
	if err != nil {
		return nil, fmt.Errorf("EMBISTO_TEMPLATES_DIR: %w", err)
	}
	log.InfoContext(ctx, "template formats loaded", slog.Any("formats", render.Names(templates)))
	return templates, nil
}

var (
	errNoImageKey  = errors.New("EMBISTO_IMAGE_KEY is not set: thumbnails are off")
	errNoPublicURL = errors.New("EMBISTO_PUBLIC_URL is not set: a thumbnail's URL starts with the address clients reach the server at")
)

// newThumbnails are the parts that serve thumbnails, and the enricher that hands out their URLs when clients have an
// address to reach them at; nil and nil when thumbnails are off.
func newThumbnails(ctx context.Context, loaded settings.Settings, log *slog.Logger, measured *metrics.Metrics) (*api.Thumbnails, card.Enricher, error) {
	if loaded.ImageKey == "" {
		log.InfoContext(ctx, "thumbnails are off: cards link pictures on their sites; set EMBISTO_IMAGE_KEY to turn them on")
		return nil, nil, nil
	}
	signer, err := thumbnails.NewSigner([]byte(loaded.ImageKey), loaded.ImagePresets)
	if err != nil {
		return nil, nil, fmt.Errorf("thumbnails: %w", err)
	}
	fetcher := embisto.NewFetcher(embisto.Options{Timeout: imageTimeout, UserAgent: embisto.Defaults().UserAgent})
	served := &api.Thumbnails{
		Signer: signer, Observer: measured,
		Processor: thumbnails.NewResizer(fetcher, thumbnails.Limits{
			MaxBytes: imageMaxBytes, MaxPixels: imageMaxPixels, MaxMemory: imageMaxMemory, Concurrency: imageConcurrency, Wait: imageWait,
		}),
	}
	if loaded.PublicURL == "" {
		log.WarnContext(ctx, "thumbnails are served at /i/, but cards carry no URLs of them without EMBISTO_PUBLIC_URL")
		return served, nil, nil
	}
	return served, thumbnails.Enricher{Signer: signer, BaseURL: loaded.PublicURL}, nil
}

// runSign prints the URL of a picture's variant in a size, for a client backend to see how URLs are made, or to test one.
func runSign(args []string, environ func() []string, out io.Writer) error {
	if len(args) != 2 {
		return fmt.Errorf("%w: usage: embisto sign <preset|WxH-fit|WxH-fill> <url>", settings.ErrInvalid)
	}
	loaded, err := settings.Load(environ)
	if err != nil {
		return err //nolint:wrapcheck // it names the setting and what is wrong with it
	}
	switch {
	case loaded.ImageKey == "":
		return errNoImageKey
	case loaded.PublicURL == "":
		return errNoPublicURL
	}
	signer, err := thumbnails.NewSigner([]byte(loaded.ImageKey), loaded.ImagePresets)
	if err != nil {
		return fmt.Errorf("thumbnails: %w", err)
	}
	_, err = fmt.Fprintln(out, loaded.PublicURL+signer.Path(args[0], args[1]))
	return err //nolint:wrapcheck // a failed print to the terminal
}
