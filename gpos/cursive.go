// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gpos

import "github.com/KarpelesLab/gofreetype/layout"

// Cursive attachment (GPOS Type 3) connects consecutive glyphs by
// aligning one glyph's exit anchor with the next glyph's entry anchor.
// It is the positioning half of Arabic cursive joining.
//
// The on-disk subtable:
//
//	uint16 format (1)
//	Offset16 coverageOffset
//	uint16 entryExitCount
//	EntryExitRecord entryExitRecords[entryExitCount]
//
// Each EntryExitRecord is:
//
//	Offset16 entryAnchorOffset (0 = absent)
//	Offset16 exitAnchorOffset  (0 = absent)

// CursiveAnchors returns the (entry, hasEntry, exit, hasExit) anchors for
// glyph g within the given Type-3 Lookup, or zeroed anchors + false if the
// lookup is not cursive or g is not covered.
func (t *Table) CursiveAnchors(lookupIndex uint16, g uint16) (entry Anchor, hasEntry bool, exit Anchor, hasExit bool, ok bool) {
	if int(lookupIndex) >= len(t.Lookups) {
		return Anchor{}, false, Anchor{}, false, false
	}
	lk := t.Lookups[lookupIndex]
	actualType, subtables := resolveExtension(lk.Type, lk.SubtableData)
	if actualType != 3 {
		return Anchor{}, false, Anchor{}, false, false
	}
	for _, sub := range subtables {
		e, he, x, hx, found, err := lookupCursive(sub, g)
		if err != nil || !found {
			continue
		}
		return e, he, x, hx, true
	}
	return Anchor{}, false, Anchor{}, false, false
}

func lookupCursive(sub []byte, g uint16) (entry Anchor, hasEntry bool, exit Anchor, hasExit bool, found bool, err error) {
	if len(sub) < 6 {
		return Anchor{}, false, Anchor{}, false, false, FormatError("cursive subtable header truncated")
	}
	format := u16(sub, 0)
	if format != 1 {
		return Anchor{}, false, Anchor{}, false, false, UnsupportedError("cursive format " + intCount(int(format)))
	}
	covOff := int(u16(sub, 2))
	count := int(u16(sub, 4))
	if 6+4*count > len(sub) {
		return Anchor{}, false, Anchor{}, false, false, FormatError("cursive entryExit records truncated")
	}
	cov, err := layout.ParseCoverage(sub, covOff)
	if err != nil {
		return Anchor{}, false, Anchor{}, false, false, err
	}
	idx := cov.Index(g)
	if idx < 0 || idx >= count {
		return Anchor{}, false, Anchor{}, false, false, nil
	}
	rec := 6 + 4*idx
	entryOff := int(u16(sub, rec))
	exitOff := int(u16(sub, rec+2))
	if entryOff != 0 {
		entry, hasEntry, err = parseAnchor(sub, entryOff)
		if err != nil {
			return Anchor{}, false, Anchor{}, false, false, err
		}
	}
	if exitOff != 0 {
		exit, hasExit, err = parseAnchor(sub, exitOff)
		if err != nil {
			return Anchor{}, false, Anchor{}, false, false, err
		}
	}
	return entry, hasEntry, exit, hasExit, true, nil
}
