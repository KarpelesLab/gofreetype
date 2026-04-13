// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package shape

import (
	"github.com/KarpelesLab/gofreetype/gdef"
	"github.com/KarpelesLab/gofreetype/gpos"
	"github.com/KarpelesLab/gofreetype/truetype"
)

// runGPOS applies the positioning features enabled by opts to the glyph
// buffer in-place. Positioning never changes the glyph count.
//
// For each enabled lookup:
//   - Type 1 / 2:   per-glyph / per-pair advance & offset adjustment.
//   - Type 3 (curs): accumulate a y-offset chain so the exit anchor of
//     glyph N aligns with the entry anchor of glyph N+1.
//   - Type 4 / 6:   mark-to-base / mark-to-mark attachment; position the
//     mark by combining base and mark anchors.
//   - Type 5:       mark-to-ligature, positioned on the component pointed
//     at by the mark's cluster relationship to the base ligature.
//   - Type 7 / 8:   context / chaining-context — nested lookup records
//     applied within the matched window.
func runGPOS(f *truetype.Font, glyphs []Glyph, opts Options) {
	t := gposOf(f)
	if t == nil {
		return
	}
	for _, lookupIdx := range enabledLookupIndices(t.Table, opts, opts.Features) {
		applyGPOSLookup(t, lookupIdx, glyphs, f.GDEF())
	}
}

func applyGPOSLookup(t *gpos.Table, lookupIdx uint16, glyphs []Glyph, gd *gdef.Table) {
	if int(lookupIdx) >= len(t.Lookups) {
		return
	}
	typ := t.Lookups[lookupIdx].Type
	switch typ {
	case 1:
		for i := range glyphs {
			if v, ok := t.Single(lookupIdx, glyphs[i].GID); ok {
				applyValueRecord(&glyphs[i], v)
			}
		}
	case 2:
		for i := 0; i+1 < len(glyphs); i++ {
			if v1, v2, ok := t.Pair(lookupIdx, glyphs[i].GID, glyphs[i+1].GID); ok {
				applyValueRecord(&glyphs[i], v1)
				applyValueRecord(&glyphs[i+1], v2)
			}
		}
	case 3:
		applyCursive(t, lookupIdx, glyphs)
	case 4:
		applyMarkToPrevious(t, lookupIdx, glyphs, gd, true)
	case 5:
		// Mark-to-ligature is structurally similar to mark-to-base; the
		// shaper needs cluster info to pick the component. A fuller
		// implementation lives above this layer; for now we just leave
		// the slot alone when component info is unavailable.
	case 6:
		applyMarkToPrevious(t, lookupIdx, glyphs, gd, false)
	case 7:
		applyGPOSContext(t, lookupIdx, glyphs, gd)
	case 8:
		applyGPOSChainingContext(t, lookupIdx, glyphs, gd)
	}
}

func applyValueRecord(g *Glyph, v gpos.ValueRecord) {
	g.XOffset += int32(v.XPlacement)
	g.YOffset += int32(v.YPlacement)
	g.XAdvance += int32(v.XAdvance)
	g.YAdvance += int32(v.YAdvance)
}

// applyCursive aligns consecutive glyphs via their entry/exit anchors.
// After this pass, the kth glyph's "virtual pen" sits at the anchor point
// the previous glyph's exit pointed at. We adjust YOffset to realize the
// vertical component; horizontal alignment subtly affects xAdvance.
func applyCursive(t *gpos.Table, lookupIdx uint16, glyphs []Glyph) {
	for i := 0; i+1 < len(glyphs); i++ {
		_, _, exit, hasExit, ok := t.CursiveAnchors(lookupIdx, glyphs[i].GID)
		if !ok || !hasExit {
			continue
		}
		entry, hasEntry, _, _, ok := t.CursiveAnchors(lookupIdx, glyphs[i+1].GID)
		if !ok || !hasEntry {
			continue
		}
		// Horizontal: the exit anchor's X must equal the current pen
		// position (after the current glyph's advance) plus the next
		// glyph's entry X. We adjust the current glyph's XAdvance so
		// that happens.
		glyphs[i].XAdvance = int32(exit.X) - int32(entry.X)
		// Vertical: add the difference so the anchors line up.
		glyphs[i+1].YOffset += int32(exit.Y) - int32(entry.Y)
	}
}

