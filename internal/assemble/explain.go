package assemble

import (
	"fmt"
	"time"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/trace"
)

// fieldNames name the fields in a trace as the card's JSON does.
var fieldNames = map[field]string{
	title: "title", description: "description", siteName: "site_name", author: "author", authorURL: "author_url",
	keywords: "keywords", publishedAt: "published_at", canonicalURL: "canonical_url", icon: "icon", images: "images",
	video: "video", embed: "embed", kind: "type", duration: "duration", limited: "limited",
}

// decide records a field's candidates, in the order asked, and the one at winner; -1 is none. Without a trace it does
// nothing.
func (m *merger) decide(f field, order []card.Facts, winner int, get func(card.Facts) any, zero any) {
	if m.trace == nil {
		return
	}
	decision := trace.Decision{Field: fieldNames[f], Reason: "no source had a value"}
	for _, facts := range order {
		if value := get(facts); value != nil && value != zero {
			decision.Candidates = append(decision.Candidates, trace.Candidate{Source: facts.Source, Value: shown(value)})
		}
	}
	if winner >= 0 {
		decision.Winner = order[winner].Source
		decision.Reason = "first with a value in the field's order: site providers, then " + fmt.Sprint(priority[f])
	}
	m.trace.Decide(decision)
}

// shown is a value as a person reads it.
func shown(value any) string {
	switch v := value.(type) {
	case time.Time:
		return v.Format(time.RFC3339)
	case card.Image:
		return fmt.Sprintf("%s (%dx%d)", v.URL, v.Width, v.Height)
	case card.Video:
		return fmt.Sprintf("%s %s (%dx%d)", v.URL, v.Type, v.Width, v.Height)
	case card.Embed:
		return fmt.Sprintf("%s's HTML, %d bytes (%dx%d)", v.Provider, len(v.HTML), v.Width, v.Height)
	default:
		return fmt.Sprint(v)
	}
}
