package pipeline

import (
	"unicode/utf8"

	"github.com/pantafive/embisto/card"
)

// What one source or provider may contribute to a card, so that a page cannot make the card, or the work of merging
// it, as large as it likes: a page of a hundred thousand pictures gives the first few.
const (
	maxPictures    = 16
	maxVideos      = 8
	maxLinks       = 16
	maxBoilerplate = 8
	maxTitle       = 512
	maxText        = 4096
	maxURL         = 4096
	maxEmbed       = 64 << 10
)

// bound cuts the facts to what one source may contribute: lists to their first items, texts to their first runes,
// and a URL longer than any a client would open dropped.
func bound(facts card.Facts) card.Facts {
	facts.Title = runes(facts.Title, maxTitle)
	facts.Description = runes(facts.Description, maxText)
	facts.SiteName = runes(facts.SiteName, maxTitle)
	facts.Author = runes(facts.Author, maxTitle)
	facts.Keywords = runes(facts.Keywords, maxText)
	facts.AuthorURL = address(facts.AuthorURL)
	facts.CanonicalURL = address(facts.CanonicalURL)
	facts.Icons = first(addressed(facts.Icons, func(image card.Image) string { return image.URL }), maxPictures)
	facts.Images = first(addressed(facts.Images, func(image card.Image) string { return image.URL }), maxPictures)
	facts.Videos = first(addressed(facts.Videos, func(video card.Video) string { return video.URL }), maxVideos)
	if len(facts.Embed.HTML) > maxEmbed {
		facts.Embed = card.Embed{}
	}
	facts.Links = first(addressed(facts.Links, func(link card.Link) string { return link.Href }), maxLinks)
	for i := range facts.Links {
		if len(facts.Links[i].HTML) > maxEmbed {
			facts.Links[i].HTML = ""
		}
	}
	facts.Boilerplate.Texts = first(facts.Boilerplate.Texts, maxBoilerplate)
	for i, text := range facts.Boilerplate.Texts {
		facts.Boilerplate.Texts[i] = runes(text, maxText)
	}
	facts.Boilerplate.Images = first(facts.Boilerplate.Images, maxBoilerplate)
	return facts
}

// addressed are the items whose URL a client would open: no longer than maxURL. It stops looking once it has more
// than any list may keep, so that a page's hundred thousand pictures cost no more than its first few.
func addressed[T any](list []T, href func(T) string) []T {
	var kept []T
	for _, item := range list {
		if len(href(item)) <= maxURL {
			kept = append(kept, item)
		}
		if len(kept) > maxPictures {
			break
		}
	}
	return kept
}

// first is the list's first items, in a list of its own, so that cutting never keeps the rest alive.
func first[T any](list []T, limit int) []T {
	if len(list) <= limit {
		return list
	}
	return append([]T(nil), list[:limit]...)
}

// runes is the text's first runes.
func runes(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := 0
	for i := 0; i < limit && cut < len(text); i++ {
		_, size := utf8.DecodeRuneInString(text[cut:])
		cut += size
	}
	return text[:cut]
}

func address(raw string) string {
	if len(raw) > maxURL {
		return ""
	}
	return raw
}
