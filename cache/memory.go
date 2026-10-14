package cache

import (
	"container/list"
	"context"
	"sync"
	"time"
)

// entryOverhead is what an entry costs besides its key, group and value: the list element, the maps' slots and the
// entry itself, rounded up.
const entryOverhead = 256

// Memory is a least-recently-used store whose entries also expire, bounded by the bytes it holds rather than by how
// many entries: a page can make one card far larger than another. Entries belong to groups, which are dropped together.
type Memory struct {
	mu      sync.Mutex
	budget  int64
	used    int64
	now     func() time.Time
	entries map[string]*list.Element
	groups  map[string]map[string]*list.Element
	recency *list.List
}

type memoryEntry struct {
	group   string
	key     string
	value   []byte
	expires time.Time
}

func (e *memoryEntry) cost() int64 {
	return int64(len(e.group) + len(e.key) + len(e.value) + entryOverhead)
}

// NewMemory keeps entries in the process within budget bytes, timed by the clock; nil is the real one.
func NewMemory(budget int64, now func() time.Time) *Memory {
	if now == nil {
		now = time.Now
	}
	return &Memory{budget: budget, now: now, entries: map[string]*list.Element{}, groups: map[string]map[string]*list.Element{}, recency: list.New()}
}

// Get is the value under the key; false for none or an expired one.
func (m *Memory) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	element, ok := m.entries[key]
	if !ok {
		return nil, false, nil
	}
	found := entryOf(element)
	if !m.now().Before(found.expires) {
		m.remove(element)
		return nil, false, nil
	}
	m.recency.MoveToFront(element)
	return found.value, true, nil
}

// Set keeps the value in its group for ttl, evicting the least recently used entries until the budget holds. A value
// larger than the whole budget is not kept.
func (m *Memory) Set(_ context.Context, group, key string, value []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if element, ok := m.entries[key]; ok {
		m.remove(element)
	}
	kept := &memoryEntry{group: group, key: key, value: value, expires: m.now().Add(ttl)}
	if kept.cost() > m.budget {
		return nil
	}
	element := m.recency.PushFront(kept)
	m.entries[key] = element
	if m.groups[group] == nil {
		m.groups[group] = map[string]*list.Element{}
	}
	m.groups[group][key] = element
	m.used += kept.cost()
	for m.used > m.budget {
		m.remove(m.recency.Back())
	}
	return nil
}

// Delete drops the key; it says whether an unexpired entry was there.
func (m *Memory) Delete(_ context.Context, key string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	element, ok := m.entries[key]
	if !ok {
		return 0, nil
	}
	isLive := m.now().Before(entryOf(element).expires)
	m.remove(element)
	if isLive {
		return 1, nil
	}
	return 0, nil
}

// DeleteGroup drops every entry of the group; it says how many were unexpired.
func (m *Memory) DeleteGroup(_ context.Context, group string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dropped := 0
	for _, element := range m.groups[group] {
		if m.now().Before(entryOf(element).expires) {
			dropped++
		}
		m.remove(element)
	}
	return dropped, nil
}

// Clear drops every entry.
func (m *Memory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries, m.groups, m.used = map[string]*list.Element{}, map[string]map[string]*list.Element{}, 0
	m.recency.Init()
}

func (m *Memory) remove(element *list.Element) {
	found := entryOf(element)
	m.recency.Remove(element)
	delete(m.entries, found.key)
	delete(m.groups[found.group], found.key)
	if len(m.groups[found.group]) == 0 {
		delete(m.groups, found.group)
	}
	m.used -= found.cost()
}

func entryOf(element *list.Element) *memoryEntry {
	found, _ := element.Value.(*memoryEntry) // the list holds entries only
	return found
}
