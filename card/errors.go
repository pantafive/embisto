package card

import (
	"errors"
	"log/slog"
	"net/http"
	"slices"
)

// Code tells failures apart for a client.
type Code string

// The codes embisto fails with, the preview's and the server's.
const (
	// CodeInvalidRequest is a request whose parameters are malformed or out of range.
	CodeInvalidRequest Code = "invalid_request"
	CodeInvalidURL     Code = "invalid_url"
	CodeBlockedURL     Code = "blocked_url"
	CodeFetchFailed    Code = "fetch_failed"
	CodeTimeout        Code = "timeout"
	CodeTooLarge       Code = "too_large"
	// CodeNotFound is an item that does not exist, or no longer does: a page answering 404 or 410, or a provider
	// saying the video or post was removed. Error.Status keeps which.
	CodeNotFound Code = "not_found"
	// CodeUnknownFormat is an output format the server does not have.
	CodeUnknownFormat Code = "unknown_format"
	// CodeNotCached is a purge that found nothing to drop.
	CodeNotCached Code = "not_cached"
	// CodeCacheUnavailable is a purge the shared cache did not answer.
	CodeCacheUnavailable Code = "cache_unavailable"
	// CodeInvalidSignature and the codes after it are the image proxy's own refusals.
	CodeInvalidSignature Code = "invalid_signature"
	CodeInvalidOptions   Code = "invalid_options"
	CodeTooManyPixels    Code = "too_many_pixels"
	CodeUnsupportedImage Code = "unsupported_image"
	CodeImageTooLarge    Code = "image_too_large"
	CodeBusy             Code = "busy"
	// CodeCanceled is a request its client gave up on.
	CodeCanceled Code = "canceled"
	// CodeInternal is a failure of embisto itself, such as a module that panicked.
	CodeInternal Code = "internal"
)

// Entry is a code with the HTTP status it is answered with and the words a client is shown.
type Entry struct {
	Code   Code
	Status int
	Text   string
}

// catalogue is every code embisto fails with; answers, the image route, the Iframely format and the API's spec are
// derived from it. A text never holds what a page or the network said.
var catalogue = []Entry{
	{CodeInvalidRequest, http.StatusBadRequest, "a parameter is malformed or out of range"},
	{CodeInvalidURL, http.StatusBadRequest, "the URL is not an http or https address embisto can preview"},
	{CodeBlockedURL, http.StatusForbidden, "the URL resolves to an address that is not public"},
	{CodeFetchFailed, http.StatusBadGateway, "the page could not be fetched"},
	{CodeTimeout, http.StatusGatewayTimeout, "the page did not answer in time"},
	{CodeTooLarge, http.StatusBadGateway, "the page is larger than the limit"},
	{CodeNotFound, http.StatusNotFound, "the page or the item does not exist, or was removed"},
	{CodeUnknownFormat, http.StatusBadRequest, "the server has no such format"},
	{CodeNotCached, http.StatusNotFound, "nothing of this URL was cached"},
	{CodeCacheUnavailable, http.StatusServiceUnavailable, "the cache did not answer; nothing was purged"},
	{CodeInvalidSignature, http.StatusForbidden, "the URL's signature does not match"},
	{CodeInvalidOptions, http.StatusBadRequest, "the size is neither a preset nor an allowed size"},
	{CodeTooManyPixels, http.StatusRequestEntityTooLarge, "the picture has more pixels than the limit"},
	{CodeUnsupportedImage, http.StatusUnsupportedMediaType, "the source is not a JPEG, PNG, GIF or WebP picture"},
	{CodeImageTooLarge, http.StatusRequestEntityTooLarge, "the picture's file is over the size limit"},
	{CodeBusy, http.StatusServiceUnavailable, "too many pictures are being processed; try again shortly"},
	{CodeCanceled, http.StatusServiceUnavailable, "the request was canceled"},
	{CodeInternal, http.StatusInternalServerError, "the request failed"},
}

// Catalogue is every code embisto fails with, with its status and words.
func Catalogue() []Entry {
	return slices.Clone(catalogue)
}

// Status is the HTTP status the code is answered with; an unknown code is internal.
func (c Code) Status() int {
	return c.entry().Status
}

// Text is the words a client is shown for the code.
func (c Code) Text() string {
	return c.entry().Text
}

func (c Code) entry() Entry {
	if at := slices.IndexFunc(catalogue, func(e Entry) bool { return e.Code == c }); at >= 0 {
		return catalogue[at]
	}
	return catalogue[len(catalogue)-1]
}

// Error is a preview that could not be made, with the code saying why. Its text is the catalogue's, or a Detail
// written for clients; what went wrong underneath, which may name addresses or quote a page, is only unwrapped and
// logged.
type Error struct {
	Code Code
	// Status is the HTTP status the page answered with, for a fetch_failed or not_found on an error status; zero
	// otherwise.
	Status int
	// Detail replaces the code's text with more precise words; never text from a page or the network.
	Detail string
	Err    error
}

func (e *Error) Error() string {
	if e.Detail != "" {
		return e.Detail
	}
	return e.Code.Text()
}

func (e *Error) Unwrap() error {
	return e.Err
}

// HTTPStatus is the status the failure is answered with: the code's, or 410 for an item the site says is gone.
func (e *Error) HTTPStatus() int {
	if e.Code == CodeNotFound && e.Status == http.StatusGone {
		return http.StatusGone
	}
	return e.Code.Status()
}

// LogValue logs the cause with the code, which the error's own text leaves out.
func (e *Error) LogValue() slog.Value {
	attrs := []slog.Attr{slog.String("code", string(e.Code))}
	if e.Status != 0 {
		attrs = append(attrs, slog.Int("status", e.Status))
	}
	if e.Err != nil {
		attrs = append(attrs, slog.String("cause", e.Err.Error()))
	}
	return slog.GroupValue(attrs...)
}

// CodeOf is the code of an *Error anywhere in the chain; CodeFetchFailed for any other error, and none for no error.
func CodeOf(err error) Code {
	if err == nil {
		return ""
	}
	if coded, ok := errors.AsType[*Error](err); ok {
		return coded.Code
	}
	return CodeFetchFailed
}
