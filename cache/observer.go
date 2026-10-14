package cache

// Observer hears what the layer does, for metrics.
type Observer interface {
	// LookedUp is a lookup: hit, miss, or stale for an entry found past its expiry.
	LookedUp(result string)
	// Coalesced is a request that waited for a preview another made: in the process (local) or on another instance
	// (shared).
	Coalesced(scope string)
	// Locked is how a shared lock went: acquired, waited for another's preview, timeout, or error.
	Locked(outcome string)
	// TTLChosen is where a kept entry's TTL came from: origin, min, max, default, or failure.
	TTLChosen(source string)
}

// RedisObserver hears whether the shared store in Redis answers.
type RedisObserver interface {
	// RedisUp is the shared store starting or stopping to answer.
	RedisUp(isUp bool)
	// RedisFailed is an operation it failed, which the fallback served instead.
	RedisFailed()
}

// Lookup results, TTL sources, lock outcomes and coalescing scopes, as the observer hears them.
const (
	LookupHit   = "hit"
	LookupMiss  = "miss"
	LookupStale = "stale"

	TTLOrigin  = "origin"
	TTLMin     = "min"
	TTLMax     = "max"
	TTLDefault = "default"
	TTLFailure = "failure"

	LockAcquired = "acquired"
	LockWaited   = "waited"
	LockTimeout  = "timeout"
	LockError    = "error"

	CoalescedLocal  = "local"
	CoalescedShared = "shared"
)

type unobserved struct{}

func (unobserved) LookedUp(string)  {}
func (unobserved) Coalesced(string) {}
func (unobserved) Locked(string)    {}
func (unobserved) TTLChosen(string) {}
