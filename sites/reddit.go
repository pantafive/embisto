package sites

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/oembed"
)

const redditOEmbed = "https://www.reddit.com/oembed"

var redditPost = regexp.MustCompile(`^/r/[A-Za-z0-9_]+/comments/[a-z0-9]+(/[^/]*)?/?$`)

// Reddit answers for posts from Reddit's oEmbed endpoint alone: Reddit refuses most servers' requests for its pages.
// A deleted or unknown post is not_found.
type Reddit struct{}

func (Reddit) Name() string {
	return "reddit"
}

func (Reddit) Match(u *url.URL) card.Match {
	switch strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.") {
	case "reddit.com", "old.reddit.com", "new.reddit.com", "m.reddit.com":
		if redditPost.MatchString(u.Path) {
			return card.MatchedAlone
		}
	}
	return card.Unmatched
}

func (Reddit) Provide(ctx context.Context, in card.Input) (card.Facts, error) {
	post := url.URL{Scheme: schemeHTTPS, Host: "www.reddit.com", Path: in.URL.Path}
	endpoint, err := oembed.Endpoint(redditOEmbed, &post, in.Options)
	if err != nil {
		return card.Facts{}, err //nolint:wrapcheck // it names the endpoint
	}
	facts, err := oembed.Get(ctx, in.Fetcher, endpoint, redditOEmbed)
	if err != nil {
		return card.Facts{}, gone(err, http.StatusNotFound)
	}
	facts.SiteName, facts.CanonicalURL = "Reddit", post.String()
	if facts.Author != "" {
		facts.Author = "u/" + strings.TrimPrefix(facts.Author, "u/")
	}
	return facts, nil
}
