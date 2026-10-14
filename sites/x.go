package sites

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/oembed"
)

// xEndpoint is X's oEmbed endpoint, which answers for x.com and twitter.com posts alike.
const xEndpoint = "https://publish.x.com/oembed"

var xPost = regexp.MustCompile(`^/[A-Za-z0-9_]{1,15}/status/[0-9]+/?$`)

// X answers for posts on X and Twitter from X's oEmbed endpoint alone: their pages show nothing without JavaScript.
// A deleted or unknown post is not_found. The card's text is the post's own, read from the embed, which is served as
// X made it.
type X struct{}

func (X) Name() string {
	return "x"
}

func (X) Match(u *url.URL) card.Match {
	switch strings.TrimPrefix(strings.TrimPrefix(u.Hostname(), "www."), "mobile.") {
	case "x.com", "twitter.com":
		if xPost.MatchString(u.Path) {
			return card.MatchedAlone
		}
	}
	return card.Unmatched
}

func (X) Provide(ctx context.Context, in card.Input) (card.Facts, error) {
	post := url.URL{Scheme: schemeHTTPS, Host: "x.com", Path: in.URL.Path}
	endpoint, err := oembed.Endpoint(xEndpoint, &post, in.Options)
	if err != nil {
		return card.Facts{}, err //nolint:wrapcheck // it names the endpoint
	}
	facts, err := oembed.Get(ctx, in.Fetcher, endpoint, xEndpoint)
	if err != nil {
		return card.Facts{}, gone(err, http.StatusNotFound)
	}
	// The endpoint names the author but gives no title: the post's text is in its blockquote.
	if facts.Author != "" {
		facts.Title = facts.Author + " on X"
	}
	facts.Description = firstParagraph(facts.Embed.HTML)
	facts.CanonicalURL = post.String()
	return facts, nil
}

// firstParagraph is the text of the first <p> of a fragment.
func firstParagraph(fragment string) string {
	tokens := html.NewTokenizer(strings.NewReader(fragment))
	var text strings.Builder
	depth := 0
	for {
		switch tokens.Next() { //nolint:exhaustive // only the paragraph's tags and text matter
		case html.ErrorToken:
			return strings.Join(strings.Fields(text.String()), " ")
		case html.StartTagToken:
			if name, _ := tokens.TagName(); atom.Lookup(name) == atom.P {
				depth++
			}
		case html.EndTagToken:
			if name, _ := tokens.TagName(); atom.Lookup(name) == atom.P && depth > 0 {
				return strings.Join(strings.Fields(text.String()), " ")
			}
		case html.TextToken:
			if depth > 0 {
				text.Write(tokens.Text())
			}
		}
	}
}
