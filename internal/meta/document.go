// Package meta parses a page's metadata and reads the generic sources from it: Open Graph, Twitter Cards and plain
// HTML.
package meta

import (
	"bytes"
	"cmp"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/pantafive/embisto/card"
)

// Parse reads the metadata of a fetched page. Tags are read from the whole document, since some sites put them in the
// body; the title only from the head, since an <svg> in the body has its own.
func Parse(resp card.Response) card.Page {
	p := parser{page: card.Page{Response: resp, Base: resp.URL}, tokens: html.NewTokenizer(bytes.NewReader(resp.Body))}
	for {
		switch p.tokens.Next() { //nolint:exhaustive // text and end tags carry no metadata
		case html.ErrorToken:
			return p.page
		case html.StartTagToken, html.SelfClosingTagToken:
			p.element(p.tokens.Token())
		}
	}
}

type parser struct {
	page    card.Page
	tokens  *html.Tokenizer
	hasBase bool
	inBody  bool
}

func (p *parser) element(token html.Token) {
	switch token.DataAtom { //nolint:exhaustive // only these elements carry metadata
	case atom.Body:
		p.inBody = true
	case atom.Title:
		p.title()
	case atom.Base:
		p.base(token)
	case atom.Meta:
		addTag(&p.page, token, p.inBody)
	case atom.Link:
		addLink(&p.page, token)
	case atom.Script:
		p.linkedData(token)
	}
}

// linkedData keeps a JSON-LD block's text; the tokenizer gives a script's whole body as one text token.
func (p *parser) linkedData(token html.Token) {
	if !strings.EqualFold(strings.TrimSpace(attr(token, "type")), "application/ld+json") {
		return
	}
	if p.tokens.Next() == html.TextToken {
		p.page.LinkedData = append(p.page.LinkedData, string(p.tokens.Text()))
	}
}

func (p *parser) title() {
	if !p.inBody && p.page.Title == "" && p.tokens.Next() == html.TextToken {
		p.page.Title = clean(string(p.tokens.Text()))
	}
}

// base takes the first <base href>, as browsers do.
func (p *parser) base(token html.Token) {
	if base, err := url.Parse(p.page.Resolve(attr(token, "href"))); err == nil && base.Host != "" && !p.hasBase {
		p.page.Base, p.hasBase = base, true
	}
}

// addTag keeps a <meta> by its property, name or itemprop. An itemprop in the body belongs to whatever item it sits
// in, such as a comment or a related article, never to the page: only the head's speak for the page.
func addTag(page *card.Page, token html.Token, inBody bool) {
	named := cmp.Or(attr(token, "property"), attr(token, "name"))
	if named == "" && inBody {
		return
	}
	key := strings.ToLower(cmp.Or(named, attr(token, "itemprop")))
	value := clean(attr(token, "content"))
	if key != "" && value != "" {
		page.Tags = append(page.Tags, card.Tag{Key: key, Value: value})
	}
}

func addLink(page *card.Page, token html.Token) {
	href := page.Resolve(attr(token, "href"))
	if href == "" {
		return
	}
	page.Links = append(page.Links, card.PageLink{
		Rel:   strings.Fields(strings.ToLower(attr(token, "rel"))),
		Href:  href,
		Type:  strings.ToLower(attr(token, "type")),
		Sizes: strings.ToLower(attr(token, "sizes")),
	})
}

func attr(token html.Token, name string) string {
	for _, a := range token.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// clean collapses the whitespace runs markup leaves in text.
func clean(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
