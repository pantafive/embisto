package sites

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/oembed"
)

const youtubeOEmbed = "https://www.youtube.com/oembed"

var (
	youtubeID    = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	youtubeList  = regexp.MustCompile(`^[A-Za-z0-9_-]{2,64}$`)
	youtubeStart = regexp.MustCompile(`^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s?)?$`)
	youtubeHosts = []string{"youtube.com", "m.youtube.com", "music.youtube.com", "youtu.be", "youtube-nocookie.com"}
)

// YouTube knows every address of a YouTube video, short or playlist: youtu.be, m., music., shorts, live, embed and
// the nocookie host, with the start a shared link carries in t=. It reads the page when YouTube serves one, and
// stands in from YouTube's oEmbed when the page is a consent wall or fails. A video YouTube says does not exist is
// not_found, not a card of YouTube's error page.
type YouTube struct {
	// NoCookie embeds the player from youtube-nocookie.com, which sets no cookies until the video plays.
	NoCookie bool
}

// youtubeItem is what a YouTube address points at.
type youtubeItem struct {
	video   string
	list    string
	isShort bool
	start   int
}

func (YouTube) Name() string {
	return "youtube"
}

func (YouTube) Match(u *url.URL) card.Match {
	if _, ok := parseYouTube(u); ok {
		return card.MatchedWithPage
	}
	return card.Unmatched
}

func (y YouTube) Provide(ctx context.Context, in card.Input) (card.Facts, error) {
	item, ok := parseYouTube(in.URL)
	if !ok {
		return card.Facts{}, nil
	}
	facts, isEmbeddable, err := y.oEmbed(ctx, in, item)
	if err != nil {
		return card.Facts{}, err
	}
	facts.Type, facts.SiteName, facts.CanonicalURL = card.TypeVideo, "YouTube", item.canonical()
	facts.Boilerplate.Texts = youtubeBoilerplate
	if hasPictures(in.Facts) {
		// The page's own pictures are larger than oEmbed's 4:3 thumbnail with its black bars.
		facts.Images = nil
	}
	if !isEmbeddable {
		// A private video, or one whose embedding is off, plays nowhere but on YouTube: no player.
		return facts, nil
	}
	player := y.player(item)
	width, height := item.size()
	facts.Videos = []card.Video{{URL: player, Type: "text/html", Width: width, Height: height}}
	if facts.Embed.HTML != "" {
		facts.Embed = card.Embed{
			HTML: fmt.Sprintf(`<iframe width="%d" height="%d" src="%s" title="%s" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" referrerpolicy="strict-origin-when-cross-origin" allowfullscreen></iframe>`,
				width, height, html.EscapeString(player), html.EscapeString(facts.Title)),
			Width: width, Height: height, Provider: "YouTube",
		}
	}
	return facts, nil
}

// oEmbed is what YouTube's oEmbed says of the item: what the page's discovery already fetched, or a call of its own.
// An item YouTube does not know is not_found; one it will not embed is not embeddable, and keeps only what the page
// says.
func (y YouTube) oEmbed(ctx context.Context, in card.Input, item youtubeItem) (card.Facts, bool, error) {
	for _, facts := range in.Facts {
		if facts.Source == card.SourceOEmbed && !facts.IsEmpty() {
			return facts, true, nil
		}
	}
	endpoint, err := oembed.Endpoint(youtubeOEmbed, mustURL(item.canonical()), in.Options)
	if err != nil {
		return card.Facts{}, false, err //nolint:wrapcheck // it names the endpoint
	}
	facts, err := oembed.Get(ctx, in.Fetcher, endpoint, youtubeOEmbed)
	if err != nil {
		failure := youtubeFailure(err)
		return card.Facts{}, false, failure
	}
	return facts, true, nil
}

// hasPictures says whether a generic source found a picture of the page, other than oEmbed's thumbnail.
func hasPictures(generic []card.Facts) bool {
	return slices.ContainsFunc(generic, func(facts card.Facts) bool {
		return facts.Source != card.SourceOEmbed && len(facts.Images) > 0
	})
}

// youtubeFailure reads what YouTube's oEmbed failure says: an unknown or impossible video is not_found; a private
// one, or one whose embedding is off, leaves the card what the page says, without a player.
func youtubeFailure(err error) error {
	var failed *card.Error
	if !errors.As(err, &failed) {
		return err
	}
	switch failed.Status {
	case http.StatusNotFound, http.StatusBadRequest:
		return &card.Error{Code: card.CodeNotFound, Status: http.StatusNotFound, Err: err}
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil
	default:
		return err
	}
}

func (y YouTube) player(item youtubeItem) string {
	host := "www.youtube.com"
	if y.NoCookie {
		host = "www.youtube-nocookie.com"
	}
	query := url.Values{}
	path := "/embed/" + item.video
	if item.video == "" {
		path = "/embed/videoseries"
		query.Set("list", item.list)
	}
	if item.start > 0 {
		query.Set("start", strconv.Itoa(item.start))
	}
	player := url.URL{Scheme: schemeHTTPS, Host: host, Path: path, RawQuery: query.Encode()}
	return player.String()
}

// size is the player's: upright for a short, widescreen otherwise.
func (item youtubeItem) size() (int, int) {
	if item.isShort {
		return 315, 560
	}
	return 560, 315
}

func (item youtubeItem) canonical() string {
	switch {
	case item.video == "":
		return "https://www.youtube.com/playlist?list=" + url.QueryEscape(item.list)
	case item.isShort:
		return "https://www.youtube.com/shorts/" + item.video
	default:
		return "https://www.youtube.com/watch?v=" + item.video
	}
}

// parseYouTube reads a YouTube address; false for one that names no video or playlist.
func parseYouTube(u *url.URL) (youtubeItem, bool) {
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if !slices.Contains(youtubeHosts, host) {
		return youtubeItem{}, false
	}
	query := u.Query()
	item := youtubeItem{list: query.Get("list"), start: youtubeSeconds(cmp.Or(query.Get("t"), query.Get("start")))}
	var ok bool
	if item.video, item.isShort, ok = youtubeVideo(host, strings.Split(strings.Trim(u.Path, "/"), "/"), query); !ok {
		return youtubeItem{}, false
	}
	if item.video != "" {
		return item, youtubeID.MatchString(item.video)
	}
	return item, youtubeList.MatchString(item.list)
}

// youtubeVideo is the video an address's path names, empty for a playlist; false for a path that names neither.
func youtubeVideo(host string, segments []string, query url.Values) (string, bool, bool) {
	switch {
	case host == "youtu.be":
		return segments[0], false, true
	case segments[0] == "watch":
		return query.Get("v"), false, true
	case segments[0] == "playlist", len(segments) == 2 && segments[1] == "videoseries":
		return "", false, true
	case len(segments) == 2 && slices.Contains([]string{"shorts", "live", "embed", "v", "e"}, segments[0]):
		return segments[1], segments[0] == "shorts", true
	default:
		return "", false, false
	}
}

// youtubeSeconds reads a start as YouTube writes it: 42, 42s, 1m10s or 1h2m3s; zero for anything else.
func youtubeSeconds(raw string) int {
	match := youtubeStart.FindStringSubmatch(raw)
	if raw == "" || match == nil {
		return 0
	}
	seconds := 0
	for i, unit := range []int{3600, 60, 1} {
		if n, err := strconv.Atoi(match[i+1]); err == nil {
			seconds += n * unit
		}
	}
	return seconds
}

func mustURL(raw string) *url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err) // built from parsed parts
	}
	return parsed
}
