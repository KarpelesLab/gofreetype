// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gsub

import "github.com/KarpelesLab/gofreetype/layout"

// GSUB Type 1: single substitution. Replace one input glyph with one output.
//
// Format 1: uint16 format (1), Offset16 coverageOffset, int16 deltaGlyphID
//   — replacement = (input + deltaGlyphID) mod 65536
//
// Format 2: uint16 format (2), Offset16 coverageOffset, uint16 glyphCount,
//   uint16 substituteGlyphIDs[glyphCount]
//   — replacement = substituteGlyphIDs[coverageIndex]

// Single returns the replacement glyph id for g via the named Type-1 lookup.
// ok is false when the lookup is not Type 1 or g is not covered.
func (t *Table) Single(lookupIndex uint16, g uint16) (uint16, bool) {
	if int(lookupIndex) >= len(t.Lookups) {
		return 0, false
	}
	lk := t.Lookups[lookupIndex]
	actualType, subtables := resolveExtension(lk.Type, lk.SubtableData)
	if actualType != 1 {
		return 0, false
	}
	for _, sub := range subtables {
		sub := sub
		out, found, err := lookupSingle(sub, g)
		if err != nil || !found {
			continue
		}
		return out, true
	}
	return 0, false
}

func lookupSingle(sub []byte, g uint16) (uint16, bool, error) {
	if len(sub) < 6 {
		return 0, false, FormatError("single subtable header truncated")
	}
	format := u16(sub, 0)
	covOff := int(u16(sub, 2))
	cov, err := layout.ParseCoverage(sub, covOff)
	if err != nil {
		return 0, false, err
	}
	idx := cov.Index(g)
	if idx < 0 {
		return 0, false, nil
	}
	switch format {
	case 1:
		delta := int16(u16(sub, 4))
		return uint16(int(g) + int(delta)), true, nil
	case 2:
		if len(sub) < 8 {
			return 0, false, FormatError("single format 2 truncated")
		}
		count := int(u16(sub, 4))
		if idx >= count {
			return 0, false, nil
		}
		if 6+2*count > len(sub) {
			return 0, false, FormatError("single format 2 body truncated")
		}
		return u16(sub, 6+2*idx), true, nil
	}
	return 0, false, UnsupportedError("single format " + intToStr(int(format)))
}
