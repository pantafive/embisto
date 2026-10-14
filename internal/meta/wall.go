package meta

import (
	"net/url"
	"slices"
	"strings"

	"github.com/pantafive/embisto/card"
)

// loginPaths start the addresses sites send a visitor to when they want a login first.
var loginPaths = []string{
	"/login", "/accounts/login", "/signin", "/sign-in", "/authwall", "/uas/login", "/checkpoint", "/auth/login",
}

// Wall says whether a page is a wall in place of the item asked for, and which: a redirect to a login or consent
// page, or a page that names only its site. Every signal is strong, so that no real page is taken for a wall: the
// address changed to a login or consent page, or a deep page's title is the site's bare name with nothing else
// said about the item. An empty limit is no wall.
func Wall(requested *url.URL, page card.Page) card.Limit {
	final := page.URL
	if final == nil {
		return ""
	}
	if strings.HasPrefix(final.Hostname(), "consent.") && !strings.HasPrefix(requested.Hostname(), "consent.") {
		return card.LimitConsentWall
	}
	if isLogin(requested.Path) {
		return ""
	}
	if isLogin(final.Path) {
		return card.LimitLoginWall
	}
	if isBareSite(requested, page) {
		return card.LimitBlocked
	}
	return ""
}

func isLogin(path string) bool {
	lowered := strings.ToLower(path)
	return slices.ContainsFunc(loginPaths, func(prefix string) bool {
		return lowered == prefix || strings.HasPrefix(lowered, prefix+"/") || strings.HasPrefix(lowered, prefix+".")
	})
}

// isBareSite is a page below the site's root whose title is only the site's name, with no description and no
// picture: what a site serves instead of a post it will not show.
func isBareSite(requested *url.URL, page card.Page) bool {
	if strings.Trim(requested.Path, "/") == "" || page.Value("og:description", "description", "twitter:description") != "" ||
		page.Value("og:image", "og:image:url", "twitter:image") != "" {
		return false
	}
	title := strings.ToLower(strings.TrimSpace(page.Title))
	return title != "" && slices.Contains(siteNames(page), title)
}

// siteNames are the names a page's site goes by: the ones it states and its domain's own label, lowercased.
func siteNames(page card.Page) []string {
	names := []string{
		strings.ToLower(page.Value("og:site_name")), strings.ToLower(page.Value("application-name")),
	}
	labels := strings.Split(strings.TrimPrefix(page.URL.Hostname(), "www."), ".")
	if len(labels) >= 2 {
		names = append(names, labels[len(labels)-2])
	}
	return slices.DeleteFunc(names, func(name string) bool { return name == "" })
}
