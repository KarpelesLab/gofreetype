// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package shape

import (
	"github.com/KarpelesLab/gofreetype/gsub"
	"github.com/KarpelesLab/gofreetype/truetype"
)

// runGSUB applies the substitution features enabled by opts over the glyph
// run. Each enabled lookup is applied left-to-right across the buffer;
// substitutions can shrink (ligature), grow (multiple), or replace
// (single, chaining-context -> nested lookups) the buffer.
//
// Contextual lookups (Type 5 / 6) trigger nested lookup application: the
// matched rule produces SequenceLookupRecord actions, which run other
// lookups at specific positions within the match. We apply nested lookups
// that target Types 1, 2, 3, 4 directly; nested references to other
// contextual lookups are ignored to avoid unbounded recursion for now.
func runGSUB(f *truetype.Font, glyphs []Glyph, opts Options) []Glyph {
	t := gsubOf(f)
	if t == nil {
		return glyphs
	}
	for _, lookupIdx := range enabledLookupIndices(t.Table, opts, opts.Features) {
		glyphs = applyGSUBLookup(t, lookupIdx, glyphs)
	}
	return glyphs
}

// applyGSUBLookup walks the buffer and invokes the given lookup at every
// position. Substitutions that change buffer length are spliced in place.
func applyGSUBLookup(t *gsub.Table, lookupIdx uint16, glyphs []Glyph) []Glyph {
	if int(lookupIdx) >= len(t.Lookups) {
		return glyphs
	}
	typ := t.Lookups[lookupIdx].Type
	i := 0
	for i < len(glyphs) {
		advanced := false
		switch typ {
		case 1: // Single
			if out, ok := t.Single(lookupIdx, glyphs[i].GID); ok {
				glyphs[i].GID = out
			}
		case 2: // Multiple
			if out, ok := t.Multiple(lookupIdx, glyphs[i].GID); ok && len(out) > 0 {
				glyphs = spliceGlyphs(glyphs, i, 1, out)
				i += len(out)
				advanced = true
			}
		case 3: // Alternate — default to first alternate.
			if alts, ok := t.Alternates(lookupIdx, glyphs[i].GID); ok && len(alts) > 0 {
				glyphs[i].GID = alts[0]
			}
		case 4: // Ligature
			if i+1 < len(glyphs) {
				run := gidsOf(glyphs[i:])
				if lig, consumed, ok := t.Ligature(lookupIdx, run); ok && consumed >= 2 {
					// Merge consumed glyphs into one. The result keeps the
					// first glyph's cluster so callers can still recover the
					// source text region.
					glyphs = spliceGlyphs(glyphs, i, consumed, []uint16{lig})
					i++
					advanced = true
				}
			}
		case 5: // Context
			if acts, consumed, ok := t.MatchContext(lookupIdx, gidsOf(glyphs), i); ok {
				applyNestedGSUB(t, glyphs[i:i+consumed], acts)
				i += consumed
				advanced = true
			}
		case 6: // Chaining context
			if acts, consumed, ok := t.MatchChainingContext(lookupIdx, gidsOf(glyphs), i); ok {
				applyNestedGSUB(t, glyphs[i:i+consumed], acts)
				i += consumed
				advanced = true
			}
		case 8: // Reverse chaining single
			// Reverse chaining runs right-to-left; the outer loop is the
			// wrong direction. Skip here; a dedicated pass over the buffer
			// in reverse handles it. See runGSUBReverse below.
		}
		if !advanced {
			i++
		}
	}
	if typ == 8 {
		runGSUBReverse(t, lookupIdx, glyphs)
	}
	return glyphs
}

// runGSUBReverse applies a Type-8 reverse-chaining lookup across the buffer
// from end to start. Type 8 never changes the buffer length so we can mutate
// in place.
func runGSUBReverse(t *gsub.Table, lookupIdx uint16, glyphs []Glyph) {
	gids := gidsOf(glyphs)
	for i := len(glyphs) - 1; i >= 0; i-- {
		if out, ok := t.ReverseChainSingle(lookupIdx, gids, i); ok {
			glyphs[i].GID = out
			gids[i] = out
		}
	}
}

// applyNestedGSUB runs the contextual-rule SequenceLookupRecord actions
// over the matched glyph slice. We only handle nested references to non-
// contextual substitution types to avoid recursive expansion. A full
// shaper would support nested contextual lookups with a bounded depth.
func applyNestedGSUB(t *gsub.Table, window []Glyph, acts []SequenceLookupRecord) {
	for _, a := range acts {
		if int(a.SequenceIndex) >= len(window) {
			continue
		}
		idx := a.LookupListIndex
		if int(idx) >= len(t.Lookups) {
			continue
		}
		pos := int(a.SequenceIndex)
		switch t.Lookups[idx].Type {
		case 1:
			if out, ok := t.Single(idx, window[pos].GID); ok {
				window[pos].GID = out
			}
		case 3:
			if alts, ok := t.Alternates(idx, window[pos].GID); ok && len(alts) > 0 {
				window[pos].GID = alts[0]
			}
		}
	}
}

// spliceGlyphs replaces len `n` glyphs starting at `start` with the given
// replacement sequence. The returned buffer may share storage with the
// input but the caller should always reassign.
func spliceGlyphs(buf []Glyph, start, n int, replacement []uint16) []Glyph {
	if n == len(replacement) {
		for i, g := range replacement {
			buf[start+i].GID = g
		}
		return buf
	}
	tail := make([]Glyph, len(buf[start+n:]))
	copy(tail, buf[start+n:])
	out := buf[:start]
	srcCluster := buf[start].Cluster
	for _, g := range replacement {
		out = append(out, Glyph{GID: g, Cluster: srcCluster})
	}
	out = append(out, tail...)
	return out
}

// gidsOf is a small projection helper: a scratch slice of plain GIDs for
// passing to the gsub / gpos context matchers.
func gidsOf(glyphs []Glyph) []uint16 {
	out := make([]uint16, len(glyphs))
	for i, g := range glyphs {
		out[i] = g.GID
	}
	return out
}
