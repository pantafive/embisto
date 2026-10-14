// Package homepage learns what a site says of every page from what its home page says: a description or a picture
// the home page shares with an item's page is the site's, not the item's.
package homepage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pantafive/embisto/cache"
	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/meta"
)

// SourceName names the source in fetch metrics; it never names a field, since it finds none.
const SourceName = "homepage"

// Options say how long a home page's words are remembered, and how long reading one may take.
type Options struct {
	TTL time.Duration
	// FailureTTL is how long a home page that said nothing is remembered as saying nothing.
	FailureTTL time.Duration
	// Timeout bounds reading a home page, within the caller's own deadline.
	Timeout time.Duration
	// Log hears about a store that failed; nil logs nothing.
	Log *slog.Logger
}

// Source offers a site's home page's description and pictures as boilerplate, read once per site and language
// within the TTL. What it remembers is in the store it is given, never in itself.
type Source struct {
	store cache.Store
	opts  Options
}

// New remembers home pages in the store.
func New(store cache.Store, opts Options) *Source {
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	return &Source{store: store, opts: opts}
}

func (*Source) Name() string {
	return SourceName
}

// said is what a home page says of itself, with the address it was served from.
type said struct {
	// Root is the home page's address after its redirects, such as a site's /en/; a page there is the home page.
	Root        string           `json:"root,omitempty"`
	Boilerplate card.Boilerplate `json:"boilerplate"`
}

func (s *Source) Extract(ctx context.Context, in card.Input) (card.Facts, error) {
	page := in.Page.URL
	if page == nil || isRoot(in.URL, "") || isRoot(page, "") || !hasAnything(in.Page) {
		return card.Facts{}, nil
	}
	key := keyOf(page.Host, card.LanguageOf(ctx))
	home, ok := s.recall(ctx, key)
	if !ok {
		home, ok = s.read(ctx, in.Fetcher, &url.URL{Scheme: page.Scheme, Host: page.Host, Path: "/"})
		if ok {
			s.remember(ctx, page.Host, key, home)
		}
	}
	if isRoot(page, home.Root) || isRoot(in.URL, home.Root) {
		return card.Facts{}, nil
	}
	return card.Facts{Source: SourceName, Boilerplate: home.Boilerplate}, nil
}

// Forget drops what the source remembers of the URL's site, in every language, and says how many entries it dropped.
func (s *Source) Forget(ctx context.Context, site *url.URL) (int, error) {
	dropped, err := s.store.DeleteGroup(ctx, groupOf(site.Host))
	if err != nil {
		return 0, fmt.Errorf("forgetting %s's home page: %w", site.Host, err)
	}
	return dropped, nil
}

// groupOf holds every language of a site's home page; the host is in braces, a Redis hash tag, so they live together.
func groupOf(host string) string {
	return "embisto:home:v1:{" + host + "}"
}

func keyOf(host, language string) string {
	return groupOf(host) + ":" + language
}

func (s *Source) recall(ctx context.Context, key string) (said, bool) {
	raw, ok, err := s.store.Get(ctx, key)
	var found said
	if err != nil || !ok || json.Unmarshal(raw, &found) != nil {
		return said{}, false
	}
	return found, true
}

func (s *Source) remember(ctx context.Context, host, key string, home said) {
	ttl := s.opts.TTL
	if len(home.Boilerplate.Texts) == 0 && len(home.Boilerplate.Images) == 0 {
		ttl = s.opts.FailureTTL
	}
	raw, err := json.Marshal(home)
	if err == nil && ttl > 0 {
		err = s.store.Set(ctx, groupOf(host), key, raw, ttl)
	}
	if err != nil {
		// It only costs the next preview of the site another read.
		s.opts.Log.WarnContext(ctx, "a home page could not be remembered", slog.String("host", host), slog.Any("error", err))
	}
}

// read is what the home page says of itself; nothing when it cannot be read. False when the caller gave up before
// it was read, which says nothing of the home page.
func (s *Source) read(ctx context.Context, fetcher card.Fetcher, home *url.URL) (said, bool) {
	reading, cancel := context.WithTimeout(ctx, s.opts.Timeout)
	defer cancel()
	resp, err := fetcher.Fetch(reading, home, "text/html")
	if ctx.Err() != nil {
		return said{}, false
	}
	if err != nil || resp.MediaType != "text/html" {
		return said{}, true
	}
	parsed := meta.Parse(resp)
	found := said{Root: resp.URL.String()}
	for _, key := range []string{"og:description", "description", "twitter:description"} {
		if text := parsed.Value(key); text != "" {
			found.Boilerplate.Texts = append(found.Boilerplate.Texts, firstRunes(text, maxText))
		}
	}
	for _, key := range []string{"og:image", "og:image:url", "twitter:image"} {
		if image := parsed.Resolve(parsed.Value(key)); image != "" {
			found.Boilerplate.Images = append(found.Boilerplate.Images, image)
		}
	}
	return found, true
}

// maxText is the most of a home page's text kept, as much as any page's text is kept for: a site cannot fill the
// store with one description.
const maxText = 4096

func firstRunes(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return string([]rune(text)[:limit])
}

// indexes are the file names a site's root is also served under.
var indexes = []string{"index.html", "index.htm", "index.php"}

// isRoot says whether the URL is a site's home page: its root, an index file there, or the address the root
// redirected to; a query of tracking parameters alone changes nothing.
func isRoot(u *url.URL, root string) bool {
	if !onlyTracking(u.Query()) {
		return false
	}
	path := strings.Trim(u.Path, "/")
	if path == "" || slices.Contains(indexes, path) {
		return true
	}
	served, err := url.Parse(root)
	return root != "" && err == nil && strings.EqualFold(served.Host, u.Host) && strings.Trim(served.Path, "/") == path
}

// onlyTracking says whether a query holds nothing but parameters that track a visit.
func onlyTracking(query url.Values) bool {
	for name := range query {
		if !strings.HasPrefix(name, "utm_") && name != "fbclid" && name != "gclid" && name != "ref" {
			return false
		}
	}
	return true
}

// hasAnything says whether the page has a description or a picture the home page could share.
func hasAnything(page card.Page) bool {
	return page.Value("og:description", "description", "twitter:description", "og:image", "og:image:url", "twitter:image") != ""
}
