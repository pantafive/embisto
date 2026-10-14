// Package freshness reads how long an origin lets a response be kept, as a shared cache reads it.
package freshness

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pantafive/embisto/card"
)

// uncacheable are the directives under which a shared cache keeps nothing.
var uncacheable = []string{"no-store", "no-cache", "private"}

// Of reads the headers: s-maxage before max-age, since embisto is a shared cache; otherwise Expires against the
// response's own Date, never the local clock, so a skewed clock changes nothing; Age taken off either. An Expires
// that is not a date means already expired.
func Of(header http.Header) card.Freshness {
	directives := parse(header.Values("Cache-Control"))
	for _, name := range uncacheable {
		if _, ok := directives[name]; ok {
			return card.Freshness{Known: true}
		}
	}
	maxAge, ok := seconds(directives, "s-maxage")
	if !ok {
		maxAge, ok = seconds(directives, "max-age")
	}
	if !ok {
		maxAge, ok = expires(header)
	}
	if !ok {
		return card.Freshness{}
	}
	if age, err := strconv.Atoi(strings.TrimSpace(header.Get("Age"))); err == nil && age > 0 {
		maxAge -= time.Duration(age) * time.Second
	}
	return card.Freshness{MaxAge: max(0, maxAge), Known: true}
}

func parse(values []string) map[string]string {
	directives := map[string]string{}
	for _, value := range values {
		for directive := range strings.SplitSeq(value, ",") {
			name, argument, _ := strings.Cut(strings.TrimSpace(directive), "=")
			directives[strings.ToLower(strings.TrimSpace(name))] = strings.Trim(strings.TrimSpace(argument), `"`)
		}
	}
	return directives
}

func seconds(directives map[string]string, name string) (time.Duration, bool) {
	raw, ok := directives[name]
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, true
	}
	return time.Duration(n) * time.Second, true
}

// expires is Expires minus Date; false when either is missing, true and zero when Expires is not a date.
func expires(header http.Header) (time.Duration, bool) {
	raw := header.Get("Expires")
	if raw == "" {
		return 0, false
	}
	at, err := http.ParseTime(raw)
	if err != nil {
		return 0, true
	}
	date, err := http.ParseTime(header.Get("Date"))
	if err != nil {
		return 0, false
	}
	return at.Sub(date), true
}
