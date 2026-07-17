package lrupq

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestPutGet(t *testing.T) {
	c := New[string, int](3)
	c.Put("a", 1)
	c.Put("b", 2)

	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("Get(a) = %v, %v; want 1, true", v, ok)
	}
	if _, ok := c.Get("missing"); ok {
		t.Fatal("Get(missing) reported a hit")
	}
	if c.Len() != 2 {
		t.Fatalf("Len() = %d; want 2", c.Len())
	}
}

func TestEvictsLeastRecentlyUsed(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)

	// Touch "a" so "b" becomes the LRU entry.
	c.Get("a")

	k, v, evicted := c.Put("c", 3)
	if !evicted || k != "b" || v != 2 {
		t.Fatalf("Put(c) evicted %q=%d, %v; want b=2, true", k, v, evicted)
	}
	if _, ok := c.Peek("b"); ok {
		t.Fatal("evicted key b still present")
	}
	for _, key := range []string{"a", "c"} {
		if _, ok := c.Peek(key); !ok {
			t.Fatalf("key %q missing after eviction", key)
		}
	}
}

func TestPutUpdatesExistingKey(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)

	// Updating "a" must refresh both its value and its recency,
	// and must not evict anything.
	if _, _, evicted := c.Put("a", 10); evicted {
		t.Fatal("updating an existing key evicted an entry")
	}
	if v, _ := c.Peek("a"); v != 10 {
		t.Fatalf("Peek(a) = %d; want 10", v)
	}

	// "b" is now the LRU entry.
	if k, _, _ := c.Oldest(); k != "b" {
		t.Fatalf("Oldest() = %q; want b", k)
	}
}

func TestPeekDoesNotTouch(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)

	c.Peek("a") // must NOT refresh "a"

	if k, _, _ := c.Oldest(); k != "a" {
		t.Fatalf("Oldest() = %q after Peek; want a", k)
	}
}

func TestRemove(t *testing.T) {
	c := New[string, int](3)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)

	if !c.Remove("b") {
		t.Fatal("Remove(b) = false; want true")
	}
	if c.Remove("b") {
		t.Fatal("second Remove(b) = true; want false")
	}
	if c.Len() != 2 {
		t.Fatalf("Len() = %d; want 2", c.Len())
	}

	// Heap must still be consistent: filling up evicts "a", the oldest survivor.
	c.Put("d", 4)
	if k, _, evicted := c.Put("e", 5); !evicted || k != "a" {
		t.Fatalf("Put(e) evicted %q, %v; want a, true", k, evicted)
	}
}

func TestCapacityOne(t *testing.T) {
	c := New[int, string](1)
	c.Put(1, "one")
	if k, v, evicted := c.Put(2, "two"); !evicted || k != 1 || v != "one" {
		t.Fatalf("Put(2) evicted %d=%q, %v; want 1=one, true", k, v, evicted)
	}
	if v, ok := c.Get(2); !ok || v != "two" {
		t.Fatalf("Get(2) = %q, %v; want two, true", v, ok)
	}
}

func TestNewPanicsOnBadCapacity(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("New(0) did not panic")
		}
	}()
	New[int, int](0)
}

// TestMatchesReferenceModel drives the cache with random operations and
// checks every result against a brute-force LRU model.
func TestMatchesReferenceModel(t *testing.T) {
	const capacity, ops, keyspace = 8, 5000, 20
	rng := rand.New(rand.NewSource(1))

	c := New[int, int](capacity)
	values := map[int]int{} // reference contents
	order := []int{}        // reference recency, oldest first

	refTouch := func(k int) {
		for i, key := range order {
			if key == k {
				order = append(order[:i], order[i+1:]...)
				break
			}
		}
		order = append(order, k)
	}

	for i := 0; i < ops; i++ {
		k := rng.Intn(keyspace)
		switch rng.Intn(3) {
		case 0: // Put
			if _, exists := values[k]; !exists && len(values) == capacity {
				victim := order[0]
				order = order[1:]
				delete(values, victim)
				if ek, _, evicted := c.Put(k, i); !evicted || ek != victim {
					t.Fatalf("op %d: Put(%d) evicted %d, %v; want %d, true", i, k, ek, evicted, victim)
				}
			} else if _, _, evicted := c.Put(k, i); evicted {
				t.Fatalf("op %d: Put(%d) evicted unexpectedly", i, k)
			}
			values[k] = i
			refTouch(k)
		case 1: // Get
			want, wantOK := values[k]
			got, ok := c.Get(k)
			if ok != wantOK || (ok && got != want) {
				t.Fatalf("op %d: Get(%d) = %d, %v; want %d, %v", i, k, got, ok, want, wantOK)
			}
			if wantOK {
				refTouch(k)
			}
		case 2: // Remove
			_, wantOK := values[k]
			if got := c.Remove(k); got != wantOK {
				t.Fatalf("op %d: Remove(%d) = %v; want %v", i, k, got, wantOK)
			}
			if wantOK {
				delete(values, k)
				for j, key := range order {
					if key == k {
						order = append(order[:j], order[j+1:]...)
						break
					}
				}
			}
		}
		if c.Len() != len(values) {
			t.Fatalf("op %d: Len() = %d; want %d", i, c.Len(), len(values))
		}
	}
}

func ExampleCache() {
	c := New[string, string](2)
	c.Put("k1", "v1")
	c.Put("k2", "v2")

	c.Get("k1")       // k1 is now most recently used
	c.Put("k3", "v3") // full: evicts k2, the LRU entry

	_, ok := c.Peek("k2")
	fmt.Println("k2 present:", ok)

	key, _, _ := c.Oldest()
	fmt.Println("next to evict:", key)
	// Output:
	// k2 present: false
	// next to evict: k1
}
