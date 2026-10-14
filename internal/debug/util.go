package debug

import (
	"errors"
	"net/url"
)

var errNoImage = errors.New("the card has no image")

func urlOf(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, errNoImage
	}
	return url.Parse(raw) //nolint:wrapcheck // a URL the card already holds
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
