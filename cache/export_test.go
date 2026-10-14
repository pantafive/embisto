package cache

import "github.com/pantafive/embisto/card"

// LockKeyOf is the shared lock's key for a URL's preview, for a test that plays a crashed holder.
func LockKeyOf(raw string) string {
	key, err := keyOf(card.Request{URL: raw})
	if err != nil {
		panic(err)
	}
	return key.name + ":lock"
}

// CostOf is what an entry of a value of the size costs the memory's budget.
func CostOf(group, key string, size int) int64 {
	return (&memoryEntry{group: group, key: key, value: make([]byte, size)}).cost()
}
