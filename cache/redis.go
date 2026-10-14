package cache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/pantafive/embisto/internal/rediscache"
)

// downFor is how long a shared cache that failed is left alone before it is tried again.
const downFor = 5 * time.Second

var errSharedCacheDown = errors.New("the shared cache does not answer")

// RedisOptions shape a shared store.
type RedisOptions struct {
	// Fallback serves while the shared store does not answer: each instance keeps its own previews then, and
	// identical requests make one preview per instance rather than one between them. Nil keeps up to 10000 entries
	// in memory. It is emptied when the shared store answers again, so that nothing it kept outlives a purge made
	// meanwhile on the shared store.
	Fallback *Memory
	Log      *slog.Logger
	Observer RedisObserver
	// Now is the clock the outage is timed by; nil is the real one.
	Now func() time.Time
}

// defaultFallbackBudget is how many bytes an instance keeps for itself while the shared store is down.
const defaultFallbackBudget = 64 << 20

// Redis is the shared store while it answers, the fallback while it does not.
type Redis struct {
	shared *rediscache.Cache
	opts   RedisOptions

	mu        sync.Mutex
	downUntil time.Time
	isDown    bool
}

// NewRedis shares a store between instances through Redis or anything that speaks its protocol: Valkey, KeyDB,
// Dragonfly. The URL is redis:// or rediss://. While it does not answer, the fallback serves, with a warning when that
// starts and ends; a purge then fails, since what it drops would reappear when the shared store is back. It is also
// the Locker the instances agree through.
func NewRedis(rawURL string, opts RedisOptions) (*Redis, error) {
	shared, err := rediscache.New(rawURL)
	if err != nil {
		return nil, err //nolint:wrapcheck // it says what is wrong with the URL without repeating it
	}
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Fallback == nil {
		opts.Fallback = NewMemory(defaultFallbackBudget, opts.Now)
	}
	return &Redis{shared: shared, opts: opts}, nil
}

func (f *Redis) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if f.down() {
		return f.opts.Fallback.Get(ctx, key)
	}
	value, ok, err := f.shared.Get(ctx, key)
	if f.failed(ctx, err) {
		return f.opts.Fallback.Get(ctx, key)
	}
	return value, ok, nil
}

func (f *Redis) Set(ctx context.Context, group, key string, value []byte, ttl time.Duration) error {
	if f.down() || f.failed(ctx, f.shared.Set(ctx, group, key, value, ttl)) {
		return f.opts.Fallback.Set(ctx, group, key, value, ttl)
	}
	return nil
}

func (f *Redis) Delete(ctx context.Context, key string) (int, error) {
	if f.down() {
		return 0, errSharedCacheDown
	}
	dropped, err := f.shared.Delete(ctx, key)
	if f.failed(ctx, err) {
		return 0, fmt.Errorf("%w: %w", errSharedCacheDown, err)
	}
	return dropped, nil
}

func (f *Redis) DeleteGroup(ctx context.Context, group string) (int, error) {
	if f.down() {
		return 0, errSharedCacheDown
	}
	dropped, err := f.shared.DeleteGroup(ctx, group)
	if f.failed(ctx, err) {
		return 0, fmt.Errorf("%w: %w", errSharedCacheDown, err)
	}
	return dropped, nil
}

// Lock takes the shared lock; while the shared cache does not answer, every instance makes its own preview, so the
// lock is granted at once.
func (f *Redis) Lock(ctx context.Context, key string, ttl time.Duration) (func(context.Context) error, bool, error) {
	alone := func(context.Context) error { return nil }
	if f.down() {
		return alone, true, nil
	}
	release, ok, err := f.shared.Lock(ctx, key, ttl)
	if f.failed(ctx, err) {
		return alone, true, nil
	}
	return release, ok, nil
}

func (f *Redis) down() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.isDown && f.opts.Now().Before(f.downUntil)
}

// failed notes how an operation went: an error marks the shared cache down for a while, a success after an outage
// marks it up again. It says whether the operation failed.
func (f *Redis) failed(ctx context.Context, err error) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		if f.isDown {
			f.isDown = false
			// What this instance kept alone may since have been purged from the shared store.
			f.opts.Fallback.Clear()
			f.opts.Log.InfoContext(ctx, "the shared cache answers again")
			f.backendUp(true)
		}
		return false
	}
	if ctx.Err() != nil {
		// The caller gave up, which says nothing of the shared store.
		return true
	}
	if f.opts.Observer != nil {
		f.opts.Observer.RedisFailed()
	}
	if !f.isDown {
		f.opts.Log.WarnContext(ctx, "the shared cache does not answer; this instance serves from memory", slog.Any("error", err))
		f.backendUp(false)
	}
	f.isDown, f.downUntil = true, f.opts.Now().Add(downFor)
	return true
}

func (f *Redis) backendUp(isUp bool) {
	if f.opts.Observer != nil {
		f.opts.Observer.RedisUp(isUp)
	}
}
