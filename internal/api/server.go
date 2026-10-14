// Package api is the HTTP surface of the library: huma routes, the problem+json error shape and request ids.
package api

import (
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"runtime/debug"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/logging"
)

// Deps are everything the server needs, injected by the composition root.
type Deps struct {
	Log *slog.Logger
	// Previews makes the cards: the core, or the core in its layers.
	Previews card.Extractor
	// Renderers write the answers: json for /v1/preview, iframely and oembed for the paths of those names. A path
	// whose renderer is missing is not served.
	Renderers []card.Renderer
	// Images serves the image proxy at /i/; nil serves no pictures.
	Images *Images
	// PublicURL is where clients reach the API, named in the spec's servers; empty names none.
	PublicURL string
	formats   map[string]card.Renderer
	// Requests hears about every request answered; nil hears nothing.
	Requests RequestObserver
	// Draining says the server is stopping, so /readyz turns traffic away before the server stops serving; nil never
	// drains.
	Draining func() bool
}

// NewServer wires the HTTP surface; OpenAPI is at /openapi.json and its preview at /docs.
func NewServer(deps Deps) http.Handler {
	if deps.Log == nil {
		deps.Log = slog.New(slog.DiscardHandler)
	}
	huma.NewError = newProblem
	deps.formats = map[string]card.Renderer{}
	for _, renderer := range deps.Renderers {
		deps.formats[renderer.Name()] = renderer
	}
	mux := http.NewServeMux()
	api := humago.New(mux, spec(deps.PublicURL))
	addRoutes(api, deps)
	addReference(mux, deps.Log)
	if deps.Requests == nil {
		return identified(recovered(mux, deps.Log))
	}
	return identified(recovered(observed(mux, deps.Requests), deps.Log))
}

// recovered answers a request whose handler panicked with a bare 500 problem; net/http would only drop the
// connection.
func recovered(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { //nolint:contextcheck // logs on the request's own context
			if panicked := recover(); panicked != nil {
				if panicked == http.ErrAbortHandler { //nolint:errorlint,err113 // net/http's own sentinel, re-raised as is
					panic(panicked)
				}
				log.ErrorContext(r.Context(), "a handler panicked", slog.Any("panic", panicked), slog.String("stack", string(debug.Stack())))
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusInternalServerError)
				if _, err := io.WriteString(w, `{"title":"Internal Server Error","status":500,"detail":"the request failed"}`); err != nil {
					log.DebugContext(r.Context(), "the client left", slog.Any("error", err))
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// identified gives every request an id on the context, so its log lines read as one story.
func identified(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := fmt.Sprintf("http-%016x", rand.Uint64()) //nolint:gosec // an id, not a secret
		next.ServeHTTP(w, r.WithContext(logging.With(r.Context(), slog.String("request_id", id))))
	})
}
