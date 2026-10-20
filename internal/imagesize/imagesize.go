// Package imagesize measures a preview's pictures from the first bytes of their files, so a client can lay a card out
// before the pictures arrive.
package imagesize

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/pantafive/embisto/card"
)

// Enricher fills in the sizes the sources did not state.
type Enricher struct {
	// Concurrency is how many pictures are read at once.
	Concurrency int
	// Timeout bounds the whole measuring; a picture not read by then stays unmeasured.
	Timeout time.Duration
	// MaxBytes is how much of a file is read: headers sit at the start, a JPEG's after its metadata.
	MaxBytes int64
	// MaxImages caps the pictures measured, the icon and the main image first.
	MaxImages int
}

// errPanicked is a measure that panicked.
var errPanicked = errors.New("panicked")

type dimensions struct {
	width, height int
}

func (Enricher) Name() string {
	return "imagesize"
}

func (e Enricher) Enrich(ctx context.Context, preview card.Preview, fetcher card.Fetcher) (card.Preview, error) {
	wanted := unmeasured(preview)
	if len(wanted) > e.MaxImages {
		wanted = wanted[:e.MaxImages]
	}
	if len(wanted) == 0 {
		return preview, nil
	}
	ctx, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()
	var mu sync.Mutex
	measured := map[string]dimensions{}
	var group errgroup.Group
	group.SetLimit(e.Concurrency)
	for _, href := range wanted {
		group.Go(func() (err error) {
			// A picture's bytes come from anyone, and errgroup does not hand a worker's panic back to Wait: unrecovered
			// here, a decoder's panic would end the program.
			defer func() {
				if recovered := recover(); recovered != nil {
					err = fmt.Errorf("%w measuring %s: %v\n%s", errPanicked, href, recovered, debug.Stack())
				}
			}()
			if found, ok := e.measure(ctx, fetcher, href); ok {
				mu.Lock()
				measured[href] = found
				mu.Unlock()
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return preview, err //nolint:wrapcheck // only a panic fails a measure; an unreadable picture stays unmeasured
	}
	return apply(preview, measured), nil
}

func (e Enricher) measure(ctx context.Context, fetcher card.Fetcher, href string) (dimensions, bool) {
	target, err := url.Parse(href)
	if err != nil {
		return dimensions{}, false
	}
	resp, err := fetcher.Peek(ctx, target, e.MaxBytes)
	if err != nil {
		return dimensions{}, false
	}
	width, height, ok := Size(resp.Body)
	return dimensions{width, height}, ok
}

// unmeasured are the pictures of a preview without a size, each once, the icon and the main image first.
func unmeasured(preview card.Preview) []string {
	var found []string
	add := func(href string, width int) {
		if href != "" && width == 0 && !slices.Contains(found, href) {
			found = append(found, href)
		}
	}
	add(preview.Icon.URL, preview.Icon.Width)
	add(preview.Image.URL, preview.Image.Width)
	for _, image := range preview.Images {
		add(image.URL, image.Width)
	}
	for _, link := range preview.Links {
		if isPicture(link) {
			add(link.Href, link.Media.Width)
		}
	}
	return found
}

func isPicture(link card.Link) bool {
	if link.Type != "" {
		return strings.HasPrefix(link.Type, "image/")
	}
	return slices.ContainsFunc(link.Rel, func(rel string) bool {
		return rel == card.RelIcon || rel == card.RelThumbnail || rel == card.RelImage
	})
}

// apply puts each measured size everywhere its picture appears, so the fields and the links agree.
func apply(preview card.Preview, measured map[string]dimensions) card.Preview {
	sized := func(image card.Image) card.Image {
		if found, ok := measured[image.URL]; ok && image.Width == 0 {
			image.Width, image.Height = found.width, found.height
		}
		return image
	}
	preview.Icon, preview.Image = sized(preview.Icon), sized(preview.Image)
	preview.Images = slices.Clone(preview.Images)
	for i, image := range preview.Images {
		preview.Images[i] = sized(image)
	}
	preview.Links = slices.Clone(preview.Links)
	for i, link := range preview.Links {
		if found, ok := measured[link.Href]; ok && link.Media.Width == 0 {
			preview.Links[i].Media = card.MediaOf(found.width, found.height)
		}
	}
	return preview
}
