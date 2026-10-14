// Package metrics counts what the server does for Prometheus. Labels name routes, sources and outcomes, never URLs,
// so the number of series stays bounded.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// labelOutcome names how something ended.
const labelOutcome = "outcome"

// Metrics are the server's series in a registry of their own.
type Metrics struct {
	registry      *prometheus.Registry
	requests      *prometheus.CounterVec
	latency       *prometheus.HistogramVec
	fetches       *prometheus.HistogramVec
	matches       *prometheus.CounterVec
	lookups       *prometheus.CounterVec
	coalesced     *prometheus.CounterVec
	locks         *prometheus.CounterVec
	ttls          *prometheus.CounterVec
	backendUp     prometheus.Gauge
	backendErrors prometheus.Counter
	images        *prometheus.HistogramVec
	cards         *prometheus.CounterVec
	bytes         *prometheus.CounterVec
}

// New registers the series, along with the Go runtime's and the process's.
func New() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "embisto_http_requests_total", Help: "HTTP requests answered, by route and status.",
		}, []string{"route", "status"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "embisto_http_request_duration_seconds", Help: "How long HTTP requests took, by route.",
			Buckets: prometheus.DefBuckets,
		}, []string{"route"}),
		fetches: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "embisto_fetch_duration_seconds", Help: "How long outgoing requests took, by the stage that made them and how they ended.",
			Buckets: prometheus.DefBuckets,
		}, []string{"stage", labelOutcome}),
		matches: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "embisto_provider_matches_total", Help: "Previews a site provider was asked for.",
		}, []string{"provider"}),
		lookups: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "embisto_cache_lookups_total", Help: "Cache lookups: hit, miss, or stale for an entry found past its expiry.",
		}, []string{"result"}),
		coalesced: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "embisto_cache_coalesced_total", Help: "Requests that waited for a preview another made, in this process (local) or on another instance (shared).",
		}, []string{"scope"}),
		locks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "embisto_cache_locks_total", Help: "Shared cache locks: acquired, waited for another's preview, timeout, or error.",
		}, []string{labelOutcome}),
		ttls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "embisto_cache_ttl_source_total", Help: "Where kept entries' TTLs came from: origin, min, max, default, or failure.",
		}, []string{"source"}),
		backendUp: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "embisto_cache_backend_up", Help: "Whether the shared cache answers; 0 means memory serves in its place.",
		}),
		backendErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "embisto_cache_backend_errors_total", Help: "Shared cache operations that failed and fell back to memory.",
		}),
		images: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "embisto_image_duration_seconds", Help: "How long a thumbnail took to serve, fetching its source included, by outcome.",
			Buckets: prometheus.DefBuckets,
		}, []string{labelOutcome}),
		cards: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "embisto_cards_total",
			Help: "Cards made afresh, cache hits not counted, by class (full, partial, minimal, wall, error) and by the provider that shaped them or generic.",
		}, []string{"class", "source"}),
		bytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "embisto_image_bytes_total", Help: "Bytes of thumbnails: read from sources (in) and served (out).",
		}, []string{"direction"}),
	}
	m.registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.requests, m.latency, m.fetches, m.matches, m.lookups, m.images, m.bytes, m.cards, m.coalesced, m.locks, m.ttls, m.backendUp, m.backendErrors)
	return m
}

// Handler serves the series in the Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Fetched counts one outgoing request.
func (m *Metrics) Fetched(stage, outcome string, took time.Duration) {
	m.fetches.WithLabelValues(stage, outcome).Observe(took.Seconds())
}

// Matched counts a provider asked for a preview.
func (m *Metrics) Matched(provider string) {
	m.matches.WithLabelValues(provider).Inc()
}

// LookedUp counts a cache lookup.
func (m *Metrics) LookedUp(result string) {
	m.lookups.WithLabelValues(result).Inc()
}

// Coalesced counts a request that waited for another's preview.
func (m *Metrics) Coalesced(scope string) {
	m.coalesced.WithLabelValues(scope).Inc()
}

// Locked counts how a shared cache's lock went.
func (m *Metrics) Locked(outcome string) {
	m.locks.WithLabelValues(outcome).Inc()
}

// TTLChosen counts where a kept entry's TTL came from.
func (m *Metrics) TTLChosen(source string) {
	m.ttls.WithLabelValues(source).Inc()
}

// BackendUp records whether the shared cache answers.
func (m *Metrics) BackendUp(isUp bool) {
	value := 0.0
	if isUp {
		value = 1
	}
	m.backendUp.Set(value)
}

// BackendFailed counts an operation the shared cache failed, which memory then served.
func (m *Metrics) BackendFailed() {
	m.backendErrors.Inc()
}

// Made counts one card made afresh.
func (m *Metrics) Made(class, source string) {
	m.cards.WithLabelValues(class, source).Inc()
}

// Processed counts one thumbnail asked for; the histogram's count is the number by outcome.
func (m *Metrics) Processed(outcome string, took time.Duration, bytesIn, bytesOut int) {
	m.images.WithLabelValues(outcome).Observe(took.Seconds())
	m.bytes.WithLabelValues("in").Add(float64(bytesIn))
	m.bytes.WithLabelValues("out").Add(float64(bytesOut))
}

// Observed counts one HTTP request.
func (m *Metrics) Observed(route string, status int, took time.Duration) {
	m.requests.WithLabelValues(route, strconv.Itoa(status)).Inc()
	m.latency.WithLabelValues(route).Observe(took.Seconds())
}
