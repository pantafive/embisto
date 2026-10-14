package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// A broken Redis must cost a request little: one short try, then the caller falls back.
const (
	dialTimeout = 200 * time.Millisecond
	ioTimeout   = 200 * time.Millisecond
	dialRetry   = 10 * time.Millisecond
	// indexSuffix names the set of a group's keys; it shares the group's hash tag, and with it the group's slot.
	indexSuffix = ":index"
)

// keep sets the value and adds its key to the group's index, which lives as long as the longest entry in it.
var keep = redis.NewScript(`
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
redis.call('SADD', KEYS[2], KEYS[1])
if redis.call('PTTL', KEYS[2]) < tonumber(ARGV[2]) then
	redis.call('PEXPIRE', KEYS[2], ARGV[2])
end
return 1`)

// purge drops every key the group's index names, then the index, in one step: a variant written while a purge ran
// in several would survive it and lose its index. The keys share the index's hash tag, and with it its slot.
var purge = redis.NewScript(`
local dropped = 0
for _, key in ipairs(redis.call('SMEMBERS', KEYS[1])) do
	dropped = dropped + redis.call('DEL', key)
end
redis.call('DEL', KEYS[1])
return dropped`)

// release deletes a lock only while it is still the holder's: an expired holder must not free another's lock.
var release = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
	return redis.call('DEL', KEYS[1])
end
return 0`)

// redisStore is the store in Redis or anything that speaks its protocol (Valkey, KeyDB, Dragonfly), shared by every
// instance, with the lock that lets them make one preview between them; Redis wraps it with a fallback.
type redisStore struct {
	client redis.UniversalClient
}

// newRedisStore connects to a redis:// or rediss:// URL; it does not dial until first used, so a Redis that is down does not
// stop the caller from starting.
func newRedisStore(rawURL string) (*redisStore, error) {
	opts, err := redis.ParseURL(rawURL)
	if err != nil {
		// The URL may hold a password: the error names the scheme's rule, never the URL.
		return nil, errors.New("the cache URL is not a valid redis:// or rediss:// URL") //nolint:err113 // a setting's error, said once at startup
	}
	opts.DialTimeout, opts.ReadTimeout, opts.WriteTimeout, opts.PoolTimeout = dialTimeout, ioTimeout, ioTimeout, ioTimeout
	// No command retries: a retried SET NX whose first reply was only late finds the instance's own lock and reads it
	// as another's. -1 is none; zero would mean go-redis's three. One quick dial retry.
	opts.MaxRetries, opts.DialerRetries, opts.DialerRetryTimeout = -1, 1, dialRetry
	return &redisStore{client: redis.NewClient(opts)}, nil
}

func (c *redisStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	value, err := c.client.Get(ctx, key).Bytes()
	switch {
	case errors.Is(err, redis.Nil):
		return nil, false, nil
	case err != nil:
		return nil, false, fmt.Errorf("redis get: %w", err)
	default:
		return value, true, nil
	}
}

func (c *redisStore) Set(ctx context.Context, group, key string, value []byte, ttl time.Duration) error {
	if err := keep.Run(ctx, c.client, []string{key, group + indexSuffix}, value, ttl.Milliseconds()).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}

func (c *redisStore) Delete(ctx context.Context, key string) (int, error) {
	n, err := c.client.Del(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("redis delete: %w", err)
	}
	return int(n), nil
}

// DeleteGroup drops the keys the group's index names, then the index; the count is of keys that were still there.
func (c *redisStore) DeleteGroup(ctx context.Context, group string) (int, error) {
	dropped, err := purge.Run(ctx, c.client, []string{group + indexSuffix}).Int()
	if err != nil {
		return 0, fmt.Errorf("redis purge: %w", err)
	}
	return dropped, nil
}

// Lock takes the key's lock with a random token; release frees it only while the token is still there.
func (c *redisStore) Lock(ctx context.Context, key string, ttl time.Duration) (func(context.Context) error, bool, error) {
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return nil, false, fmt.Errorf("lock token: %w", err)
	}
	held := hex.EncodeToString(token)
	ok, err := c.client.SetNX(ctx, key, held, ttl).Result()
	if err != nil {
		// The SET may have reached Redis though its reply did not come back in time: the lock is then this caller's,
		// and nobody else will make the preview.
		if stored, getErr := c.client.Get(ctx, key).Result(); getErr != nil || stored != held {
			return nil, false, fmt.Errorf("redis lock: %w", err)
		}
		ok = true
	}
	if !ok {
		return nil, false, nil
	}
	return func(ctx context.Context) error {
		if err := release.Run(ctx, c.client, []string{key}, held).Err(); err != nil {
			return fmt.Errorf("redis release: %w", err)
		}
		return nil
	}, true, nil
}
