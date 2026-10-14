// Package golden_test replays the pinned fixtures and the example templates through the whole pipeline.
package golden_test

import (
	"bytes"
	"context"
	"flag"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/fixture"
	"github.com/pantafive/embisto/internal/quality"
)

var update = flag.Bool("update", false, "rewrite the golden previews from what the pipeline makes now")

// fixtures are where `embisto pin` saves them, at the module's root.
var fixtures = filepath.Join("..", "..", "testdata", "fixtures")

// replayed is the default pipeline answered by the fixture's responses.
func replayed(replay card.Fetcher) *embisto.Core {
	opts := embisto.Defaults()
	opts.Fetcher = replay
	return embisto.New(opts)
}

// TestExtractMatchesTheGoldenPreview replays every fixture in testdata/fixtures, saved by hand or by `embisto pin`,
// and compares the preview with the one saved next to it.
func TestExtractMatchesTheGoldenPreview(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join(fixtures, "*"))
	require.NoError(t, err)
	require.NotEmpty(t, dirs)
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			saved, replay, err := fixture.Load(dir)
			require.NoError(t, err)
			result, err := replayed(replay).Extract(context.Background(), saved.Request)
			preview := result.Preview

			if saved.Failure != "" {
				assert.Equal(t, saved.Failure, card.CodeOf(err))
				return
			}
			require.NoError(t, err)
			if *update {
				require.NoError(t, fixture.WritePreview(dir, preview))
			}
			for _, format := range fixture.Formats {
				var rendered bytes.Buffer
				require.NoError(t, format.Render(&rendered, preview))
				want, err := os.ReadFile(filepath.Join(dir, fixture.GoldenFile(format)))
				require.NoError(t, err)
				if fixture.IsJSON(format) {
					assert.JSONEq(t, string(want), rendered.String(), format.Name())
				} else {
					assert.Equal(t, string(want), rendered.String(), format.Name())
				}
			}
		})
	}
}

func replayOf(t *testing.T, name string) card.Fetcher {
	t.Helper()
	_, replay, err := fixture.Load(filepath.Join(fixtures, name))
	require.NoError(t, err)
	return replay
}

func TestExtractRefusesAURLThatIsNotHTTP(t *testing.T) {
	_, err := replayed(replayOf(t, "bare")).Extract(context.Background(), card.Request{URL: "file:///etc/passwd"})

	assert.Equal(t, card.CodeInvalidURL, card.CodeOf(err))
}

func TestExtractPassesOnTheFetchersFailure(t *testing.T) {
	_, err := replayed(replayOf(t, "bare")).Extract(context.Background(), card.Request{URL: "https://example.com/gone"})

	require.Error(t, err)
	assert.Equal(t, card.CodeFetchFailed, card.CodeOf(err))
}

// recorder keeps what the pipeline told it; the enricher and concurrent callers report from several goroutines.
type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) Fetched(stage, outcome string, _ time.Duration) {
	r.add("fetched " + stage + " " + outcome)
}
func (r *recorder) Matched(provider string)   { r.add("matched " + provider) }
func (r *recorder) Made(class, source string) { r.add("made " + class + " " + source) }

func (r *recorder) add(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func TestExtractTellsTheObserverEveryStageThatFetched(t *testing.T) {
	observer := &recorder{}
	opts := embisto.Defaults()
	opts.Fetcher, opts.Observer = replayOf(t, "listed-provider"), observer

	_, err := embisto.New(opts).Extract(context.Background(), card.Request{URL: "https://vimeo.com/76979871"})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"fetched page ok", "matched oembed", "fetched oembed ok", "fetched imagesize fetch_failed", "made partial oembed",
	}, observer.events)
}

// A YouTube page served to a datacenter without the video's metadata carries YouTube's site-wide description; the
// card leaves the description empty rather than wrong, and is partial for it. The fixture is reconstructed from such a
// card.
func TestExtractDropsASiteWideDescription(t *testing.T) {
	saved, replay, err := fixture.Load(filepath.Join(fixtures, "youtube-generic-description"))
	require.NoError(t, err)

	result, err := replayed(replay).Extract(context.Background(), saved.Request)

	require.NoError(t, err)
	assert.Equal(t, []any{"", quality.ClassPartial}, []any{result.Preview.Description, quality.Classify(result.Preview, nil)})
}

// mine is a library user's own provider for a site the oEmbed list also covers.
type mine struct{}

func (mine) Name() string { return "mine" }

func (mine) Match(u *url.URL) card.Match {
	if u.Hostname() == "vimeo.com" {
		return card.MatchedAlone
	}
	return card.Unmatched
}

func (mine) Provide(context.Context, card.Input) (card.Facts, error) {
	return card.Facts{Title: "Told by mine"}, nil
}

func TestAProviderAddedWinsOverTheFallbacks(t *testing.T) {
	opts := embisto.Defaults()
	opts.Fetcher, opts.Providers = replayOf(t, "listed-provider"), append(opts.Providers, mine{})

	result, err := embisto.New(opts).Extract(context.Background(), card.Request{URL: "https://vimeo.com/76979871"})

	require.NoError(t, err)
	assert.Equal(t, []string{"mine", "Told by mine"}, []string{result.Provider, result.Preview.Title})
}
