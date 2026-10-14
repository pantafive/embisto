package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/cache"
	"github.com/pantafive/embisto/internal/valkeytest"
)

func TestCacheDropsAGroupOrOneKey(t *testing.T) {
	ctx := context.Background()
	store, err := cache.NewRedisStore(valkeytest.Start(t).URL)
	require.NoError(t, err)
	for _, key := range []string{"{a}:1", "{a}:2", "{a}:3"} {
		require.NoError(t, store.Set(ctx, "{a}", key, []byte("v"), time.Minute))
	}

	one, err := store.Delete(ctx, "{a}:1")
	require.NoError(t, err)
	rest, err := store.DeleteGroup(ctx, "{a}")
	require.NoError(t, err)
	_, found, err := store.Get(ctx, "{a}:2")

	require.NoError(t, err)
	assert.Equal(t, []any{1, 2, false}, []any{one, rest, found})
}

func TestLockIsHeldByOneAndFreedOnlyByItsHolder(t *testing.T) {
	ctx := context.Background()
	store, err := cache.NewRedisStore(valkeytest.Start(t).URL)
	require.NoError(t, err)
	release, first, err := store.Lock(ctx, "{a}:lock", 100*time.Millisecond)
	require.NoError(t, err)
	_, second, err := store.Lock(ctx, "{a}:lock", time.Minute)
	require.NoError(t, err)
	time.Sleep(150 * time.Millisecond)
	_, afterExpiry, err := store.Lock(ctx, "{a}:lock", time.Minute)
	require.NoError(t, err)

	// The first holder's lock expired and another took it: releasing must not free the other's.
	require.NoError(t, release(ctx))
	_, afterStaleRelease, err := store.Lock(ctx, "{a}:lock", time.Minute)

	require.NoError(t, err)
	assert.Equal(t, []bool{true, false, true, false}, []bool{first, second, afterExpiry, afterStaleRelease})
}
