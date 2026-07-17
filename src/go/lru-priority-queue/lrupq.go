// Package lrupq implements an LRU (least recently used) cache whose
// recency order is tracked with a binary min-heap (priority queue)
// instead of the classic doubly-linked list.
//
// Every access stamps the entry with a monotonically increasing tick and
// restores the heap invariant with heap.Fix, so the heap root is always
// the least recently used entry. Get and Put run in O(log n); the classic
// list-based LRU runs in O(1), but the heap layout generalizes naturally
// to richer eviction policies (TTL, weighted priorities, LFU hybrids)
// where "evict the minimum by some score" is exactly what a heap gives you.
package lrupq

import "container/heap"

// entry is a single cache slot. index is its current position in the
// heap slice and is kept up to date by the heap.Interface methods so
// heap.Fix and heap.Remove can address it directly.
type entry[K comparable, V any] struct {
	key      K
	value    V
	lastUsed uint64 // monotonic tick; smaller = less recently used
	index    int
}

// entryHeap is a min-heap of entries ordered by lastUsed.
type entryHeap[K comparable, V any] []*entry[K, V]

func (h entryHeap[K, V]) Len() int           { return len(h) }
func (h entryHeap[K, V]) Less(i, j int) bool { return h[i].lastUsed < h[j].lastUsed }
func (h entryHeap[K, V]) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *entryHeap[K, V]) Push(x any) {
	e := x.(*entry[K, V])
	e.index = len(*h)
	*h = append(*h, e)
}

func (h *entryHeap[K, V]) Pop() any {
	old := *h
	n := len(old)
	e := old[n-1]
	old[n-1] = nil // release the reference so the entry can be collected
	*h = old[:n-1]
	return e
}

// Cache is an LRU cache backed by a priority queue. The zero value is not
// usable; construct with New. Cache is not safe for concurrent use.
type Cache[K comparable, V any] struct {
	capacity int
	tick     uint64
	items    map[K]*entry[K, V]
	pq       entryHeap[K, V]
}

// New returns an empty cache that holds at most capacity entries.
// It panics if capacity is not positive.
func New[K comparable, V any](capacity int) *Cache[K, V] {
	if capacity <= 0 {
		panic("lrupq: capacity must be positive")
	}
	return &Cache[K, V]{
		capacity: capacity,
		items:    make(map[K]*entry[K, V], capacity),
		pq:       make(entryHeap[K, V], 0, capacity),
	}
}

// touch marks e as the most recently used entry and re-sifts it.
func (c *Cache[K, V]) touch(e *entry[K, V]) {
	c.tick++
	e.lastUsed = c.tick
	heap.Fix(&c.pq, e.index)
}

// Get returns the value stored under key and marks it most recently used.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	if e, ok := c.items[key]; ok {
		c.touch(e)
		return e.value, true
	}
	var zero V
	return zero, false
}

// Peek returns the value stored under key without updating its recency.
func (c *Cache[K, V]) Peek(key K) (V, bool) {
	if e, ok := c.items[key]; ok {
		return e.value, true
	}
	var zero V
	return zero, false
}

// Put stores value under key, marking it most recently used. If the key is
// new and the cache is full, the least recently used entry is evicted; the
// evicted key/value pair is returned with evicted=true.
func (c *Cache[K, V]) Put(key K, value V) (evictedKey K, evictedValue V, evicted bool) {
	if e, ok := c.items[key]; ok {
		e.value = value
		c.touch(e)
		return
	}
	if len(c.items) >= c.capacity {
		victim := heap.Pop(&c.pq).(*entry[K, V])
		delete(c.items, victim.key)
		evictedKey, evictedValue, evicted = victim.key, victim.value, true
	}
	c.tick++
	e := &entry[K, V]{key: key, value: value, lastUsed: c.tick}
	heap.Push(&c.pq, e)
	c.items[key] = e
	return
}

// Remove deletes key from the cache and reports whether it was present.
func (c *Cache[K, V]) Remove(key K) bool {
	e, ok := c.items[key]
	if !ok {
		return false
	}
	heap.Remove(&c.pq, e.index)
	delete(c.items, key)
	return true
}

// Oldest returns the current eviction candidate — the least recently used
// key/value pair — without removing or touching it.
func (c *Cache[K, V]) Oldest() (key K, value V, ok bool) {
	if len(c.pq) == 0 {
		return
	}
	e := c.pq[0]
	return e.key, e.value, true
}

// Len returns the number of entries currently in the cache.
func (c *Cache[K, V]) Len() int { return len(c.items) }

// Cap returns the maximum number of entries the cache can hold.
func (c *Cache[K, V]) Cap() int { return c.capacity }
