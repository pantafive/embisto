package sites

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/oembed"
)

const tiktokOEmbed = "https://www.tiktok.com/oembed"

var (
	tiktokVideo = regexp.MustCompile(`^/@[^/]+/(video|photo)/([0-9]+)/?$`)
	tiktokShort = regexp.MustCompile(`^/(t/)?[A-Za-z0-9]+/?$`)
)

// TikTok answers for videos and photo posts from TikTok's oEmbed endpoint: its pages need JavaScript. A short link
// (vm.tiktok.com, tiktok.com/t/) is followed to the post first. TikTok answers a missing post and a bad request
// alike, so a failure is never taken for not_found.
type TikTok struct{}

func (TikTok) Name() string {
	return "tiktok"
}

func (TikTok) Match(u *url.URL) card.Match {
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	switch {
	case (host == "tiktok.com" || host == "m.tiktok.com") && tiktokVideo.MatchString(u.Path),
		(host == "vm.tiktok.com" || host == "vt.tiktok.com") && tiktokShort.MatchString(u.Path),
		host == "tiktok.com" && strings.HasPrefix(u.Path, "/t/"):
		return card.MatchedAlone
	}
	return card.Unmatched
}

func (TikTok) Provide(ctx context.Context, in card.Input) (card.Facts, error) {
	post := in.URL
	if !tiktokVideo.MatchString(post.Path) {
		resp, err := in.Fetcher.Fetch(ctx, post, "text/html")
		if err != nil {
			return card.Facts{}, fmt.Errorf("following the short link: %w", err)
		}
		post = resp.URL
	}
	match := tiktokVideo.FindStringSubmatch(post.Path)
	if match == nil {
		return card.Facts{}, nil
	}
	canonical := url.URL{Scheme: schemeHTTPS, Host: "www.tiktok.com", Path: strings.TrimSuffix(post.Path, "/")}
	endpoint, err := oembed.Endpoint(tiktokOEmbed, &canonical, in.Options)
	if err != nil {
		return card.Facts{}, err //nolint:wrapcheck // it names the endpoint
	}
	facts, err := oembed.Get(ctx, in.Fetcher, endpoint, tiktokOEmbed)
	if err != nil {
		return card.Facts{}, err //nolint:wrapcheck // it says which step failed
	}
	facts.SiteName, facts.CanonicalURL = "TikTok", canonical.String()
	if match[1] == "video" {
		// TikTok's own player, upright, for a client that frames a player rather than running TikTok's script.
		facts.Videos = []card.Video{{URL: "https://www.tiktok.com/player/v1/" + match[2], Type: "text/html", Width: 325, Height: 578}}
	}
	return facts, nil
}
