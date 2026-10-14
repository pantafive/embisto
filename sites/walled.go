package sites

import (
	"context"
	"net/url"
	"strings"

	"github.com/pantafive/embisto/card"
)

// walledSites are sites that show their posts to logged-in visitors only, or refuse servers outright; keyed by host
// without www.
var walledSites = map[string]string{
	"instagram.com":  "Instagram",
	"facebook.com":   facebook,
	"m.facebook.com": facebook,
	"fb.com":         facebook,
	"linkedin.com":   "LinkedIn",
}

const facebook = "Facebook"

// Walled knows the sites that wall off their posts: Instagram, Facebook, LinkedIn. Their page is read as any other
// when they serve it, their oEmbed from the oembed.com list included; when they answer with a login wall, or refuse the request, as LinkedIn does with status 999,
// the card says so: the URL, the site's name and icon, marked limited, rather than the wall's text or an error.
type Walled struct{}

func (Walled) Name() string {
	return "walled"
}

func (Walled) Match(u *url.URL) card.Match {
	if _, ok := walledSites[strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")]; ok {
		return card.MatchedWithPage
	}
	return card.Unmatched
}

// Provide reads a page that was served as the oembed.com list's provider would; without one, it is the honest card of
// a wall.
func (Walled) Provide(ctx context.Context, in card.Input) (card.Facts, error) {
	if in.Page.URL != nil {
		return OEmbed{}.Provide(ctx, in)
	}
	host := strings.TrimPrefix(strings.ToLower(in.URL.Hostname()), "www.")
	icon := url.URL{Scheme: schemeHTTPS, Host: in.URL.Host, Path: "/favicon.ico"}
	return card.Facts{
		SiteName: walledSites[host], CanonicalURL: in.URL.String(), Icons: []card.Image{{URL: icon.String()}},
		Limited: card.LimitLoginWall,
	}, nil
}
