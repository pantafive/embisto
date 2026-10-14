// Package cache keeps previews: a layer around any card.Extractor, and the stores it keeps them in, in the process
// or shared between instances through Redis.
package cache

import (
	"context"
	"time"
)

// Store keeps bytes under a key for a while, in a group the key belongs to. Memory and Redis are the two; any other
// that keeps the contract fits.
type Store interface {
	// Get is the value under the key; false for none.
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, group, key string, value []byte, ttl time.Duration) error
	// Delete drops the key and says how many live entries it dropped: one or none.
	Delete(ctx context.Context, key string) (int, error)
	// DeleteGroup drops every key of the group and says how many live entries it dropped.
	DeleteGroup(ctx context.Context, group string) (int, error)
}

// Locker lets instances that share a store agree which one makes a preview, so that many identical requests across
// instances make one. A store in one process needs none.
type Locker interface {
	// Lock takes the key's lock for ttl; false when another holds it. Release gives it back only while still held;
	// a release that fails leaves the lock to expire.
	Lock(ctx context.Context, key string, ttl time.Duration) (release func(context.Context) error, ok bool, err error)
}
