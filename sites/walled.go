package sites

import (
	"context"
	"net/url"
	"strings"

	"github.com/pantafive/embisto/card"
)

// walledSites are sites that show their posts to logged-in visitors only, or refuse servers outright; keyed by
// domain, which covers its subdomains too.
var walledSites = map[string]string{
	"instagram.com": "Instagram",
	"facebook.com":  facebook,
	"fb.com":        facebook,
	"linkedin.com":  "LinkedIn",
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
	if walledSite(u) != "" {
		return card.MatchedWithPage
	}
	return card.Unmatched
}

// Provide reads a page that was served as the oembed.com list's provider would; without one, it is the honest card of
// a wall.
func (Walled) Provide(ctx context.Context, in card.Input) (card.Facts, error) {
	if in.Page.URL != nil {
		return OEmbedList{}.Provide(ctx, in)
	}
	icon := url.URL{Scheme: schemeHTTPS, Host: in.URL.Host, Path: "/favicon.ico"}
	return card.Facts{
		SiteName: walledSite(in.URL), CanonicalURL: in.URL.String(), Icons: []card.Image{{URL: icon.String()}},
		Limited: card.LimitLoginWall,
	}, nil
}

// walledSite is the name of the walled site a URL's host belongs to, such as de.linkedin.com to LinkedIn; empty for
// any other host.
func walledSite(u *url.URL) string {
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	for domain, name := range walledSites {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return name
		}
	}
	return ""
}
