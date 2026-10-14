package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/fixture"
)

const maxNameLength = 60

var (
	errPinUsage   = errors.New("usage: embisto pin [-dir testdata/fixtures] [-name name] <url>")
	errDiverged   = errors.New("the saved pages make a different preview than the live ones; the fixture is left for a look")
	notInName     = regexp.MustCompile(`[^a-z0-9]+`)
	defaultPinDir = filepath.Join("testdata", "fixtures")
)

// runPin previews a URL live, saves every response it fetched and the preview as a fixture the golden test replays,
// and checks the saved responses make the same preview.
func runPin(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("pin", flag.ContinueOnError)
	flags.SetOutput(out)
	dir := flags.String("dir", defaultPinDir, "where fixtures live")
	name := flags.String("name", "", "the fixture's name; by default made of the URL")
	retrim := flags.Bool("retrim", false, "trim again every page the fixtures in -dir saved, and pin nothing")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w: %w", errPinUsage, err)
	}
	if *retrim {
		rewritten, err := fixture.Retrim(*dir)
		if err != nil {
			return err //nolint:wrapcheck // it names the page
		}
		_, err = fmt.Fprintf(out, "%d pages trimmed again\n", rewritten)
		return err //nolint:wrapcheck // a failed print to the terminal
	}
	if flags.NArg() != 1 {
		return errPinUsage
	}
	req := card.Request{URL: flags.Arg(0)}
	opts := embisto.Defaults()
	recorder := fixture.NewRecorder(embisto.NewFetcher(opts))
	opts.Fetcher = recorder
	live, err := embisto.New(opts).Extract(ctx, req)
	failure, err := pinnedFailure(err)
	if err != nil {
		return fmt.Errorf("previewing %s: %w", req.URL, err)
	}
	target := filepath.Join(*dir, cmpName(*name, req.URL))
	if err := recorder.Save(target, req, failure); err != nil {
		return err //nolint:wrapcheck // it says what was not saved
	}
	if err := replayed(ctx, target, req, live.Preview, failure); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, target)
	return err //nolint:wrapcheck // a failed print to the terminal
}

// replayed checks the saved responses make what the live ones made, the same failure or the same preview, and
// saves the preview as the golden one.
func replayed(ctx context.Context, target string, req card.Request, live card.Preview, failure card.Code) error {
	_, replay, err := fixture.Load(target)
	if err != nil {
		return err //nolint:wrapcheck // it names the fixture
	}
	opts := embisto.Defaults()
	opts.Fetcher = replay
	replayedResult, err := embisto.New(opts).Extract(ctx, req)
	preview := replayedResult.Preview
	switch {
	case failure != "" && card.CodeOf(err) == failure:
		return nil
	case err != nil:
		return fmt.Errorf("replaying %s: %w", target, err)
	case !reflect.DeepEqual(live, preview):
		return fmt.Errorf("%w: %s", errDiverged, target)
	default:
		return fixture.WritePreview(target, preview) //nolint:wrapcheck // it says what was not saved
	}
}

// pinnedFailure is the failure a fixture keeps: an item that is gone is pinned as such; any other failure is no
// fixture.
func pinnedFailure(err error) (card.Code, error) {
	switch {
	case err == nil:
		return "", nil
	case card.CodeOf(err) == card.CodeNotFound:
		return card.CodeNotFound, nil
	default:
		return "", err
	}
}

// cmpName is the given name, or one made of the URL's host and path.
func cmpName(given, raw string) string {
	if given != "" {
		return given
	}
	name := raw
	if parsed, err := url.Parse(raw); err == nil {
		name = parsed.Host + parsed.Path + "-" + parsed.RawQuery
	}
	name = strings.Trim(notInName.ReplaceAllString(strings.ToLower(name), "-"), "-")
	return strings.TrimRight(name[:min(len(name), maxNameLength)], "-")
}
