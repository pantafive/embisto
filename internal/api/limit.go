package api

import (
	"context"
	"errors"

	"github.com/pantafive/embisto/card"
)

// errBusy is a preview refused because as many as the limit are being made. It carries no code, so no cache keeps it.
var errBusy = errors.New("too many previews are being made")

// busyDetail is what a client is told of a preview refused as busy; the catalogue's words for busy are of pictures.
const busyDetail = "too many previews are being made; try again shortly"

// limited makes at most as many previews at once as it has slots, and refuses the rest at once: each preview being
// made holds its page, so the slots bound the memory they take.
type limited struct {
	next  card.Extractor
	slots chan struct{}
}

// Limited makes at most n previews at once. It goes inside the cache, so a preview served from it takes no slot, and
// identical requests coalesced by it take one.
func Limited(next card.Extractor, n int) card.Extractor {
	return &limited{next: next, slots: make(chan struct{}, n)}
}

func (l *limited) Extract(ctx context.Context, req card.Request) (card.Result, error) {
	select {
	case l.slots <- struct{}{}:
	default:
		return card.Result{}, errBusy
	}
	defer func() { <-l.slots }()
	return l.next.Extract(ctx, req) //nolint:wrapcheck // the layer inside's own failure
}

func (l *limited) Purge(ctx context.Context, req card.Request) (int, error) {
	return l.next.Purge(ctx, req) //nolint:wrapcheck // the layer inside's own failure
}
