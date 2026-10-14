package cache

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/target"
)

// Options say how long previews and failures are kept, and how instances sharing a store agree.
type Options struct {
	// TTL is how long a preview is kept when its origin says nothing about it.
	TTL time.Duration
	// MinTTL and MaxTTL bound how long a preview is kept, whatever its origin says or does not say; a page that
	// forbids keeping it is kept MinTTL, so identical requests still make one preview. Zero lifts the bound.
	MinTTL time.Duration
	MaxTTL time.Duration
	// FailureTTL is how long a failure with a code is kept.
	FailureTTL time.Duration
	// Locker, for a store shared between instances, lets them make one preview between them; nil makes one per
	// instance.
	Locker Locker
	// LockTTL is how long an instance may hold a preview's lock; it outlasts the slowest preview, so a crashed holder
	// frees it in time. Zero is a minute.
	LockTTL time.Duration
	// Wait is how long an instance waits for another's preview before making its own. Zero is LockTTL.
	Wait time.Duration
	// Log hears about a store that failed; the preview is then made afresh. Nil logs nothing.
	Log *slog.Logger
	// Observer hears about lookups, coalescing, locks and the TTLs chosen; nil hears nothing.
	Observer Observer
	// Now is the clock entries expire by; nil is the real one.
	Now func() time.Time
}

// version changes when a kept entry's shape does, so a shared store never serves an old one.
const version = "v1"

const (
	defaultLockTTL = time.Minute
	// staleGrace keeps an entry a little past its expiry, so that finding it can be counted as stale.
	staleGrace = time.Minute
	firstPoll  = 25 * time.Millisecond
	lastPoll   = 250 * time.Millisecond
)

// Layer serves previews from a store, and makes one preview for many identical requests that arrive at once: in the
// process always, across instances with a Locker. A preview is kept as long as its origin allows, within the bounds;
// a failure with a code for less long; a caller that gave up, not at all.
type Layer struct {
	next  card.Extractor
	store Store
	opts  Options
	calls singleflight.Group
}

// New keeps the previews next makes in the store.
func New(next card.Extractor, store Store, opts Options) *Layer {
	opts.Log = cmp.Or(opts.Log, slog.New(slog.DiscardHandler))
	if opts.Observer == nil {
		opts.Observer = unobserved{}
	}
	opts.LockTTL = cmp.Or(opts.LockTTL, defaultLockTTL)
	opts.Wait = cmp.Or(opts.Wait, opts.LockTTL)
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Layer{next: next, store: store, opts: opts}
}

// entry is what the store keeps of a preview or a failure.
type entry struct {
	Preview   *card.Preview  `json:"preview,omitempty"`
	Provider  string         `json:"provider,omitempty"`
	Freshness card.Freshness `json:"freshness"`
	Made      time.Time      `json:"made"`
	Code      card.Code      `json:"code,omitempty"`
	Status    int            `json:"status,omitempty"`
	Detail    string         `json:"detail,omitempty"`
	ExpiresAt time.Time      `json:"expires_at"`
	// err is a failure not kept, such as a caller that gave up.
	err error
}

func (l *Layer) Extract(ctx context.Context, req card.Request) (card.Result, error) {
	key, err := keyOf(req)
	if err != nil {
		return card.Result{}, err
	}
	if found, result := l.lookup(ctx, key.name); result == LookupHit {
		return l.result(found)
	}
	isFirst := false
	done := l.calls.DoChan(key.name, func() (filled any, _ error) {
		isFirst = true
		// DoChan fills on a goroutine of its own, where a panic would end the process: it fails this preview instead.
		defer func() {
			if recovered := recover(); recovered != nil {
				l.opts.Log.ErrorContext(ctx, "a layer panicked; the preview fails", slog.Any("panic", recovered), slog.String("stack", string(debug.Stack())))
				filled = entry{err: &card.Error{Code: card.CodeInternal, Err: fmt.Errorf("panic: %v", recovered)}} //nolint:err113 // the panic's own value
			}
		}()
		// One caller giving up must not fail the others waiting on the same preview; the core has its own timeout.
		return l.fill(context.WithoutCancel(ctx), key, req), nil
	})
	select {
	case <-ctx.Done():
		return card.Result{}, ctx.Err() //nolint:wrapcheck // the caller's own cancellation
	case result := <-done:
		if !isFirst {
			l.opts.Observer.Coalesced(CoalescedLocal)
		}
		found, _ := result.Val.(entry) // the call returns an entry
		return l.result(found)
	}
}

