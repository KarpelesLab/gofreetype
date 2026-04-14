// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import "testing"

func TestGlyphLRUEvictsLeastRecentlyUsed(t *testing.T) {
	c := newGlyphLRU(3)

	k1 := glyphCacheKey{index: 1, fx: 0, fy: 0}
	k2 := glyphCacheKey{index: 2, fx: 0, fy: 0}
	k3 := glyphCacheKey{index: 3, fx: 0, fy: 0}
	k4 := glyphCacheKey{index: 4, fx: 0, fy: 0}

	// Fill the cache.
	c.store(c.allocate(k1), glyphCacheVal{advanceWidth: 1})
	c.store(c.allocate(k2), glyphCacheVal{advanceWidth: 2})
	c.store(c.allocate(k3), glyphCacheVal{advanceWidth: 3})

	// Touch k1 so it becomes MRU, making k2 the new LRU.
	if v, _, ok := c.get(k1); !ok || v.advanceWidth != 1 {
		t.Fatalf("get(k1): got (%v, %v), want (1, true)", v, ok)
	}

	// Insert k4 — k2 should be evicted since it's now LRU.
	c.store(c.allocate(k4), glyphCacheVal{advanceWidth: 4})

	if _, _, ok := c.get(k2); ok {
		t.Error("k2 should have been evicted")
	}
	if v, _, ok := c.get(k1); !ok || v.advanceWidth != 1 {
		t.Error("k1 should still be present")
	}
	if v, _, ok := c.get(k3); !ok || v.advanceWidth != 3 {
		t.Error("k3 should still be present")
	}
	if v, _, ok := c.get(k4); !ok || v.advanceWidth != 4 {
		t.Error("k4 should be present after insert")
	}
}

func TestGlyphLRURefillAfterInvalidate(t *testing.T) {
	c := newGlyphLRU(2)
	c.store(c.allocate(glyphCacheKey{index: 1}), glyphCacheVal{advanceWidth: 10})
	c.store(c.allocate(glyphCacheKey{index: 2}), glyphCacheVal{advanceWidth: 20})

	c.invalidate()

	if _, _, ok := c.get(glyphCacheKey{index: 1}); ok {
		t.Error("cache should be empty after invalidate")
	}
	if _, _, ok := c.get(glyphCacheKey{index: 2}); ok {
		t.Error("cache should be empty after invalidate")
	}
	// Can re-fill after invalidate.
	c.store(c.allocate(glyphCacheKey{index: 99}), glyphCacheVal{advanceWidth: 99})
	if v, _, ok := c.get(glyphCacheKey{index: 99}); !ok || v.advanceWidth != 99 {
		t.Error("refill after invalidate failed")
	}
}

// TestGlyphLRUGetMissOnCleanCache confirms get on an empty cache is a
// miss rather than a panic.
func TestGlyphLRUGetMissOnCleanCache(t *testing.T) {
	c := newGlyphLRU(4)
	if _, _, ok := c.get(glyphCacheKey{index: 42}); ok {
		t.Error("get on empty cache returned ok=true")
	}
}

// TestGlyphLRUPromoteHeadNoop verifies promoting a slot that's already
// at the head is a no-op (no list corruption).
func TestGlyphLRUPromoteHeadNoop(t *testing.T) {
	c := newGlyphLRU(3)
	k := glyphCacheKey{index: 1}
	c.store(c.allocate(k), glyphCacheVal{advanceWidth: 7})
	// Repeated gets should keep the entry valid.
	for i := 0; i < 10; i++ {
		if _, _, ok := c.get(k); !ok {
			t.Fatalf("iteration %d: get dropped the entry", i)
		}
	}
}
