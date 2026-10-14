package assemble

import (
	"slices"
	"strings"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/trace"
)

// boilerplate is every text and picture some source knows to be the site's own rather than the item's.
type boilerplate struct {
	texts  []string
	images []string
}

func boilerplateOf(all []card.Facts) boilerplate {
	var known boilerplate
	for _, facts := range all {
		for _, text := range facts.Boilerplate.Texts {
			known.texts = append(known.texts, normalized(text))
		}
		known.images = append(known.images, facts.Boilerplate.Images...)
	}
	return known
}

// scrubbed are the facts without the boilerplate, so that a field falls through to the next source's value: an
// empty field is better than a wrong one, and a real value further down is better still.
func (b boilerplate) scrubbed(t *trace.Trace, all []card.Facts) []card.Facts {
	if len(b.texts) == 0 && len(b.images) == 0 {
		return all
	}
	dropped := func(field, source, value string) {
		t.Decide(trace.Decision{Field: field, Candidates: []trace.Candidate{{Source: source, Value: value}}, Reason: "dropped: the site says this of every page"})
	}
	cleaned := make([]card.Facts, len(all))
	for i, facts := range all {
		if facts.Title != "" && slices.Contains(b.texts, normalized(facts.Title)) {
			dropped("title", facts.Source, facts.Title)
			facts.Title = ""
		}
		if facts.Description != "" && slices.Contains(b.texts, normalized(facts.Description)) {
			dropped("description", facts.Source, facts.Description)
			facts.Description = ""
		}
		facts.Images = slices.DeleteFunc(slices.Clone(facts.Images), func(image card.Image) bool {
			return slices.Contains(b.images, image.URL)
		})
		cleaned[i] = facts
	}
	return cleaned
}

// normalized compares texts as the page parser writes them: whitespace collapsed.
func normalized(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
