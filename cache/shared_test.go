package cache_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto"
	"github.com/pantafive/embisto/cache"
	"github.com/pantafive/embisto/card"
	"github.com/pantafive/embisto/internal/valkeytest"
)

// countingOrigin serves a slow page and counts how often it was asked for it, apart from the home page and pictures
// the pipeline also fetches.
func countingOrigin(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	// Slow enough that every caller arrives while the first is still waiting.
	return heldOrigin(t, hits, func(*http.Request) { time.Sleep(200 * time.Millisecond) })
}

// heldOrigin is countingOrigin with the page served once hold returns.
func heldOrigin(t *testing.T, hits *atomic.Int32, hold func(*http.Request)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path != "/post" {
			return
		}
		hits.Add(1)
		hold(r)
		_, _ = io.WriteString(w, `<title>One post</title><meta name="description" content="Said once.">`)
	}))
	t.Cleanup(server.Close)
	return server
}

func localCore() *embisto.Core {
	return embisto.New(embisto.Options{AllowedNetworks: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}})
}

// together runs calls at the same moment and gives every result.
func together(calls int, call func(i int) (card.Result, error)) ([]card.Result, []error) {
	results, errs := make([]card.Result, calls), make([]error, calls)
	var start, done sync.WaitGroup
	start.Add(1)
	for i := range calls {
		done.Go(func() {
			start.Wait()
			results[i], errs[i] = call(i)
		})
	}
	start.Done()
	done.Wait()
	return results, errs
}

func titlesOf(t *testing.T, results []card.Result, errs []error) map[string]int {
	t.Helper()
	titles := map[string]int{}
	for i, err := range errs {
		require.NoError(t, err)
		titles[results[i].Preview.Title]++
	}
	return titles
}

func TestLayerFetchesOnceForFiveHundredCallersAtOnce(t *testing.T) {
	var hits atomic.Int32
	origin := countingOrigin(t, &hits)
	previews := cache.New(localCore(), cache.NewMemory(1<<20, nil), cache.Options{TTL: time.Hour})

	results, errs := together(500, func(int) (card.Result, error) {
		return previews.Extract(context.Background(), card.Request{URL: origin.URL + "/post"})
	})

	assert.Equal(t, []any{int32(1), map[string]int{"One post": 500}}, []any{hits.Load(), titlesOf(t, results, errs)})
}

// instance is one embisto, as a server builds it: its own core, its own Redis client and its own memory, sharing
// only the Valkey every instance shares.
func instance(t *testing.T, url string, observer *recorder) *cache.Layer {
	t.Helper()
	shared, err := cache.NewRedis(url, cache.RedisOptions{})
	require.NoError(t, err)
	opts := options
	opts.Now, opts.Observer, opts.Locker, opts.LockTTL, opts.Wait = time.Now, observer, shared, 10*time.Second, 10*time.Second
	return cache.New(localCore(), shared, opts)
}

func TestTwoInstancesFetchOnceForManyCallersAtOnce(t *testing.T) {
	valkey := valkeytest.Start(t)
	var hits atomic.Int32
	origin := countingOrigin(t, &hits)
	observers := []*recorder{{}, {}}
	instances := []*cache.Layer{instance(t, valkey.URL, observers[0]), instance(t, valkey.URL, observers[1])}

	results, errs := together(200, func(i int) (card.Result, error) {
		return instances[i%2].Extract(context.Background(), card.Request{URL: origin.URL + "/post"})
	})

	waited := observers[0].has("locked waited") || observers[1].has("locked waited")
	assert.Equal(t, []any{int32(1), map[string]int{"One post": 200}, true}, []any{hits.Load(), titlesOf(t, results, errs), waited})
}

func TestAnInstanceMakesThePreviewItselfWhenTheLockHolderVanishes(t *testing.T) {
	valkey := valkeytest.Start(t)
	var hits atomic.Int32
	origin := countingOrigin(t, &hits)
	observer := &recorder{}
	previews := instance(t, valkey.URL, observer)
	// A holder that crashed: the lock is taken and nobody will write the preview or free the lock.
	locks, err := cache.NewRedisStore(valkey.URL)
	require.NoError(t, err)
	_, ok, err := locks.Lock(context.Background(), cache.LockKeyOf(origin.URL+"/post"), 300*time.Millisecond)
	require.NoError(t, err)
	require.True(t, ok)

	result, err := previews.Extract(context.Background(), card.Request{URL: origin.URL + "/post"})

	require.NoError(t, err)
	assert.Equal(t, []any{"One post", int32(1), true}, []any{result.Preview.Title, hits.Load(), observer.has("locked acquired")})
}

