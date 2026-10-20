package golden_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/render"
)

// hostile serves one page for every URL, and no picture.
type hostile struct {
	page []byte
}

func (h hostile) Fetch(ctx context.Context, target *url.URL, _ string) (card.Response, error) {
	if err := ctx.Err(); err != nil {
		return card.Response{}, err
	}
	return card.Response{URL: target, MediaType: "text/html", Body: h.page}, nil
}

func (hostile) Peek(context.Context, *url.URL, int64) (card.Response, error) {
	return card.Response{}, &card.Error{Code: card.CodeFetchFailed, Err: errors.New("no picture")}
}

// TestAPageCannotMakeTheCardAsLargeAsItLikes feeds a page of 120,000 pictures and a description of megabytes,
// under the body limit, which once took a minute of CPU and made a card of megabytes.
func TestAPageCannotMakeTheCardAsLargeAsItLikes(t *testing.T) {
	var page strings.Builder
	page.WriteString(`<title>Evil</title><meta property="og:description" content="` + strings.Repeat("words ", 200_000) + `">`)
	for i := range 120_000 {
		page.WriteString(`<meta property=og:image content=/` + strconv.Itoa(i) + `>`)
	}
	opts := embisto.Defaults()
	opts.Fetcher, opts.HomePages = hostile{page: []byte(page.String())}, nil

	started := time.Now()
	result, err := embisto.New(opts).Extract(context.Background(), card.Request{URL: "https://evil.example/item"})
	took := time.Since(started)

	require.NoError(t, err)
	raw, err := json.Marshal(result.Preview)
	require.NoError(t, err)
	assert.Less(t, took, 3*time.Second)
	assert.LessOrEqual(t, len(result.Preview.Images), 16)
	assert.LessOrEqual(t, len(raw), 64<<10)
}

// slow is a source that finishes after the preview's deadline and ignores it.
type slow struct{}

func (slow) Read(context.Context, card.Input) (card.Facts, error) {
	time.Sleep(300 * time.Millisecond)
	return card.Facts{Source: "slow", Title: "Too late"}, nil
}

func TestNoCardIsMadePastTheDeadline(t *testing.T) {
	opts := embisto.Defaults()
	opts.Fetcher, opts.HomePages, opts.Timeout = hostile{page: []byte(`<title>A page</title>`)}, nil, 100*time.Millisecond
	opts.Sources = append(opts.Sources, slow{})

	_, err := embisto.New(opts).Extract(context.Background(), card.Request{URL: "https://example.com/post"})

	assert.Equal(t, card.CodeTimeout, card.CodeOf(err))
}

// leaving serves a page, and gives up on the preview, as a client that disconnects, once its oEmbed is asked.
type leaving struct {
	cancel context.CancelFunc
}

func (l leaving) Fetch(ctx context.Context, target *url.URL, _ string) (card.Response, error) {
	if target.Path == "/oembed" {
		l.cancel()
		<-ctx.Done()
		return card.Response{}, ctx.Err()
	}
	page := `<title>A post</title><link rel="alternate" type="application/json+oembed" href="/oembed">`
	return card.Response{URL: target, MediaType: "text/html", Body: []byte(page)}, nil
}

func (leaving) Peek(ctx context.Context, _ *url.URL, _ int64) (card.Response, error) {
	return card.Response{}, ctx.Err()
}

// TestACallerThatGivesUpGetsItsCancellationNotATimeout: the caller's cancellation was answered as timeout.
func TestACallerThatGivesUpGetsItsCancellationNotATimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := embisto.Defaults()
	opts.Fetcher, opts.HomePages = leaving{cancel: cancel}, nil

	_, err := embisto.New(opts).Extract(ctx, card.Request{URL: "https://example.com/post"})

	require.ErrorIs(t, err, context.Canceled)
	_, isCoded := errors.AsType[*card.Error](err)
	assert.False(t, isCoded)
}

// local lets the defaults fetch from a test server.
var local = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}

// hanging serves a page whose oEmbed endpoint, home page and picture never answer, and nothing else either.
func hanging(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/post" {
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<meta property="og:title" content="A post"><meta property="og:image" content="/picture.png">`+
			`<link rel="alternate" type="application/json+oembed" href="/oembed">`)
	}))
	t.Cleanup(server.Close)
	return server
}

// TestStagesThatDoNotAnswerLeaveTheCardOfThePage: an oEmbed endpoint, a home page or a picture that hung took the
// preview to its deadline, and the card of a page already read failed with timeout.
func TestStagesThatDoNotAnswerLeaveTheCardOfThePage(t *testing.T) {
	server := hanging(t)
	opts := embisto.Defaults()
	opts.AllowedNetworks, opts.Timeout = local, time.Second

	started := time.Now()
	result, err := embisto.New(opts).Extract(context.Background(), card.Request{URL: server.URL + "/post"})

	require.NoError(t, err)
	assert.Equal(t, "A post", result.Preview.Title)
	assert.Less(t, time.Since(started), opts.Timeout)
}

func TestAPageThatDoesNotAnswerTimesOut(t *testing.T) {
	server := hanging(t)
	opts := embisto.Defaults()
	opts.AllowedNetworks, opts.Timeout = local, 200*time.Millisecond

	_, err := embisto.New(opts).Extract(context.Background(), card.Request{URL: server.URL + "/never"})

	assert.Equal(t, card.CodeTimeout, card.CodeOf(err))
}

// TestATextCardPrintsNoControlCharacter: a page's or an oEmbed answer's escapes reached format=text, made for
// terminals, as they came: a clipboard write, a cleared screen, a line that seemed to be the card's URL.
func TestATextCardPrintsNoControlCharacter(t *testing.T) {
	tests := []struct {
		name    string
		fetcher card.Fetcher
	}{
		{"from Open Graph", hostile{page: []byte(`<meta property="og:title" content="Hi&#x1b;]52;c;cHduZWQ=&#x07;&#x1b;[2J&#x202e;txt.exe"><meta property="og:description" content="A line&#x0d;forged&#x2066;">`)}},
		{"from oEmbed", replayOf(t, "control-characters")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := embisto.Defaults()
			opts.Fetcher, opts.HomePages = tt.fetcher, nil

			result, err := embisto.New(opts).Extract(context.Background(), card.Request{URL: "https://example.com/post"})
			require.NoError(t, err)
			var text strings.Builder
			require.NoError(t, render.Text{}.Render(&text, result.Preview))

			lines := strings.Split(strings.TrimSuffix(text.String(), "\n"), "\n")
			assert.Len(t, lines, 3)
			assert.Equal(t, "https://example.com/post", lines[len(lines)-1])
			assert.Equal(t, -1, strings.IndexFunc(text.String(), func(r rune) bool {
				return r != '\n' && unicode.IsControl(r) || r == '\u202e' || r == '\u2066'
			}), text.String())
		})
	}
}
