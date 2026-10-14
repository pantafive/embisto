// Package target reads the URL and language of a request as the pipeline does, so that a cache keys equal requests
// written differently alike.
package target

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/text/language"

	"github.com/pantafive/embisto/card"
)

var (
	errEmptyURL    = errors.New("the url is empty")
	errLongURL     = errors.New("the url is too long")
	errScheme      = errors.New("the scheme is not http or https")
	errNoHost      = errors.New("the url has no host")
	errCredentials = errors.New("the url carries credentials")
)

const maxURLLength = 2048

// hasScheme is a URL that starts with a scheme; one whose query holds a URL, such as example.com/?u=https://x, has
// none of its own.
var hasScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)

// Parse reads the URL to preview: http or https, a host and no credentials; a URL without a scheme is taken as
// https. The fragment never reaches the server, so it is dropped. It fails with invalid_url, in embisto's own words.
func Parse(raw string) (*url.URL, error) {
	target, err := parse(raw)
	if err != nil {
		refused := &card.Error{Code: card.CodeInvalidURL, Err: err}
		for _, known := range []error{errEmptyURL, errLongURL, errScheme, errNoHost, errCredentials} {
			if errors.Is(err, known) {
				// A parser's own words would quote the URL back.
				refused.Detail = known.Error()
			}
		}
		return nil, refused
	}
	return target, nil
}

func parse(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "":
		return nil, errEmptyURL
	case len(raw) > maxURLLength:
		return nil, fmt.Errorf("%w: over %d bytes", errLongURL, maxURLLength)
	case !hasScheme.MatchString(raw):
		raw = "https://" + raw
	}
	target, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing the url: %w", err)
	}
	target.Scheme = strings.ToLower(target.Scheme)
	switch {
	case target.Scheme != "http" && target.Scheme != "https":
		return nil, fmt.Errorf("%w: %q", errScheme, target.Scheme)
	case target.Hostname() == "":
		return nil, errNoHost
	case target.User != nil:
		return nil, errCredentials
	}
	target.Host = strings.ToLower(target.Host)
	target.Fragment, target.RawFragment = "", ""
	return target, nil
}

// Language is a language as the pipeline reads it, so that de and DE share a cache entry; empty for none or a
// malformed one.
func Language(raw string) string {
	if tag, err := language.Parse(raw); err == nil && raw != "" {
		return tag.String()
	}
	return ""
}