// Purge drops the request's kept previews after the layers inside have dropped theirs, and counts them all.
func (l *Layer) Purge(ctx context.Context, req card.Request) (int, error) {
	inside, err := l.next.Purge(ctx, req)
	if err != nil {
		return 0, err //nolint:wrapcheck // the next layer's own failure
	}
	key, err := keyOf(req)
	if err != nil {
		return 0, err
	}
	var dropped int
	if req.Options == (card.Options{}) {
		dropped, err = l.store.DeleteGroup(ctx, key.group)
	} else {
		dropped, err = l.store.Delete(ctx, key.name)
	}
	if err != nil {
		return 0, fmt.Errorf("purging: %w", err)
	}
	return inside + dropped, nil
}

// fill makes the preview, or, when another instance is making it, waits for that one: polling the store, taking the
// lock when it is freed without a value, and making the preview itself when the wait runs out rather than failing.
func (l *Layer) fill(ctx context.Context, key cacheKey, req card.Request) entry {
	if l.opts.Locker == nil {
		return l.make(ctx, key, req)
	}
	deadline, poll := l.opts.Now().Add(l.opts.Wait), firstPoll
	for waited := false; ; waited = true {
		release, ok, err := l.opts.Locker.Lock(ctx, key.name+":lock", l.opts.LockTTL)
		switch {
		case err != nil:
			l.opts.Observer.Locked(LockError)
			return l.make(ctx, key, req)
		case ok:
			defer func() {
				if err := release(ctx); err != nil {
					l.opts.Log.WarnContext(ctx, "a preview's lock was not released; it expires on its own", slog.Any("error", err))
				}
			}()
			// The holder that just let go may have written it, even when the lock came at once: it may have been freed
			// between this instance's lookup and its lock.
			if found, result := l.recheck(ctx, key.name); result == LookupHit {
				l.opts.Observer.Locked(LockWaited)
				l.opts.Observer.Coalesced(CoalescedShared)
				return found
			}
			l.opts.Observer.Locked(LockAcquired)
			return l.make(ctx, key, req)
		}
		if waited {
			if found, result := l.recheck(ctx, key.name); result == LookupHit {
				l.opts.Observer.Locked(LockWaited)
				l.opts.Observer.Coalesced(CoalescedShared)
				return found
			}
		}
		if !l.opts.Now().Before(deadline) {
			l.opts.Observer.Locked(LockTimeout)
			return l.make(ctx, key, req)
		}
		time.Sleep(poll)
		poll = min(2*poll, lastPoll)
	}
}

// make runs the next extractor and keeps what it made for as long as the responses it was made from allow.
func (l *Layer) make(ctx context.Context, key cacheKey, req card.Request) entry {
	made, err := l.next.Extract(ctx, req)
	kept := entry{Preview: &made.Preview, Provider: made.Provider, Freshness: made.Freshness, Made: l.opts.Now()}
	ttl, source := l.ttl(made.Freshness)
	if err != nil {
		failed, ok := errors.AsType[*card.Error](err)
		if !ok {
			return entry{err: err}
		}
		kept = entry{Code: failed.Code, Status: failed.Status, Detail: failed.Detail, Made: l.opts.Now()}
		ttl, source = l.opts.FailureTTL, TTLFailure
	}
	// A layer inside that keeps the preview less long bounds this one: an outer cache never outlives an inner one.
	if !made.Expires.IsZero() {
		ttl = min(ttl, made.Expires.Sub(l.opts.Now()))
	}
	l.opts.Observer.TTLChosen(source)
	if ttl <= 0 {
		return kept
	}
	kept.ExpiresAt = l.opts.Now().Add(ttl)
	l.keep(ctx, key, kept, ttl)
	return kept
}