func TestAnInstanceServesFromMemoryWhileTheSharedStoreIsDown(t *testing.T) {
	valkey := valkeytest.Start(t)
	var hits atomic.Int32
	origin := countingOrigin(t, &hits)
	previews := instance(t, valkey.URL, &recorder{})
	valkey.Stop(t)
	req := card.Request{URL: origin.URL + "/post"}

	started := time.Now()
	first, err := previews.Extract(context.Background(), req)
	require.NoError(t, err)
	second, err := previews.Extract(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, []any{"One post", "One post", int32(1), true},
		[]any{first.Preview.Title, second.Preview.Title, hits.Load(), time.Since(started) < 2*time.Second})
}

// TestALockWhoseReplyCameLateIsStillTheTakersOwn covers a latency skew: instance A reaches Valkey through a
// link that holds back the reply to its SET NX until the client gives up on it, so the lock is taken while A hears
// nothing; instance B asks for the same URL once the lock is in Valkey. A once retried, found its own lock, and waited
// for nobody.
func TestALockWhoseReplyCameLateIsStillTheTakersOwn(t *testing.T) {
	valkey := valkeytest.Start(t)
	slowLink := valkeytest.StartProxy(t, valkey.URL)
	var hits atomic.Int32
	seenByB := make(chan struct{})
	origin := heldOrigin(t, &hits, func(r *http.Request) {
		select {
		case <-seenByB:
		case <-r.Context().Done():
		}
	})
	observers := []*recorder{{}, {}}
	a, b := instance(t, slowLink.URL, observers[0]), instance(t, valkey.URL, observers[1])
	req := card.Request{URL: origin.URL + "/post"}
	settings, err := redis.ParseURL(valkey.URL)
	require.NoError(t, err)
	control := redis.NewClient(settings)
	t.Cleanup(func() { require.NoError(t, control.Close()) })

	var wg sync.WaitGroup
	var titles [2]string
	var errs [2]error
	wg.Go(func() {
		result, err := a.Extract(context.Background(), req)
		titles[0], errs[0] = result.Preview.Title, err
	})
	// B asks once A's lock is in Valkey, and A's page is held until B has missed it in the store: B can only wait for A.
	require.Eventually(t, func() bool {
		return control.Exists(context.Background(), cache.LockKeyOf(req.URL)).Val() == 1
	}, 10*time.Second, 10*time.Millisecond, "A's lock never reached Valkey")
	wg.Go(func() {
		result, err := b.Extract(context.Background(), req)
		titles[1], errs[1] = result.Preview.Title, err
	})
	require.Eventually(t, func() bool { return observers[1].has("looked up miss") }, 10*time.Second, 10*time.Millisecond,
		"B never looked in the store")
	close(seenByB)
	wg.Wait()

	require.NoError(t, errors.Join(errs[:]...))
	// One SET NX: A made the preview under the lock it took, rather than ask again until its own lock ran out.
	assert.Equal(t, []any{int32(1), [2]string{"One post", "One post"}, int32(1), true},
		[]any{hits.Load(), titles, slowLink.LockAttempts.Load(), observers[1].has("locked waited")})
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

func TestTheFallbackIsEmptiedWhenTheSharedStoreAnswersAgain(t *testing.T) {
	valkey := valkeytest.Start(t)
	ctx := context.Background()
	var ahead atomic.Int64
	clock := func() time.Time { return time.Now().Add(time.Duration(ahead.Load())) }
	fallback := cache.NewMemory(1<<20, clock)
	shared, err := cache.NewRedis(valkey.URL, cache.RedisOptions{Fallback: fallback, Now: clock})
	require.NoError(t, err)
	settings, err := redis.ParseURL(valkey.URL)
	require.NoError(t, err)
	control := redis.NewClient(settings)
	t.Cleanup(func() { require.NoError(t, control.Close()) })
	// An outage: while Valkey is paused, a preview is kept in this instance's memory alone.
	require.NoError(t, control.Do(ctx, "CLIENT", "PAUSE", "800", "ALL").Err())
	require.NoError(t, shared.Set(ctx, "g", "kept-alone", []byte("old"), time.Hour))
	_, keptAlone, err := fallback.Get(ctx, "kept-alone")
	require.NoError(t, err)
	require.True(t, keptAlone)
	time.Sleep(900 * time.Millisecond)
	ahead.Add(int64(10 * time.Second))

	_, _, err = shared.Get(ctx, "anything")
	require.NoError(t, err)
	_, stillKept, err := fallback.Get(ctx, "kept-alone")

	require.NoError(t, err)
	assert.False(t, stillKept)
}
