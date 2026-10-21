package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/internal/valkeytest"
)

// TestAVariantWrittenDuringAPurgeKeepsItsIndex races a variant's write against its group's purge. A purge in three
// round trips let the variant survive it without its index, so that no later purge reached it.
func TestAVariantWrittenDuringAPurgeKeepsItsIndex(t *testing.T) {
	ctx := context.Background()
	url := valkeytest.Start(t).URL
	purger, err := newRedisStore(url)
	require.NoError(t, err)
	writer, err := newRedisStore(url)
	require.NoError(t, err)
	orphans := 0
	for round := range 300 {
		group := fmt.Sprintf("{race%d}", round)
		require.NoError(t, writer.Set(ctx, group, group+":a", []byte("a"), time.Hour))
		var wg sync.WaitGroup
		wg.Go(func() {
			_, err := purger.DeleteGroup(ctx, group)
			assert.NoError(t, err)
		})
		wg.Go(func() {
			assert.NoError(t, writer.Set(ctx, group, group+":b", []byte("b"), time.Hour))
		})
		wg.Wait()
		_, live, err := writer.Get(ctx, group+":b")
		require.NoError(t, err)
		indexed, err := writer.client.SIsMember(ctx, group+indexSuffix, group+":b").Result()
		require.NoError(t, err)
		if live && !indexed {
			orphans++
		}
	}

	assert.Zero(t, orphans)
}
