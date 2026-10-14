package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/cache"
)

type clock struct {
	at time.Time
}

func (c *clock) now() time.Time {
	return c.at
}

func TestMemoryForgetsAValueOnceItExpires(t *testing.T) {
	ctx, clock := context.Background(), &clock{at: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	memory := cache.NewMemory(1<<20, clock.now)
	require.NoError(t, memory.Set(ctx, "g", "k", []byte("v"), time.Minute))

	clock.at = clock.at.Add(time.Minute)
	_, ok, err := memory.Get(ctx, "k")

	require.NoError(t, err)
	assert.False(t, ok)
}

func TestMemoryEvictsTheLeastRecentlyUsedWhenOverBudget(t *testing.T) {
	ctx, clock := context.Background(), &clock{at: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	// Room for two entries of this size, each costing its key, group, value and overhead.
	memory := cache.NewMemory(2*cache.CostOf("g", "a", 100), clock.now)
	value := make([]byte, 100)
	require.NoError(t, memory.Set(ctx, "g", "a", value, time.Hour))
	require.NoError(t, memory.Set(ctx, "g", "b", value, time.Hour))
	_, _, err := memory.Get(ctx, "a")
	require.NoError(t, err)

	require.NoError(t, memory.Set(ctx, "g", "c", value, time.Hour))

	kept := map[string]bool{}
	for _, key := range []string{"a", "b", "c"} {
		_, ok, err := memory.Get(ctx, key)
		require.NoError(t, err)
		kept[key] = ok
	}
	assert.Equal(t, map[string]bool{"a": true, "b": false, "c": true}, kept)
}

func TestMemoryKeepsNothingLargerThanItsWholeBudget(t *testing.T) {
	ctx := context.Background()
	memory := cache.NewMemory(1<<10, nil)

	require.NoError(t, memory.Set(ctx, "g", "huge", make([]byte, 4<<10), time.Hour))
	_, ok, err := memory.Get(ctx, "huge")

	require.NoError(t, err)
	assert.False(t, ok)
}

func TestMemoryDropsAGroupWithoutTouchingOthers(t *testing.T) {
	ctx := context.Background()
	memory := cache.NewMemory(1<<20, nil)
	for _, entry := range [][2]string{{"a", "a1"}, {"a", "a2"}, {"b", "b1"}} {
		require.NoError(t, memory.Set(ctx, entry[0], entry[1], []byte("v"), time.Hour))
	}

	dropped, err := memory.DeleteGroup(ctx, "a")
	require.NoError(t, err)
	_, kept, err := memory.Get(ctx, "b1")

	require.NoError(t, err)
	assert.Equal(t, []any{2, true}, []any{dropped, kept})
}
