package cache_test

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/cache"
	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/valkeytest"
)

func localCore() *embisto.Core {
	return embisto.New(embisto.Options{AllowedNetworks: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}})
}

// backends hears whether a shared store answers.
type backends struct {
	mu     sync.Mutex
	events []string
}

func (b *backends) RedisUp(isUp bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, map[bool]string{true: "up", false: "down"}[isUp])
}

func (b *backends) RedisFailed() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, "failed")
}

func TestACallerThatGaveUpLeavesTheSharedStoreUp(t *testing.T) {
	valkey := valkeytest.Start(t)
	observer := &backends{}
	shared, err := cache.NewRedis(valkey.URL, cache.RedisOptions{Observer: observer})
	require.NoError(t, err)
	opts := options
	opts.Now, opts.Locker = time.Now, shared
	previews := cache.New(localCore(), shared, opts)
	gaveUp, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = previews.Extract(gaveUp, card.Request{URL: "https://example.com/post"})
	require.Error(t, err)
	_, err = previews.Purge(context.Background(), card.Request{URL: "https://example.com/post"})

	require.NoError(t, err)
	assert.Empty(t, observer.events)
}
