package assemble

import (
	"path"
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/pantafive/embisto/card"
)

// typeHTML is the media type of a page a player or an app is framed from.
const typeHTML = "text/html"

// imageTypes guess a picture's media type from its extension, since pages rarely state it.
var imageTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp",
	".svg": "image/svg+xml", ".ico": "image/x-icon", ".avif": "image/avif",
}

// links are every resource every source named, providers first, each href once with the relations and the sources
// of all that named it.
func links(page string, order []card.Facts) []card.Link {
	var all []card.Link
	seen := map[string]int{}
	add := func(link card.Link, source string) {
		if link.Href == "" {
			return
		}
		link.Rel = append(link.Rel, source)
		at, ok := seen[link.Href]
		if !ok {
			seen[link.Href] = len(all)
			all = append(all, link)
			return
		}
		all[at] = merged(all[at], link)
	}
	for _, facts := range order {
		for _, link := range linksOf(page, facts) {
			add(link, facts.Source)
		}
	}
	return all
}

func linksOf(page string, facts card.Facts) []card.Link {
	var found []card.Link
	for _, icon := range facts.Icons {
		found = append(found, card.Link{Href: icon.URL, Rel: []string{card.RelIcon}, Type: imageType(icon.URL), Media: card.MediaOf(icon.Width, icon.Height)})
	}
	for i, image := range facts.Images {
		rel := card.RelThumbnail
		if i == 0 && facts.Type == card.TypePhoto {
			rel = card.RelImage
		}
		found = append(found, card.Link{Href: image.URL, Rel: []string{rel}, Type: imageType(image.URL), Media: card.MediaOf(image.Width, image.Height)})
	}
	for _, video := range facts.Videos {
		rel := []string{card.RelPlayer}
		if video.Type != "" && video.Type != typeHTML {
			rel = append(rel, card.RelFile)
		}
		found = append(found, card.Link{Href: video.URL, Rel: rel, Type: video.Type, Media: card.MediaOf(video.Width, video.Height)})
	}
	if facts.Embed.HTML != "" {
		found = append(found, embedLink(page, facts))
	}
	return append(found, facts.Links...)
}

// embedLink is the provider's HTML as a link. HTML that frames a page is a player of that page: a video keeps its
// proportions, anything else, such as an audio widget, its height. Other HTML is an app for the page itself, as wide
// as the provider made it.
func embedLink(page string, facts card.Facts) card.Link {
	src := IframeSource(facts.Embed.HTML)
	switch {
	case src != "" && facts.Type == card.TypeVideo:
		return card.Link{Href: src, Rel: []string{card.RelPlayer}, Type: typeHTML, HTML: facts.Embed.HTML, Media: card.MediaOf(facts.Embed.Width, facts.Embed.Height)}
	case src != "":
		return card.Link{Href: src, Rel: []string{card.RelPlayer}, Type: typeHTML, HTML: facts.Embed.HTML, Media: card.Media{Height: facts.Embed.Height}}
	default:
		return card.Link{Href: page, Rel: []string{card.RelApp}, Type: typeHTML, HTML: facts.Embed.HTML, Media: card.Media{Width: facts.Embed.Width}}
	}
}

// IframeSource is the address the first iframe in the HTML shows; empty for none.
func IframeSource(fragment string) string {
	tokens := html.NewTokenizer(strings.NewReader(fragment))
	for {
		switch tokens.Next() { //nolint:exhaustive // only the iframe matters
		case html.ErrorToken:
			return ""
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokens.Token()
			if token.DataAtom != atom.Iframe {
				continue
			}
			for _, a := range token.Attr {
				if a.Key == "src" && (strings.HasPrefix(a.Val, "https://") || strings.HasPrefix(a.Val, "http://")) {
					return a.Val
				}
			}
		}
	}
}

// merged adds what a later source said about a link the first did not: relations, a type, a size, the HTML.
func merged(link, later card.Link) card.Link {
	for _, rel := range later.Rel {
		if !slices.Contains(link.Rel, rel) {
			link.Rel = append(link.Rel, rel)
		}
	}
	if link.Type == "" {
		link.Type = later.Type
	}
	if link.Media.Width == 0 {
		link.Media = later.Media
	}
	if link.HTML == "" {
		link.HTML = later.HTML
	}
	return link
}

func imageType(href string) string {
	before, _, _ := strings.Cut(href, "?")
	return imageTypes[strings.ToLower(path.Ext(before))]
}
