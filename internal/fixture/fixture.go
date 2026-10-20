// Package fixture records what a preview fetched and replays it without the network, so a page saved once stays a
// test of the whole pipeline.
package fixture

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/imagesize"
	"github.com/pantafive/embisto/render"
)

// Files of a fixture's directory besides the saved bodies.
const (
	FixtureFile = "fixture.json"
	PreviewFile = "preview.json"
)

const (
	methodFetch = "fetch"
	methodPeek  = "peek"
)

// ErrNotRecorded is a request the fixture holds no answer to.
var ErrNotRecorded = errors.New("not recorded")

// Fixture is the request a preview was made for and every answer the network gave it.
type Fixture struct {
	Request card.Request `json:"request"`
	// Failure is the code the preview fails with, for an item that is gone; empty for a preview that is made.
	Failure   card.Code  `json:"failure,omitempty"`
	Exchanges []Exchange `json:"exchanges"`
}

// Exchange is one request and its answer: a response, with its body in a file next to the fixture, or a failure.
// A peek keeps no body, only the size its picture measures, so no fixture holds a picture.
type Exchange struct {
	Method    string    `json:"method"`
	URL       string    `json:"url"`
	FinalURL  string    `json:"final_url,omitempty"`
	MediaType string    `json:"media_type,omitempty"`
	File      string    `json:"file,omitempty"`
	Width     int       `json:"width,omitempty"`
	Height    int       `json:"height,omitempty"`
	Code      card.Code `json:"code,omitempty"`
	Status    int       `json:"status,omitempty"`
}

// mediaHTML is a page's media type, the one Trim trims.
const mediaHTML = "text/html"

func (e Exchange) key() string {
	return e.Method + " " + e.URL
}

// Replay answers from a fixture's directory instead of the network.
type Replay struct {
	dir       string
	exchanges map[string]Exchange
}

// Load reads the fixture in a directory.
func Load(dir string) (Fixture, *Replay, error) {
	raw, err := os.ReadFile(filepath.Join(dir, FixtureFile)) //nolint:gosec // the directory is the caller's own fixture
	if err != nil {
		return Fixture{}, nil, fmt.Errorf("reading the fixture: %w", err)
	}
	var fixture Fixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		return Fixture{}, nil, fmt.Errorf("decoding %s: %w", filepath.Join(dir, FixtureFile), err)
	}
	replay := &Replay{dir: dir, exchanges: map[string]Exchange{}}
	for _, exchange := range fixture.Exchanges {
		replay.exchanges[exchange.key()] = exchange
	}
	return fixture, replay, nil
}

func (r *Replay) Fetch(_ context.Context, target *url.URL, _ string) (card.Response, error) {
	return r.answer(Exchange{Method: methodFetch, URL: target.String()})
}

func (r *Replay) Peek(_ context.Context, target *url.URL, _ int64) (card.Response, error) {
	return r.answer(Exchange{Method: methodPeek, URL: target.String()})
}

func (r *Replay) answer(asked Exchange) (card.Response, error) {
	exchange, ok := r.exchanges[asked.key()]
	if !ok {
		return card.Response{}, &card.Error{Code: card.CodeFetchFailed, Err: fmt.Errorf("%w: %s", ErrNotRecorded, asked.key())}
	}
	if exchange.Code != "" {
		return card.Response{}, &card.Error{Code: exchange.Code, Status: exchange.Status, Err: fmt.Errorf("replayed: %s", exchange.Code)} //nolint:err113 // the recorded failure
	}
	final, err := url.Parse(exchange.FinalURL)
	if err != nil || exchange.FinalURL == "" {
		final, _ = url.Parse(exchange.URL)
	}
	resp := card.Response{URL: final, MediaType: exchange.MediaType}
	if exchange.Width > 0 {
		resp.Body = pngHeader(exchange.Width, exchange.Height)
	}
	if exchange.File != "" {
		if resp.Body, err = os.ReadFile(filepath.Join(r.dir, exchange.File)); err != nil {
			return card.Response{}, &card.Error{Code: card.CodeFetchFailed, Err: err}
		}
	}
	return resp, nil
}

// Recorder fetches through another fetcher and keeps every answer.
type Recorder struct {
	next card.Fetcher

	mu       sync.Mutex
	recorded []recorded
}

type recorded struct {
	exchange Exchange
	body     []byte
}

// NewRecorder records what the fetcher answers.
func NewRecorder(next card.Fetcher) *Recorder {
	return &Recorder{next: next}
}

func (r *Recorder) Fetch(ctx context.Context, target *url.URL, accept string) (card.Response, error) {
	resp, err := r.next.Fetch(ctx, target, accept)
	r.keep(Exchange{Method: methodFetch, URL: target.String()}, resp, err)
	return resp, err //nolint:wrapcheck // the recorded fetcher's own answer
}

func (r *Recorder) Peek(ctx context.Context, target *url.URL, limit int64) (card.Response, error) {
	resp, err := r.next.Peek(ctx, target, limit)
	r.keep(Exchange{Method: methodPeek, URL: target.String()}, resp, err)
	return resp, err //nolint:wrapcheck // the recorded fetcher's own answer
}

