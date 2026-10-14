package api

import (
	"net/http"
	"strings"
	"time"
)

// RequestObserver hears about every request answered: its route pattern, never its URL, so the labels stay few.
type RequestObserver interface {
	Observed(route string, status int, took time.Duration)
}

// unmatched is the route of a request no route claimed.
const unmatched = "unmatched"

// observed reports each request with the route the mux matched it to; the mux records the pattern on the request it
// is given, so this sits directly around it.
func observed(mux *http.ServeMux, observer RequestObserver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		mux.ServeHTTP(recorder, r)
		route := unmatched
		if _, path, ok := strings.Cut(r.Pattern, " "); ok {
			route = path
		}
		observer.Observed(route, recorder.status, time.Since(started))
	})
}

type statusRecorder struct {
	http.ResponseWriter

	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(status int) {
	if !s.wroteHeader {
		s.status, s.wroteHeader = status, true
	}
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}
