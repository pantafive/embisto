package fixture

import (
	"bytes"
	"cmp"
	"encoding/json"
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Trim drops what no source reads from a page, so a saved page is small, and holds nothing of the people who visited
// or commented on it: scripts other than JSON-LD, styles and comments everywhere, in the body everything but its
// metadata, every <meta> and <link> no source reads, such as a visitor's payload or a request's id, every attribute
// of <html>, <head>, <body> and <base> but the base's href, and in JSON-LD every key no source reads, such as the
// comments under a video.
func Trim(page []byte) []byte {
	t := trimmer{tokens: html.NewTokenizer(bytes.NewReader(page))}
	for {
		kind := t.tokens.Next()
		if kind == html.ErrorToken {
			return t.out.Bytes()
		}
		t.token(kind)
	}
}

// metaPrefixes and metaNames are the <meta> keys a source reads, by property, name or itemprop.
var (
	metaPrefixes = []string{"og:", "twitter:", "article:", "video:"}
	metaNames    = []string{
		"description", "author", "keywords", "application-name", "datepublished", "uploaddate", "date", "name",
		"headline", "image", "thumbnailurl", "embedurl", "duration",
	}
	// linkRels are the <link> relations a source reads.
	linkRels = []string{"icon", "shortcut", "apple-touch-icon", "apple-touch-icon-precomposed", "canonical", "alternate", "image_src"}
	// LinkedDataKeys are the JSON-LD keys a source reads; any other is dropped from a saved page.
	LinkedDataKeys = []string{
		"@context", "@graph", "@type", "@id", "@value", "author", "contentUrl", "datePublished", "description",
		"embedUrl", "encodingFormat", "headline", "height", "image", "keywords", "name", "publisher", "thumbnailUrl",
		"uploadDate", "url", "width",
	}
)

type trimmer struct {
	tokens *html.Tokenizer
	out    bytes.Buffer
	// inTitle is set within the title, whose text is kept; inLinkedData within a JSON-LD block, whose text is kept
	// with only the keys a source reads.
	inTitle, inLinkedData, inBody bool
}

func (t *trimmer) token(kind html.TokenType) {
	raw := append([]byte(nil), t.tokens.Raw()...)
	switch kind { //nolint:exhaustive // doctypes and comments are handled below
	case html.TextToken:
		switch {
		case t.inLinkedData:
			t.out.Write(linkedData(raw))
		case t.inTitle:
			t.out.Write(raw)
		}
	case html.StartTagToken, html.SelfClosingTagToken:
		t.start(kind, raw)
	case html.EndTagToken:
		if t.keepsEnd() {
			t.out.Write(raw)
		}
	case html.DoctypeToken:
		t.out.Write(raw)
	}
}

func (t *trimmer) start(kind html.TokenType, raw []byte) {
	token := t.tokens.Token()
	switch {
	case !t.keepsStart(token, kind == html.StartTagToken):
	case token.DataAtom == atom.Script:
		// Written bare: a script's other attributes, such as a nonce, are no source's business.
		t.out.WriteString(`<script type="application/ld+json">`)
	case token.DataAtom == atom.Base:
		t.out.WriteString(`<base href="` + html.EscapeString(attr(token, "href")) + `">`)
	case token.DataAtom == atom.Html, token.DataAtom == atom.Head, token.DataAtom == atom.Body:
		// Written bare too: no source reads their attributes, which may name an experiment or a visitor's theme.
		t.out.WriteString("<" + token.Data + ">")
	default:
		t.out.Write(raw)
	}
}

// keepsStart keeps the elements a source reads, and nothing else: a script, a style or a <div> may carry a nonce,
// a build's id or a visitor's.
func (t *trimmer) keepsStart(token html.Token, opens bool) bool {
	switch token.DataAtom { //nolint:exhaustive // every other element is dropped
	case atom.Body:
		t.inBody = true
		return true
	case atom.Html, atom.Head, atom.Base:
		return true
	case atom.Title:
		// A title in the body is an SVG's, never the page's.
		t.inTitle = opens && !t.inBody
		return !t.inBody
	case atom.Meta:
		return isReadMeta(token)
	case atom.Link:
		return isReadLink(token)
	case atom.Script:
		if isLinkedData(token) {
			t.inLinkedData = opens
			return true
		}
	}
	return false
}

func (t *trimmer) keepsEnd() bool {
	name, _ := t.tokens.TagName()
	switch atom.Lookup(name) { //nolint:exhaustive // every other element is dropped
	case atom.Html, atom.Head, atom.Body:
		return true
	case atom.Title:
		t.inTitle = false
		return !t.inBody
	case atom.Script:
		wasLinkedData := t.inLinkedData
		t.inLinkedData = false
		return wasLinkedData
	}
	return false
}

// isReadMeta says whether a source reads the <meta>: a charset, or a key it knows.
func isReadMeta(token html.Token) bool {
	if attr(token, "charset") != "" || strings.EqualFold(attr(token, "http-equiv"), "content-type") {
		return true
	}
	key := strings.ToLower(cmp.Or(attr(token, "property"), attr(token, "name"), attr(token, "itemprop")))
	return slices.Contains(metaNames, key) || slices.ContainsFunc(metaPrefixes, func(prefix string) bool { return strings.HasPrefix(key, prefix) })
}

func isReadLink(token html.Token) bool {
	return slices.ContainsFunc(strings.Fields(strings.ToLower(attr(token, "rel"))), func(rel string) bool { return slices.Contains(linkRels, rel) })
}

// linkedData is a JSON-LD block with only the keys a source reads; nothing when it is not JSON.
func linkedData(raw []byte) []byte {
	var block any
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil
	}
	kept, err := json.Marshal(keepRead(block))
	if err != nil {
		return nil
	}
	return kept
}

func keepRead(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		kept := map[string]any{}
		for key, child := range typed {
			if slices.Contains(LinkedDataKeys, key) {
				kept[key] = keepRead(child)
			}
		}
		return kept
	case []any:
		kept := make([]any, len(typed))
		for i, child := range typed {
			kept[i] = keepRead(child)
		}
		return kept
	default:
		return value
	}
}

func isLinkedData(token html.Token) bool {
	return strings.EqualFold(strings.TrimSpace(attr(token, "type")), "application/ld+json")
}

func attr(token html.Token, name string) string {
	for _, a := range token.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}
