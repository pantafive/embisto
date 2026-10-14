package thumbnails

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/pantafive/embisto/card"
)

var (
	// ErrTooLarge is a source file over the byte limit.
	ErrTooLarge = errors.New("the picture's file is too large")
	// ErrBusy is a picture refused because the processor had no free slot in time.
	ErrBusy = errors.New("too many pictures are being processed")
)

// Picture is an encoded picture to serve.
type Picture struct {
	Body        []byte
	ContentType string
	// SourceBytes is how much of the source was read, for metrics.
	SourceBytes int
}

// Processor makes the picture a verified URL asks for. The built-in one resizes in the process; an adapter may hand
// the work to an image service such as imgproxy.
type Processor interface {
	Process(ctx context.Context, source *url.URL, box Box, format Format) (Picture, error)
}

// Limits bound what one picture may cost and how many are processed at once.
type Limits struct {
	// MaxBytes is the largest source file read.
	MaxBytes int64
	// MaxPixels is the most pixels a source may have; its header is checked before it is decoded, since a decoded
	// picture takes four bytes a pixel.
	MaxPixels int
	// MaxMemory bounds the memory every picture being resized takes at once, decoded and scaling; each is admitted
	// by what its header says it will take, and waits up to Wait for room. One that could never fit is refused.
	MaxMemory int64
	// Concurrency is how many pictures are processed at once; more wait for a slot up to Wait.
	Concurrency int
	Wait        time.Duration
}

// Builtin resizes in the process, in pure Go: JPEG, PNG, the first frame of a GIF and WebP in; JPEG, or PNG for a
// picture with transparency, out, or WebP when asked.
type Builtin struct {
	fetcher card.Fetcher
	limits  Limits
	slots   chan struct{}
	memory  *semaphore.Weighted
}

// NewBuiltin fetches sources through the fetcher, which should be the one that refuses addresses that are not
// public.
func NewBuiltin(fetcher card.Fetcher, limits Limits) *Builtin {
	return &Builtin{fetcher: fetcher, limits: limits, slots: make(chan struct{}, limits.Concurrency), memory: semaphore.NewWeighted(limits.MaxMemory)}
}

func (b *Builtin) Process(ctx context.Context, source *url.URL, box Box, format Format) (Picture, error) {
	// The slot comes first: a request waiting for one holds no file in memory.
	waiting, cancel := context.WithTimeout(ctx, b.limits.Wait)
	defer cancel()
	select {
	case b.slots <- struct{}{}:
		defer func() { <-b.slots }()
	case <-waiting.Done():
		if ctx.Err() != nil {
			return Picture{}, ctx.Err() //nolint:wrapcheck // the caller's own cancellation
		}
		return Picture{}, ErrBusy
	}
	resp, err := b.fetcher.Peek(ctx, source, b.limits.MaxBytes+1)
	if err != nil {
		return Picture{}, err //nolint:wrapcheck // the fetcher's *card.Error is the contract
	}
	if int64(len(resp.Body)) > b.limits.MaxBytes {
		return Picture{}, fmt.Errorf("%w: over %d bytes", ErrTooLarge, b.limits.MaxBytes)
	}
	cost, err := Cost(resp.Body, box, b.limits.MaxPixels)
	if err != nil {
		return Picture{}, err
	}
	if err := b.admit(ctx, cost); err != nil {
		return Picture{}, err
	}
	defer b.memory.Release(cost)
	result, err := resize(resp.Body, box, b.limits.MaxPixels, format)
	if err != nil {
		return Picture{}, err
	}
	return Picture{Body: result.Body, ContentType: result.ContentType, SourceBytes: len(resp.Body)}, nil
}

// admit waits up to Wait for room for a picture that will take cost bytes; one that could never fit is refused.
func (b *Builtin) admit(ctx context.Context, cost int64) error {
	if cost > b.limits.MaxMemory {
		return fmt.Errorf("%w: resizing takes %d bytes, over the %d all pictures share", ErrTooManyPixels, cost, b.limits.MaxMemory)
	}
	// The wait starts afresh: the fetch may have taken the slot's whole wait.
	admitted, cancel := context.WithTimeout(ctx, b.limits.Wait)
	defer cancel()
	if err := b.memory.Acquire(admitted, cost); err != nil {
		if ctx.Err() != nil {
			return ctx.Err() //nolint:wrapcheck // the caller's own cancellation
		}
		return ErrBusy
	}
	return nil
}
