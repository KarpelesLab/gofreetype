// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gsub

import "github.com/KarpelesLab/gofreetype/layout"

// GSUB Type 4: ligature substitution. Replace a run of consecutive glyphs
// with one ligature glyph. The classic example is "f i" → "ﬁ".
//
//	uint16 format (1)
//	Offset16 coverageOffset
//	uint16 ligatureSetCount
//	Offset16 ligatureSetOffsets[ligatureSetCount]
//
// LigatureSet (one per first glyph in coverage):
//
//	uint16 ligatureCount
//	Offset16 ligatureOffsets[ligatureCount]
//
// Ligature:
//
//	uint16 ligatureGlyph
//	uint16 componentCount    // number of glyphs in the ligature, including the first
//	uint16 componentGlyphIDs[componentCount - 1]  // tail components

// Ligature tries each ligature rooted at input[0] and returns the first
// whose full component list matches a prefix of input.
//
//	ligGID:    the glyph id of the resulting ligature.
//	consumed:  number of input glyphs consumed (>= 2 on a match).
//	ok:        true iff a match was found.
func (t *Table) Ligature(lookupIndex uint16, input []uint16) (ligGID uint16, consumed int, ok bool) {
	if len(input) < 1 || int(lookupIndex) >= len(t.Lookups) {
		return 0, 0, false
	}
	lk := t.Lookups[lookupIndex]
	actualType, subtables := resolveExtension(lk.Type, lk.SubtableData)
	if actualType != 4 {
		return 0, 0, false
	}
	for _, sub := range subtables {
		g, n, found, err := lookupLigature(sub, input)
		if err != nil || !found {
			continue
		}
		return g, n, true
	}
	return 0, 0, false
}

func lookupLigature(sub []byte, input []uint16) (uint16, int, bool, error) {
	if len(sub) < 6 {
		return 0, 0, false, FormatError("ligature subtable header truncated")
	}
	format := u16(sub, 0)
	if format != 1 {
		return 0, 0, false, UnsupportedError("ligature format " + intToStr(int(format)))
	}
	covOff := int(u16(sub, 2))
	count := int(u16(sub, 4))
	if 6+2*count > len(sub) {
		return 0, 0, false, FormatError("LigatureSet offsets truncated")
	}
	cov, err := layout.ParseCoverage(sub, covOff)
	if err != nil {
		return 0, 0, false, err
	}
	idx := cov.Index(input[0])
	if idx < 0 || idx >= count {
		return 0, 0, false, nil
	}
	setOff := int(u16(sub, 6+2*idx))
	if setOff+2 > len(sub) {
		return 0, 0, false, FormatError("LigatureSet header out of bounds")
	}
	ligCount := int(u16(sub, setOff))
	if setOff+2+2*ligCount > len(sub) {
		return 0, 0, false, FormatError("LigatureSet body out of bounds")
	}
	for i := 0; i < ligCount; i++ {
		ligOff := setOff + int(u16(sub, setOff+2+2*i))
		if ligOff+4 > len(sub) {
			return 0, 0, false, FormatError("Ligature header out of bounds")
		}
		ligGlyph := u16(sub, ligOff)
		compCount := int(u16(sub, ligOff+2))
		if compCount < 1 {
			continue
		}
		if len(input) < compCount {
			continue
		}
		if ligOff+4+2*(compCount-1) > len(sub) {
			return 0, 0, false, FormatError("Ligature components out of bounds")
		}
		match := true
		for j := 1; j < compCount; j++ {
			if input[j] != u16(sub, ligOff+4+2*(j-1)) {
				match = false
				break
			}
		}
		if match {
			return ligGlyph, compCount, true, nil
		}
	}
	return 0, 0, false, nil
}
