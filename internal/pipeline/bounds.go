package pipeline

import (
	"strings"
	"unicode"
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

// bound cuts the facts to what one source may contribute: lists to their first items, texts made plain and cut to
// their first runes, and a URL longer than any a client would open dropped.
func bound(facts card.Facts) card.Facts {
	facts.Title = runes(plain(facts.Title), maxTitle)
	facts.Description = runes(plain(facts.Description), maxText)
	facts.SiteName = runes(plain(facts.SiteName), maxTitle)
	facts.Author = runes(plain(facts.Author), maxTitle)
	facts.Keywords = runes(plain(facts.Keywords), maxText)
	facts.AuthorURL = address(facts.AuthorURL)
	facts.CanonicalURL = address(facts.CanonicalURL)
	facts.Icons = first(addressed(facts.Icons, func(image card.Image) string { return image.URL }), maxPictures)
	facts.Images = first(addressed(facts.Images, func(image card.Image) string { return image.URL }), maxPictures)
	facts.Videos = first(addressed(facts.Videos, func(video card.Video) string { return video.URL }), maxVideos)
	if len(facts.Embed.HTML) > maxEmbed {
		facts.Embed = card.Embed{}
	}
	facts.Embed.Provider = runes(plain(facts.Embed.Provider), maxTitle)
	facts.Links = first(addressed(facts.Links, func(link card.Link) string { return link.Href }), maxLinks)
	for i := range facts.Links {
		if len(facts.Links[i].HTML) > maxEmbed {
			facts.Links[i].HTML = ""
		}
	}
	facts.Boilerplate.Texts = first(facts.Boilerplate.Texts, maxBoilerplate)
	for i, text := range facts.Boilerplate.Texts {
		facts.Boilerplate.Texts[i] = runes(plain(text), maxText)
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

// plain is a text a client may print as it is: without control characters, such as a terminal's escapes, or the
// bidi controls that reorder the text around it, and with its whitespace runs collapsed to one space.
func plain(text string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && !unicode.IsSpace(r) || '\u202a' <= r && r <= '\u202e' || '\u2066' <= r && r <= '\u2069' {
			return -1
		}
		return r
	}, text)), " ")
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
