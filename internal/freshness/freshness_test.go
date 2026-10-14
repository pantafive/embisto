package freshness_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/freshness"
)

func TestOf(t *testing.T) {
	const date = "Tue, 13 Oct 2026 12:00:00 GMT"
	tests := []struct {
		name   string
		header http.Header
		want   card.Freshness
	}{
		{"nothing said", http.Header{}, card.Freshness{}},
		{"max-age", http.Header{"Cache-Control": {"public, max-age=600"}}, card.Freshness{MaxAge: 10 * time.Minute, Known: true}},
		{"s-maxage before max-age", http.Header{"Cache-Control": {"max-age=60, s-maxage=3600"}}, card.Freshness{MaxAge: time.Hour, Known: true}},
		{"age taken off", http.Header{"Cache-Control": {"max-age=600"}, "Age": {"100"}}, card.Freshness{MaxAge: 500 * time.Second, Known: true}},
		{"older than its max-age", http.Header{"Cache-Control": {"max-age=60"}, "Age": {"100"}}, card.Freshness{Known: true}},
		{"no-store", http.Header{"Cache-Control": {"no-store, max-age=600"}}, card.Freshness{Known: true}},
		{"private", http.Header{"Cache-Control": {"private, max-age=600"}}, card.Freshness{Known: true}},
		{"no-cache", http.Header{"Cache-Control": {"no-cache"}}, card.Freshness{Known: true}},
		{"Expires against Date", http.Header{"Expires": {"Tue, 13 Oct 2026 14:00:00 GMT"}, "Date": {date}}, card.Freshness{MaxAge: 2 * time.Hour, Known: true}},
		{"Expires that is no date", http.Header{"Expires": {"-1"}, "Date": {date}}, card.Freshness{Known: true}},
		{"Expires without Date", http.Header{"Expires": {"Tue, 13 Oct 2026 14:00:00 GMT"}}, card.Freshness{}},
		{"max-age before Expires", http.Header{"Cache-Control": {"max-age=60"}, "Expires": {"Tue, 13 Oct 2026 14:00:00 GMT"}, "Date": {date}}, card.Freshness{MaxAge: time.Minute, Known: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, freshness.Of(tt.header))
		})
	}
}