// ttl is how long to keep a preview whose origin said what it did, within the bounds, and where that came from.
func (l *Layer) ttl(said card.Freshness) (time.Duration, string) {
	ttl, source := l.opts.TTL, TTLDefault
	if said.Known {
		ttl, source = said.MaxAge, TTLOrigin
	}
	switch {
	case l.opts.MinTTL > 0 && ttl < l.opts.MinTTL:
		return l.opts.MinTTL, TTLMin
	case l.opts.MaxTTL > 0 && ttl > l.opts.MaxTTL:
		return l.opts.MaxTTL, TTLMax
	default:
		return ttl, source
	}
}

func (l *Layer) keep(ctx context.Context, key cacheKey, kept entry, ttl time.Duration) {
	raw, err := json.Marshal(kept)
	if err != nil {
		l.opts.Log.ErrorContext(ctx, "a preview could not be kept", slog.Any("error", err))
		return
	}
	if err := l.store.Set(ctx, key.group, key.name, raw, ttl+min(ttl, staleGrace)); err != nil {
		l.opts.Log.WarnContext(ctx, "the store failed to keep a preview", slog.Any("error", err))
	}
}

// lookup is the entry under the key: a hit, a miss, or stale when found past its expiry, which is a miss too.
func (l *Layer) lookup(ctx context.Context, key string) (entry, string) {
	found, result := l.recheck(ctx, key)
	l.opts.Observer.LookedUp(result)
	return found, result
}

// recheck is lookup unheard by the observer, for the looks a fill takes again while it waits for another instance:
// a request is one lookup however often it looks.
func (l *Layer) recheck(ctx context.Context, key string) (entry, string) {
	raw, ok, err := l.store.Get(ctx, key)
	result := LookupMiss
	var found entry
	switch {
	case err != nil:
		l.opts.Log.WarnContext(ctx, "the store failed to answer; the preview is made afresh", slog.Any("error", err))
	case !ok:
	case json.Unmarshal(raw, &found) != nil:
		l.opts.Log.WarnContext(ctx, "a kept preview is unreadable; it is made afresh")
	case !l.opts.Now().Before(found.ExpiresAt):
		result = LookupStale
	default:
		result = LookupHit
	}
	return found, result
}

// result is what an entry serves: the preview with the freshness its origin has left, or the failure. Both carry
// when the entry expires.
func (l *Layer) result(found entry) (card.Result, error) {
	served := card.Result{Expires: found.ExpiresAt}
	switch {
	case found.err != nil:
		return card.Result{}, found.err
	case found.Preview == nil:
		return served, &card.Error{Code: found.Code, Status: found.Status, Detail: found.Detail}
	}
	served.Preview, served.Provider, served.Freshness = *found.Preview, found.Provider, found.Freshness
	if served.Freshness.Known {
		served.Freshness.MaxAge = max(0, served.Freshness.MaxAge-l.opts.Now().Sub(found.Made))
	}
	return served, nil
}

// cacheKey names a variant of a URL's preview, in the group of all its variants. The URL's hash is in braces, a Redis
// hash tag, so that a URL's variants, its index and its locks live together.
type cacheKey struct {
	group string
	name  string
}

// keyOf is the key of a request as the pipeline reads it: the URL normalized, the language canonical, so that equal
// requests written differently share one entry. A URL the pipeline refuses is refused here too.
func keyOf(req card.Request) (cacheKey, error) {
	parsed, err := target.Parse(req.URL)
	if err != nil {
		return cacheKey{}, err //nolint:wrapcheck // the *card.Error is the contract
	}
	opts := req.Options
	opts.Language = target.Language(opts.Language)
	url := sha256.Sum256([]byte(parsed.String()))
	options, err := json.Marshal(opts)
	if err != nil {
		return cacheKey{}, fmt.Errorf("keying the options: %w", err)
	}
	variant := sha256.Sum256(options)
	group := "embisto:preview:" + version + ":{" + hex.EncodeToString(url[:16]) + "}"
	return cacheKey{group: group, name: group + ":" + hex.EncodeToString(variant[:16])}, nil
}
