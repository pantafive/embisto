// Command embisto serves link previews over HTTP.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/cache"
	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/api"
	"github.com/pantafive/embisto/internal/logging"
	"github.com/pantafive/embisto/internal/metrics"
	"github.com/pantafive/embisto/internal/settings"
	"github.com/pantafive/embisto/render"
)

const (
	headerTimeout = 10 * time.Second
	// shutdownGrace is how long requests in flight get to finish once the server stops taking new ones: longer than
	// the slowest answer, and with unreadyFor within the 30 seconds Kubernetes gives a pod to stop.
	shutdownGrace = 24 * time.Second
	// unreadyFor is how long a stopping server reports not ready while still serving, so the load balancer stops
	// sending before the server stops taking.
	unreadyFor = 5 * time.Second
	// lockTTL outlasts the slowest preview: the core's timeout, with the enrichers and the home page after it, twice
	// over. A holder that crashed frees the lock this soon.
	lockTTL = 15 * time.Second
	// readTimeout bounds reading a request; none has a body worth waiting for.
	readTimeout = 15 * time.Second
	// writeTimeout bounds a whole answer. The slowest is a preview that waited lockTTL for another instance and then
	// made itself, or a picture fetched and resized; both end well within it.
	writeTimeout = 45 * time.Second
	// idleTimeout is how long a kept-alive connection may sit between requests.
	idleTimeout = 2 * time.Minute
)

// usage lists the commands; the server reads its settings from the environment.
const usage = `Usage:
  embisto                    serve the API, configured by EMBISTO_* environment variables
                             (see docs/configuration.md)
  embisto sign <size> <url>  print the URL of a picture's thumbnail in a size
  embisto pin [-dir dir] [-name name] <url>
                             save a URL's responses and preview as a test fixture
  embisto help               print this
`

var errUnknownCommand = errors.New("unknown command")

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args, os.Environ, os.Stdout)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

// run is the whole program as a testable function: it serves until the context ends, or runs the command named.
func run(ctx context.Context, args []string, environ func() []string, out io.Writer) error {
	if len(args) < 2 {
		return runServer(ctx, environ)
	}
	switch args[1] {
	case "pin":
		return runPin(ctx, args[2:], out)
	case "sign":
		return runSign(args[2:], environ, out)
	case "help", "-h", "-help", "--help":
		_, err := io.WriteString(out, usage)
		return err //nolint:wrapcheck // a failed print to the terminal
	default:
		return fmt.Errorf("%w %q\n%s", errUnknownCommand, args[1], usage)
	}
}

// runServer serves the API and, on the internal listener, the metrics, until the context ends.
func runServer(ctx context.Context, environ func() []string) error {
	loaded, err := settings.Load(environ)
	if err != nil {
		return err //nolint:wrapcheck // it names the setting and what is wrong with it
	}
	log := logging.New(os.Stdout, loaded.LogLevel)
	measured := metrics.New()
	opts := embisto.Defaults()
	opts.Log, opts.Observer, opts.DefaultAcceptLanguage = log, measured, loaded.DefaultAcceptLanguage
	opts.Providers = providers(loaded)
	templates, err := templateFormats(ctx, loaded, log)
	if err != nil {
		return err
	}
	renderers := append(render.Defaults(), templates...)
	served, enricher, err := newThumbnails(ctx, loaded, log, measured)
	if err != nil {
		return err
	}
	if enricher != nil {
		// After the sizes are measured, so the variants know theirs.
		opts.Enrichers = append(opts.Enrichers, enricher)
	}
	shared, err := sharedStore(loaded, log, measured)
	if err != nil {
		return err
	}
	if shared != nil {
		// Instances learn a site's home page once between them.
		opts.HomePages = shared
	}
	previews := layered(embisto.New(opts), shared, loaded, log, measured)
	var draining atomic.Bool
	deps := api.Deps{
		Log: log, Previews: previews, Renderers: renderers, PublicURL: loaded.PublicURL, Requests: measured, Thumbnails: served,
		Draining: draining.Load,
	}
	metricsMux := http.NewServeMux()
	metricsMux.Handle("GET /metrics", measured.Handler())
	if loaded.Debug {
		metricsMux.Handle("GET /debug", debugPage(ctx, opts, served, log))
	}

	return serve(ctx, &draining, log,
		newServer(loaded.Addr, api.NewServer(deps)),
		newServer(loaded.MetricsAddr, metricsMux),
	)
}

// sharedStore is the store instances share, when the settings name one; nil keeps each instance's own.
func sharedStore(loaded settings.Settings, log *slog.Logger, measured *metrics.Metrics) (*cache.Redis, error) {
	if loaded.CacheURL == "" {
		return nil, nil //nolint:nilnil // no shared store is no failure
	}
	shared, err := cache.NewRedis(loaded.CacheURL, cache.RedisOptions{Fallback: cache.NewMemory(max(1<<20, loaded.CacheMemory), nil), Log: log, Observer: measured})
	if err != nil {
		return nil, fmt.Errorf("the shared cache: %w", err)
	}
	measured.RedisUp(true)
	return shared, nil
}

// layered wraps the core in the server's cache: the shared store, whose instances agree through its locks, or this
// instance's memory.
func layered(core card.Extractor, shared *cache.Redis, loaded settings.Settings, log *slog.Logger, measured *metrics.Metrics) card.Extractor {
	opts := cache.Options{
		TTL: loaded.CacheTTL, MinTTL: loaded.CacheMinTTL, MaxTTL: loaded.CacheMaxTTL, FailureTTL: loaded.FailureTTL,
		LockTTL: lockTTL, Log: log, Observer: measured,
	}
	switch {
	case shared != nil:
		opts.Locker = shared
		return cache.New(core, shared, opts)
	case loaded.CacheMemory > 0:
		return cache.New(core, cache.NewMemory(loaded.CacheMemory, nil), opts)
	default:
		return core
	}
}

// newServer bounds every phase of a connection, so slow or stalled clients cannot hold the server's connections.
func newServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr: addr, Handler: handler,
		ReadHeaderTimeout: headerTimeout, ReadTimeout: readTimeout, WriteTimeout: writeTimeout, IdleTimeout: idleTimeout,
	}
}

// serve runs the servers until the context ends or one fails, then stops them all.
func serve(ctx context.Context, draining *atomic.Bool, log *slog.Logger, servers ...*http.Server) error {
	failed := make(chan error, len(servers))
	for _, server := range servers {
		go func() {
			log.InfoContext(ctx, "serving", slog.String("addr", server.Addr))
			failed <- server.ListenAndServe()
		}()
	}
	select {
	case err := <-failed:
		return fmt.Errorf("serving: %w", err)
	case <-ctx.Done():
	}
	draining.Store(true)
	log.InfoContext(ctx, "stopping: not ready, still serving", slog.Duration("for", unreadyFor))
	time.Sleep(unreadyFor)
	shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
	defer cancel()
	for _, server := range servers {
		if err := server.Shutdown(shutdown); err != nil {
			return fmt.Errorf("shutting down: %w", err)
		}
	}
	for range servers {
		if err := <-failed; !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serving: %w", err)
		}
	}
	return nil
}
