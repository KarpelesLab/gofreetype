// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

// The glyph cache maps a (glyph, fx, fy) triple to a previously-
// rasterized mask slot in the face's shared masks image. The original
// implementation was a flat array indexed by a hash of the triple,
// which produces random-replacement eviction under collision — fine
// for workloads with few unique glyphs but thrashy for anything
// touching more unique glyphs than the cache holds (CJK text, emoji-
// heavy strings, large-font rendering).
//
// This file replaces that strategy with a classic LRU: slots are fixed
// in memory (their Y offset in the masks image never changes), but
// which slot holds which glyph is determined by a doubly-linked list
// tracking recent use. On a cache miss, the least-recently-used slot
// is overwritten.

// glyphLRU is a fixed-capacity LRU cache keyed by glyphCacheKey.
// Each slot corresponds to a single tile in the face's masks image.
type glyphLRU struct {
	keys  []glyphCacheKey // key at each slot
	vals  []glyphCacheVal // value at each slot
	next  []int           // doubly-linked list of slots; -1 is end
	prev  []int
	byKey map[glyphCacheKey]int // key -> slot index
	head  int                   // most recently used slot
	tail  int                   // least recently used slot
}

// newGlyphLRU constructs a fresh LRU cache of the given capacity. All
// slots start "invalid" (their glyphCacheKey has fy == 0xff, which the
// rasterizer never sets) in the LRU list order 0..cap-1 so the first
// inserts fill empty slots in order.
func newGlyphLRU(cap int) *glyphLRU {
	c := &glyphLRU{
		keys:  make([]glyphCacheKey, cap),
		vals:  make([]glyphCacheVal, cap),
		next:  make([]int, cap),
		prev:  make([]int, cap),
		byKey: make(map[glyphCacheKey]int, cap),
	}
	for i := range c.keys {
		c.keys[i].fy = 0xff
		c.prev[i] = i - 1
		c.next[i] = i + 1
	}
	if cap > 0 {
		c.next[cap-1] = -1
		c.head = 0
		c.tail = cap - 1
	} else {
		c.head, c.tail = -1, -1
	}
	return c
}

// get returns the value at the slot holding key (and moves that slot to
// the MRU end), or (0, -1, false) if the key is absent.
func (c *glyphLRU) get(key glyphCacheKey) (glyphCacheVal, int, bool) {
	slot, ok := c.byKey[key]
	if !ok {
		return glyphCacheVal{}, -1, false
	}
	c.promote(slot)
	return c.vals[slot], slot, true
}

// allocate returns the slot the caller should use to store a new entry
// with the given key, evicting the LRU slot if the cache is at
// capacity. The returned slot is already promoted to MRU; the caller
// fills c.keys[slot]/c.vals[slot] after rasterizing.
func (c *glyphLRU) allocate(key glyphCacheKey) int {
	// Reuse the current tail.
	slot := c.tail
	if slot < 0 {
		// Capacity 0 — shouldn't happen in practice.
		return -1
	}
	// Evict the old key at that slot if it was valid.
	oldKey := c.keys[slot]
	if oldKey.fy != 0xff {
		delete(c.byKey, oldKey)
	}
	c.keys[slot] = key
	c.byKey[key] = slot
	c.promote(slot)
	return slot
}

// promote moves the named slot to the head of the LRU list.
func (c *glyphLRU) promote(slot int) {
	if slot == c.head {
		return
	}
	// Unlink from current position.
	p := c.prev[slot]
	n := c.next[slot]
	if p >= 0 {
		c.next[p] = n
	}
	if n >= 0 {
		c.prev[n] = p
	}
	if slot == c.tail {
		c.tail = p
	}
	// Insert at head.
	c.prev[slot] = -1
	c.next[slot] = c.head
	if c.head >= 0 {
		c.prev[c.head] = slot
	}
	c.head = slot
	if c.tail < 0 {
		c.tail = slot
	}
}

// store puts val into the slot previously handed out by allocate.
func (c *glyphLRU) store(slot int, val glyphCacheVal) {
	c.vals[slot] = val
}

// invalidate clears every slot. Used when a state change (e.g. hinting
// toggle) means all cached rasterizations are stale.
func (c *glyphLRU) invalidate() {
	for k := range c.byKey {
		delete(c.byKey, k)
	}
	for i := range c.keys {
		c.keys[i].fy = 0xff
		c.prev[i] = i - 1
		c.next[i] = i + 1
	}
	if len(c.keys) > 0 {
		c.next[len(c.keys)-1] = -1
		c.head = 0
		c.tail = len(c.keys) - 1
	}
}
