package sites_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/sites"
)

func parse(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed
}

// discovered are the facts of YouTube's oEmbed as the page's discovery found them, so no request is made.
var discovered = []card.Facts{{Source: card.SourceOEmbed, Title: "A video", Embed: card.Embed{HTML: "<iframe></iframe>"}}}

func TestYouTubeReadsEveryAddressOfAnItem(t *testing.T) {
	tests := []struct {
		url, canonical, player string
	}{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "https://www.youtube.com/embed/dQw4w9WgXcQ"},
		{"https://youtu.be/dQw4w9WgXcQ?t=42", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "https://www.youtube.com/embed/dQw4w9WgXcQ?start=42"},
		{"https://m.youtube.com/watch?v=dQw4w9WgXcQ&t=1m10s", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "https://www.youtube.com/embed/dQw4w9WgXcQ?start=70"},
		{"https://music.youtube.com/watch?v=dQw4w9WgXcQ&t=1h2m3s", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "https://www.youtube.com/embed/dQw4w9WgXcQ?start=3723"},
		{"https://www.youtube.com/shorts/tPEE9ZwTmy0", "https://www.youtube.com/shorts/tPEE9ZwTmy0", "https://www.youtube.com/embed/tPEE9ZwTmy0"},
		{"https://www.youtube.com/live/dQw4w9WgXcQ", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "https://www.youtube.com/embed/dQw4w9WgXcQ"},
		{"https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "https://www.youtube.com/embed/dQw4w9WgXcQ"},
		{"https://www.youtube.com/playlist?list=PLlrxD0HtieHhS8VzuMCfQD4uJ9yne1mE6", "https://www.youtube.com/playlist?list=PLlrxD0HtieHhS8VzuMCfQD4uJ9yne1mE6", "https://www.youtube.com/embed/videoseries?list=PLlrxD0HtieHhS8VzuMCfQD4uJ9yne1mE6"},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			facts, err := sites.YouTube{}.Provide(context.Background(), card.Input{URL: parse(t, tt.url), Facts: discovered})

			require.NoError(t, err)
			require.NotEmpty(t, facts.Videos)
			assert.Equal(t, []string{tt.canonical, tt.player}, []string{facts.CanonicalURL, facts.Videos[0].URL})
		})
	}
}

func TestYouTubeEmbedsWithoutCookiesWhenAsked(t *testing.T) {
	facts, err := sites.YouTube{NoCookie: true}.Provide(context.Background(), card.Input{URL: parse(t, "https://youtu.be/dQw4w9WgXcQ"), Facts: discovered})

	require.NoError(t, err)
	require.NotEmpty(t, facts.Videos)
	assert.Equal(t, "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ", facts.Videos[0].URL)
}

func TestProvidersMatchOnlyTheirItems(t *testing.T) {
	tests := []struct {
		provider card.Provider
		url      string
		want     card.Match
	}{
		{sites.YouTube{}, "https://www.youtube.com/watch?v=dQw4w9WgXcQ", card.MatchedWithPage},
		{sites.YouTube{}, "https://www.youtube.com/@RickAstleyYT", card.Unmatched},
		{sites.YouTube{}, "https://www.youtube.com/watch?v=short", card.Unmatched},
		{sites.X{}, "https://twitter.com/jack/status/20", card.MatchedAlone},
		{sites.X{}, "https://x.com/jack", card.Unmatched},
		{sites.Reddit{}, "https://old.reddit.com/r/golang/comments/1wym3v0/hooking_into_the_go_toolchain/", card.MatchedAlone},
		{sites.Reddit{}, "https://www.reddit.com/r/golang/", card.Unmatched},
		{sites.TikTok{}, "https://www.tiktok.com/@scout2015/video/6718335390845095173", card.MatchedAlone},
		{sites.TikTok{}, "https://vm.tiktok.com/ZMhvqjqjq/", card.MatchedAlone},
		{sites.TikTok{}, "https://www.tiktok.com/@scout2015", card.Unmatched},
		{sites.Walled{}, "https://www.instagram.com/p/C0Q8vYvR5u6/", card.MatchedWithPage},
		{sites.Walled{}, "https://www.linkedin.com/in/someone", card.MatchedWithPage},
		{sites.Walled{}, "https://example.com/p/1", card.Unmatched},
	}
	for _, tt := range tests {
		t.Run(tt.provider.Name()+" "+tt.url, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.provider.Match(parse(t, tt.url)))
		})
	}
}

func TestWalledStandsInForAPageItCouldNotRead(t *testing.T) {
	facts, err := sites.Walled{}.Provide(context.Background(), card.Input{URL: parse(t, "https://www.linkedin.com/company/google")})

	require.NoError(t, err)
	assert.Equal(t, card.Facts{
		SiteName: "LinkedIn", CanonicalURL: "https://www.linkedin.com/company/google",
		Icons: []card.Image{{URL: "https://www.linkedin.com/favicon.ico"}}, Limited: card.LimitLoginWall,
	}, facts)
}

// refusing answers every request as YouTube's oEmbed answers a private video, or one whose embedding is off.
type refusing struct {
	status int
}

func (r refusing) Fetch(context.Context, *url.URL, string) (card.Response, error) {
	return card.Response{}, &card.Error{Code: card.CodeFetchFailed, Status: r.status, Err: errors.New("refused")}
}

func (r refusing) Peek(ctx context.Context, target *url.URL, _ int64) (card.Response, error) {
	return r.Fetch(ctx, target, "")
}

func TestYouTubeGivesNoPlayerForAVideoItWillNotEmbed(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			facts, err := sites.YouTube{}.Provide(context.Background(), card.Input{URL: parse(t, "https://www.youtube.com/watch?v=dQw4w9WgXcQ"), Fetcher: refusing{status: status}})

			require.NoError(t, err)
			assert.Equal(t, []any{0, ""}, []any{len(facts.Videos), facts.Embed.HTML})
		})
	}
}
