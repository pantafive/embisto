// Package fetch is the only code that goes to the network: an HTTP client that refuses addresses which are not
// public, checked on every connection it opens, so neither a redirect nor a DNS answer that changes between lookups
// reaches the host's own network.
package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html/charset"

	"github.com/pantafive/embisto/card"
)

var (
	// ErrBlocked is a URL whose address is not public, or a redirect to a scheme other than http and https.
	ErrBlocked = errors.New("blocked address")
	// ErrTooLarge is a body over the size limit.
	ErrTooLarge = errors.New("body too large")
	// ErrTimeout is a request that did not finish in time.
	ErrTimeout = errors.New("timed out")
	// ErrFailed is a request that reached no answer worth reading: a refused connection, a bad status, a redirect
	// loop.
	ErrFailed = errors.New("fetch failed")
)

// charsetSniff is how many leading bytes the HTML standard looks through for a <meta charset>.
const charsetSniff = 1024

// Options shape the client; every field is required except Allowed.
type Options struct {
	Timeout      time.Duration
	MaxBody      int64
	MaxRedirects int
	UserAgent    string
	// Allowed are networks reachable although not public, for a deployment that previews its own intranet.
	Allowed []netip.Prefix
}

// Client gets pages over HTTP for a preview.
type Client struct {
	http      *http.Client
	maxBody   int64
	userAgent string
}

// New builds a client.
func New(opts Options) *Client {
	return &Client{
		http: &http.Client{
			Transport: guarded(opts.Allowed, opts.Timeout),
			Timeout:   opts.Timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > opts.MaxRedirects {
					return fmt.Errorf("%w: more than %d redirects", ErrFailed, opts.MaxRedirects)
				}
				if !isWeb(req.URL) {
					return fmt.Errorf("%w: redirect to %s", ErrBlocked, req.URL.Scheme)
				}
				return nil
			},
		},
		maxBody:   opts.MaxBody,
		userAgent: opts.UserAgent,
	}
}

func guarded(allowed []netip.Prefix, timeout time.Duration) *http.Transport {
	dialer := &net.Dialer{Timeout: timeout, Control: func(_, address string, _ syscall.RawConn) error {
		// The address is already resolved here, so a name that resolves to a private address is caught too.
		addrPort, err := netip.ParseAddrPort(address)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrBlocked, address)
		}
		addr := addrPort.Addr().Unmap()
		if !IsPublic(addr) && !slices.ContainsFunc(allowed, func(p netip.Prefix) bool { return p.Contains(addr) }) {
			return fmt.Errorf("%w: %s", ErrBlocked, addr)
		}
		return nil
	}}
	return &http.Transport{
		// No proxy from the environment: the guard would check the proxy's address instead of the target's.
		Proxy:               nil,
		DialContext:         dialer.DialContext,
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: timeout,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
	}
}

func isWeb(u *url.URL) bool {
	return u.Scheme == "http" || u.Scheme == "https"
}

// Fetch gets a URL, asking for the media types in accept; it fails with a *card.Error wrapping one of the package's
// errors, or with the context's own error when the caller gave up.
func (c *Client) Fetch(ctx context.Context, target *url.URL, accept string) (card.Response, error) {
	resp, err := c.get(ctx, target, http.Header{"Accept": {accept}}, c.readText)
	return resp, coded(ctx, err)
}

// Peek gets at most the first limit bytes of a URL of any media type. It asks for that range, and stops reading there
// when the server sends the whole body anyway.
func (c *Client) Peek(ctx context.Context, target *url.URL, limit int64) (card.Response, error) {
	read := func(ctx context.Context, body io.Reader, _ string, resp *card.Response) error {
		var err error
		if resp.Body, err = io.ReadAll(io.LimitReader(body, limit)); err != nil {
			return classify(ctx, err)
		}
		return nil
	}
	resp, err := c.get(ctx, target, http.Header{"Range": {fmt.Sprintf("bytes=0-%d", limit-1)}}, read)
	return resp, coded(ctx, err)
}

// coded gives an error its card code; the caller's own cancellation stays as it is.
func coded(ctx context.Context, err error) error {
	if err == nil || ctx.Err() != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return err
	}
	failed := &card.Error{Code: codeOf(err), Err: err}
	if status, ok := errors.AsType[StatusError](err); ok {
		failed.Status = status.Status
	}
	return failed
}

// StatusError is a page that answered with an error status.
type StatusError struct {
	Status int
}

func (e StatusError) Error() string {
	return fmt.Sprintf("%s: status %d", ErrFailed, e.Status)
}

