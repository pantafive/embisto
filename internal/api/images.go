package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/thumbs"
)

// Images are the image proxy's parts: the signer that checks its URLs and the processor that makes the pictures.
type Images struct {
	Signer    *thumbs.Signer
	Processor thumbs.Processor
	// Observer hears about every picture asked for; nil hears nothing.
	Observer ImageObserver
}

// ImageObserver hears how each picture went: its outcome, never its URL.
type ImageObserver interface {
	Processed(outcome string, took time.Duration, bytesIn, bytesOut int)
}

const (
	// cacheForever lets every cache keep a picture for good: its URL is signed and never changes.
	cacheForever = "public, max-age=31536000, immutable"
	// cacheNever keeps a failure out of every cache, so a passing upstream fault does not stick for a year.
	cacheNever = "no-store"
	tagImages  = "Images"

	outcomeOK          = "ok"
	outcomeNotModified = "not_modified"
)

type imageInput struct {
	Signature   string `path:"signature" doc:"HMAC-SHA256 of size/source with the server's image key, unpadded base64url."`
	Size        string `path:"size" doc:"A preset's name, such as thumb, card or large, or WxH-fit / WxH-fill with sides from the allowed list; followed by .webp for WebP." example:"card.webp"`
	Source      string `path:"source" doc:"The original picture's URL, unpadded base64url."`
	IfNoneMatch string `header:"If-None-Match"`
}

type imageOutput struct {
	Status       int
	ContentType  string `header:"Content-Type"`
	CacheControl string `header:"Cache-Control"`
	ETag         string `header:"ETag"` //nolint:tagliatelle // the header's name
	NoSniff      string `header:"X-Content-Type-Options"`
	Body         []byte
}

// retryBusy is how soon a processing slot frees, for a picture refused as busy.
const retryBusy = "1"

// imageRefusals are the codes of the image proxy's own failures.
var imageRefusals = []struct {
	err  error
	code card.Code
}{
	{thumbs.ErrSignature, card.CodeInvalidSignature},
	{thumbs.ErrOptions, card.CodeInvalidOptions},
	{thumbs.ErrSource, card.CodeInvalidURL},
	{thumbs.ErrTooLarge, card.CodeImageTooLarge},
	{thumbs.ErrTooManyPixels, card.CodeTooManyPixels},
	{thumbs.ErrUnsupported, card.CodeUnsupportedImage},
	{thumbs.ErrBusy, card.CodeBusy},
}

// binary is the schema of a picture's bytes.
var binary = &huma.Schema{Type: huma.TypeString, Format: "binary"}

func addImageRoute(api huma.API, deps Deps) {
	if deps.Images == nil {
		return
	}
	huma.Register(api, huma.Operation{
		OperationID: "image",
		Method:      http.MethodGet,
		Path:        "/i/{signature}/{size}/{source}",
		Summary:     "Get a picture of a preview, resized",
		Description: "The URLs come in the `variants` of a card's `image`, `icon` and `images`, signed by the server; clients only open them. " +
			"The picture is fetched, decoded, resized into the size's box without ever growing, and served as JPEG, or as PNG when it has transparency; " +
			"a size ending in .webp serves lossy WebP, transparency kept, at about half the bytes. The format is part of the signed size, so caches never mix them. " +
			"Only JPEG, PNG, GIF and WebP are served, always re-encoded. Pictures are immutable and served with immutable cache headers. " +
			"Failures are problem+json and never cached: invalid_signature (403), invalid_options or invalid_url (400), image_too_large or too_many_pixels (413), " +
			"unsupported_image (415), busy (503, with Retry-After), and the fetch failures of a preview.",
		Tags: []string{tagImages},
		Responses: map[string]*huma.Response{
			"200": {Description: "The resized picture.", Content: map[string]*huma.MediaType{
				"image/jpeg": {Schema: binary}, "image/png": {Schema: binary}, "image/webp": {Schema: binary},
			}},
			"304": {Description: "The client's copy is current."},
		},
	}, deps.image)
}

func (deps Deps) image(ctx context.Context, in *imageInput) (*imageOutput, error) {
	started := time.Now()
	observe := func(outcome string, picture thumbs.Picture) {
		if deps.Images.Observer != nil {
			deps.Images.Observer.Processed(outcome, time.Since(started), picture.SourceBytes, len(picture.Body))
		}
	}
	request, err := deps.Images.Signer.Verify(in.Signature, in.Size, in.Source)
	if err != nil {
		return nil, deps.refuseImage(ctx, err, observe)
	}
	// The signature names exactly one picture, which never changes: it is the picture's tag.
	tag := `"` + in.Signature + `"`
	if in.IfNoneMatch == tag {
		observe(outcomeNotModified, thumbs.Picture{})
		return &imageOutput{Status: http.StatusNotModified, CacheControl: cacheForever, ETag: tag}, nil
	}
	picture, err := deps.Images.Processor.Process(ctx, request.Source, request.Box, request.Format)
	if err != nil {
		return nil, deps.refuseImage(ctx, err, observe)
	}
	observe(outcomeOK, picture)
	return &imageOutput{
		Status: http.StatusOK, ContentType: picture.ContentType, CacheControl: cacheForever, ETag: tag, NoSniff: noSniff,
		Body: picture.Body,
	}, nil
}

// refuseImage answers a failed picture as problem+json that no cache keeps.
func (deps Deps) refuseImage(ctx context.Context, err error, observe func(string, thumbs.Picture)) error {
	refused := imageRefusalOf(err)
	observe(string(refused.Code), thumbs.Picture{})
	deps.Log.InfoContext(ctx, "no picture", slog.Any("error", refused))
	problem := problemOf(refused)
	problem.headers = http.Header{"Cache-Control": {cacheNever}}
	if refused.Code == card.CodeBusy {
		problem.headers.Set("Retry-After", retryBusy)
	}
	return problem
}

func imageRefusalOf(err error) *card.Error {
	for _, known := range imageRefusals {
		if errors.Is(err, known.err) {
			return &card.Error{Code: known.code, Err: err}
		}
	}
	if errors.Is(err, context.Canceled) {
		return &card.Error{Code: card.CodeCanceled, Err: err}
	}
	if failed, ok := errors.AsType[*card.Error](err); ok {
		return failed
	}
	return &card.Error{Code: card.CodeFetchFailed, Err: err}
}