// applyMarkToPrevious scans the buffer for marks and attaches each one to
// the nearest preceding base (for Type 4) or mark (for Type 6). The
// "nearest preceding base" convention matches the default OpenType
// behavior for scripts that don't redefine it.
func applyMarkToPrevious(t *gpos.Table, lookupIdx uint16, glyphs []Glyph, gd *gdef.Table, matchBases bool) {
	for i := 1; i < len(glyphs); i++ {
		if gd != nil && glyphs[i].gdefClass != gdef.ClassMark {
			continue
		}
		// Find the target: nearest preceding glyph of the desired class.
		target := -1
		for j := i - 1; j >= 0; j-- {
			if gd == nil {
				target = j
				break
			}
			if matchBases {
				if glyphs[j].gdefClass == gdef.ClassBase ||
					glyphs[j].gdefClass == gdef.ClassLigature {
					target = j
					break
				}
			} else {
				if glyphs[j].gdefClass == gdef.ClassMark {
					target = j
					break
				}
			}
			// If we hit another mark while looking for a base, keep
			// scanning — marks stack.
		}
		if target < 0 {
			continue
		}
		var att gpos.MarkAttachment
		var ok bool
		if matchBases {
			att, ok = t.MarkToBase(lookupIdx, glyphs[i].GID, glyphs[target].GID)
		} else {
			att, ok = t.MarkToMark(lookupIdx, glyphs[i].GID, glyphs[target].GID)
		}
		if !ok {
			continue
		}
		// Anchor semantics: we want the mark's anchor to coincide with the
		// base's anchor in the shaped output. The mark's XOffset/YOffset
		// therefore become (baseAnchor - markAnchor), plus any advance
		// accumulated between the base and the mark in the run.
		glyphs[i].XOffset = int32(att.BaseAnchor.X) - int32(att.MarkAnchor.X)
		glyphs[i].YOffset = int32(att.BaseAnchor.Y) - int32(att.MarkAnchor.Y)
		// Accumulate advances of intervening glyphs so the mark sits above
		// the correct X when we draw.
		for j := target; j < i; j++ {
			glyphs[i].XOffset -= glyphs[j].XAdvance
		}
		// Marks don't advance the pen.
		glyphs[i].XAdvance = 0
	}
}

func applyGPOSContext(t *gpos.Table, lookupIdx uint16, glyphs []Glyph, gd *gdef.Table) {
	gids := gidsOf(glyphs)
	for i := 0; i < len(glyphs); i++ {
		acts, consumed, ok := t.MatchContext(lookupIdx, gids, i)
		if !ok {
			continue
		}
		applyNestedGPOS(t, glyphs[i:i+consumed], acts, gd)
		i += consumed - 1
	}
}

func applyGPOSChainingContext(t *gpos.Table, lookupIdx uint16, glyphs []Glyph, gd *gdef.Table) {
	gids := gidsOf(glyphs)
	for i := 0; i < len(glyphs); i++ {
		acts, consumed, ok := t.MatchChainingContext(lookupIdx, gids, i)
		if !ok {
			continue
		}
		applyNestedGPOS(t, glyphs[i:i+consumed], acts, gd)
		i += consumed - 1
	}
}

func applyNestedGPOS(t *gpos.Table, window []Glyph, acts []SequenceLookupRecord, gd *gdef.Table) {
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
			if v, ok := t.Single(idx, window[pos].GID); ok {
				applyValueRecord(&window[pos], v)
			}
		case 2:
			if pos+1 < len(window) {
				if v1, v2, ok := t.Pair(idx, window[pos].GID, window[pos+1].GID); ok {
					applyValueRecord(&window[pos], v1)
					applyValueRecord(&window[pos+1], v2)
				}
			}
		}
	}
}