func (StatusError) Unwrap() error {
	return ErrFailed
}

func codeOf(err error) card.Code {
	switch {
	case errors.Is(err, ErrBlocked):
		return card.CodeBlockedURL
	case errors.Is(err, ErrTimeout):
		return card.CodeTimeout
	case errors.Is(err, ErrTooLarge):
		return card.CodeTooLarge
	default:
		return card.CodeFetchFailed
	}
}

// reader takes the body of a successful response, given its Content-Type.
type reader func(ctx context.Context, body io.Reader, contentType string, resp *card.Response) error

func (c *Client) get(ctx context.Context, target *url.URL, header http.Header, read reader) (card.Response, error) {
	if !isWeb(target) {
		return card.Response{}, fmt.Errorf("%w: scheme %q", ErrBlocked, target.Scheme)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return card.Response{}, fmt.Errorf("%w: %w", ErrFailed, err)
	}
	req.Header = header
	req.Header.Set("User-Agent", c.userAgent)
	if language := card.LanguageOf(ctx); language != "" {
		req.Header.Set("Accept-Language", language)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return card.Response{}, classify(ctx, err)
	}
	defer resp.Body.Close() //nolint:errcheck // a read-only body; its close error says nothing
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return card.Response{}, StatusError{Status: resp.StatusCode}
	}
	contentType := resp.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = "application/octet-stream"
	}
	got := card.Response{URL: resp.Request.URL, MediaType: mediaType, Freshness: freshnessOf(resp.Header)}
	if err := read(ctx, resp.Body, contentType, &got); err != nil {
		return card.Response{}, err
	}
	return got, nil
}

// readText reads a text body within the size limit, decoded to UTF-8; other media come without one.
func (c *Client) readText(ctx context.Context, body io.Reader, contentType string, resp *card.Response) error {
	if !isText(resp.MediaType) {
		return nil
	}
	var err error
	resp.Body, err = c.read(ctx, body, resp.MediaType, contentType)
	return err
}

func (c *Client) read(ctx context.Context, body io.Reader, mediaType, contentType string) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, c.maxBody+1))
	if err != nil {
		return nil, classify(ctx, err)
	}
	if int64(len(raw)) > c.maxBody {
		return nil, fmt.Errorf("%w: over %d bytes", ErrTooLarge, c.maxBody)
	}
	decoded, err := decode(raw, mediaType, contentType)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailed, err)
	}
	return decoded, nil
}

func isText(mediaType string) bool {
	return strings.HasPrefix(mediaType, "text/") || strings.HasSuffix(mediaType, "json") ||
		strings.HasSuffix(mediaType, "xml")
}

// lateMeta finds a <meta> that names a charset, for one past the first 1024 bytes the HTML standard looks through.
var lateMeta = regexp.MustCompile(`(?i)<meta[^>]+charset\s*=\s*["']?\s*([a-z0-9_.:-]+)`)

// lateMetaWithin is how far into a body a late <meta charset> is looked for.
const lateMetaWithin = 64 << 10

// decode turns a body into UTF-8. A byte-order mark or the header's charset decides; else JSON is UTF-8, as its
// standard says; else a body that is valid UTF-8 throughout is taken as UTF-8, though its first kilobyte is ASCII
// alone; else a <meta charset>, early or late; else windows-1252, as browsers default to.
func decode(raw []byte, mediaType, contentType string) ([]byte, error) {
	encoding, name, isCertain := charset.DetermineEncoding(raw[:min(len(raw), charsetSniff)], contentType)
	if !isCertain {
		switch {
		case strings.HasSuffix(mediaType, "json"), utf8.Valid(raw):
			return raw, nil
		case lateMeta.Match(raw[:min(len(raw), lateMetaWithin)]):
			if late, lateName := charset.Lookup(string(lateMeta.FindSubmatch(raw[:min(len(raw), lateMetaWithin)])[1])); late != nil {
				encoding, name = late, lateName
			}
		}
	}
	if name == "utf-8" {
		return raw, nil
	}
	decoded, err := io.ReadAll(encoding.NewDecoder().Reader(bytes.NewReader(raw)))
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", name, err)
	}
	return decoded, nil
}

// classify translates a transport error into the package's own; a caller that gave up gets its context's error.
func classify(ctx context.Context, err error) error {
	if ctx.Err() != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ctx.Err() //nolint:wrapcheck // the caller's own cancellation
	}
	var netErr net.Error
	switch {
	case errors.Is(err, ErrBlocked), errors.Is(err, ErrFailed):
		return err
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return fmt.Errorf("%w: %w", ErrTimeout, err)
	default:
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
}