// Save writes the request and what was recorded for it into a directory; saved HTML keeps only what the pipeline
// reads, and a peek only the size of its picture.
func (r *Recorder) Save(dir string, req card.Request, failure card.Code) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("making the fixture's directory: %w", err)
	}
	fixture := Fixture{Request: req, Failure: failure}
	for i, rec := range r.recorded {
		exchange := rec.exchange
		if exchange.Method == methodPeek {
			// An unmeasured picture keeps no size, and replays as an empty body, which measures to nothing either.
			if width, height, ok := imagesize.Size(rec.body); ok {
				exchange.Width, exchange.Height = width, height
			}
		} else if len(rec.body) > 0 {
			body := rec.body
			if exchange.MediaType == mediaHTML {
				body = Trim(body)
			}
			exchange.File = strconv.Itoa(i+1) + extension(exchange.MediaType)
			if err := os.WriteFile(filepath.Join(dir, exchange.File), body, 0o600); err != nil {
				return fmt.Errorf("saving a body: %w", err)
			}
		}
		fixture.Exchanges = append(fixture.Exchanges, exchange)
	}
	raw, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the fixture: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, FixtureFile), append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("saving the fixture: %w", err)
	}
	return nil
}

func (r *Recorder) keep(exchange Exchange, resp card.Response, err error) {
	if err != nil {
		var failed *card.Error
		if !errors.As(err, &failed) {
			return
		}
		exchange.Code, exchange.Status = failed.Code, failed.Status
	} else {
		exchange.MediaType = resp.MediaType
		if final := resp.URL.String(); final != exchange.URL {
			exchange.FinalURL = final
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recorded = append(r.recorded, recorded{exchange: exchange, body: resp.Body})
}

// extension names a fetched body's file; the fetcher keeps the body of text alone.
func extension(mediaType string) string {
	switch mediaType {
	case "text/html":
		return ".html"
	case "application/json", "application/json+oembed":
		return ".json"
	default:
		return ".txt"
	}
}

// pngHeader is the start of a PNG file of a size, which is all that measuring a picture reads.
func pngHeader(width, height int) []byte {
	const (
		depth = 8
		rgba  = 6
		// ihdrLength is the IHDR chunk's data: width, height and five one-byte fields.
		ihdrLength = 13
	)
	chunk := binary.BigEndian.AppendUint32([]byte("IHDR"), uint32(width)) //nolint:gosec // a size the fixture states
	chunk = binary.BigEndian.AppendUint32(chunk, uint32(height))          //nolint:gosec // a size the fixture states
	chunk = append(chunk, depth, rgba, 0, 0, 0)
	head := binary.BigEndian.AppendUint32([]byte("\x89PNG\r\n\x1a\n"), ihdrLength)
	head = append(head, chunk...)
	return binary.BigEndian.AppendUint32(head, crc32.ChecksumIEEE(chunk))
}

// WritePreview saves the golden answers of a fixture in every format, indented for review.
func WritePreview(dir string, preview card.Preview) error {
	for _, format := range Formats {
		var rendered, indented bytes.Buffer
		if err := format.Render(&rendered, preview); err != nil {
			return fmt.Errorf("rendering the preview as %s: %w", format.Name(), err)
		}
		if !IsJSON(format) {
			indented = rendered
		} else if err := json.Indent(&indented, rendered.Bytes(), "", "  "); err != nil {
			return fmt.Errorf("indenting the preview: %w", err)
		}
		if err := os.WriteFile(filepath.Join(dir, GoldenFile(format)), indented.Bytes(), 0o600); err != nil {
			return fmt.Errorf("saving the preview: %w", err)
		}
	}
	return nil
}

// Formats are the renderers a fixture keeps a golden answer of.
var Formats = render.Defaults()

// GoldenFile is the file of a format's golden answer.
func GoldenFile(format card.Renderer) string {
	switch {
	case format.Name() == (render.JSON{}).Name():
		return PreviewFile
	case IsJSON(format):
		return format.Name() + ".json"
	default:
		return "preview." + format.Name()
	}
}

// IsJSON says whether a format writes JSON, which a golden file keeps indented and a test compares as JSON.
func IsJSON(format card.Renderer) bool {
	return strings.HasPrefix(format.ContentType(), "application/json")
}

// Retrim trims again every page the fixtures under root saved, for when Trim learns to drop more. It says how many
// pages it rewrote.
func Retrim(root string) (int, error) {
	dirs, err := filepath.Glob(filepath.Join(root, "*", FixtureFile))
	if err != nil {
		return 0, fmt.Errorf("finding the fixtures: %w", err)
	}
	rewritten := 0
	for _, file := range dirs {
		saved, _, err := Load(filepath.Dir(file))
		if err != nil {
			return rewritten, err
		}
		for _, exchange := range saved.Exchanges {
			if exchange.MediaType != mediaHTML || exchange.File == "" {
				continue
			}
			page := filepath.Join(filepath.Dir(file), exchange.File)
			raw, err := os.ReadFile(page) //nolint:gosec // the caller's own fixtures
			if err != nil {
				return rewritten, fmt.Errorf("reading %s: %w", page, err)
			}
			if err := os.WriteFile(page, Trim(raw), 0o600); err != nil { //nolint:gosec // a page the caller's own fixture names
				return rewritten, fmt.Errorf("rewriting %s: %w", page, err)
			}
			rewritten++
		}
	}
	return rewritten, nil
}
