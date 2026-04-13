// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gsub

import "github.com/KarpelesLab/gofreetype/layout"

// GSUB Type 8: Reverse Chaining Contextual Single Substitution. Applied
// right-to-left, replaces exactly one glyph (the input) when it is both
// in the coverage AND its backtrack and lookahead context match. The
// replacement is named directly in the subtable (no nested lookups).
//
// Because reverse chaining is applied right-to-left, a shaper driver must
// walk the glyph run from end to start when applying Type 8 lookups.
//
//	uint16 format (1)
//	Offset16 coverageOffset
//	uint16 backtrackGlyphCount
//	Offset16 backtrackCoverageOffsets[backtrackGlyphCount]
//	uint16 lookaheadGlyphCount
//	Offset16 lookaheadCoverageOffsets[lookaheadGlyphCount]
//	uint16 glyphCount
//	uint16 substituteGlyphIDs[glyphCount]

// ReverseChainSingle tries to apply a Type-8 reverse-chaining lookup at
// position `at` in the glyph run. If the lookup matches, it returns the
// replacement glyph id; otherwise ok is false.
func (t *Table) ReverseChainSingle(lookupIndex uint16, glyphs []uint16, at int) (uint16, bool) {
	if int(lookupIndex) >= len(t.Lookups) {
		return 0, false
	}
	lk := t.Lookups[lookupIndex]
	actualType, subtables := resolveExtension(lk.Type, lk.SubtableData)
	if actualType != 8 {
		return 0, false
	}
	for _, sub := range subtables {
		out, found, err := lookupReverseChainSingle(sub, glyphs, at)
		if err != nil || !found {
			continue
		}
		return out, true
	}
	return 0, false
}

func lookupReverseChainSingle(sub []byte, glyphs []uint16, at int) (uint16, bool, error) {
	if len(sub) < 6 {
		return 0, false, FormatError("reverse-chain subtable header truncated")
	}
	format := u16(sub, 0)
	if format != 1 {
		return 0, false, UnsupportedError("reverse-chain format " + intToStr(int(format)))
	}
	if at < 0 || at >= len(glyphs) {
		return 0, false, nil
	}
	covOff := int(u16(sub, 2))
	cov, err := layout.ParseCoverage(sub, covOff)
	if err != nil {
		return 0, false, err
	}
	idx := cov.Index(glyphs[at])
	if idx < 0 {
		return 0, false, nil
	}

	p := 4
	if p+2 > len(sub) {
		return 0, false, FormatError("reverse-chain backtrack count missing")
	}
	nBack := int(u16(sub, p))
	p += 2
	if p+2*nBack > len(sub) {
		return 0, false, FormatError("reverse-chain backtrack offsets truncated")
	}
	backOffs := make([]int, nBack)
	for i := 0; i < nBack; i++ {
		backOffs[i] = int(u16(sub, p+2*i))
	}
	p += 2 * nBack

	if p+2 > len(sub) {
		return 0, false, FormatError("reverse-chain lookahead count missing")
	}
	nAhead := int(u16(sub, p))
	p += 2
	if p+2*nAhead > len(sub) {
		return 0, false, FormatError("reverse-chain lookahead offsets truncated")
	}
	aheadOffs := make([]int, nAhead)
	for i := 0; i < nAhead; i++ {
		aheadOffs[i] = int(u16(sub, p+2*i))
	}
	p += 2 * nAhead

	if p+2 > len(sub) {
		return 0, false, FormatError("reverse-chain glyph count missing")
	}
	glyphCount := int(u16(sub, p))
	p += 2
	if p+2*glyphCount > len(sub) {
		return 0, false, FormatError("reverse-chain substitute ids truncated")
	}

	// Bounds check.
	if at < nBack {
		return 0, false, nil
	}
	if at+1+nAhead > len(glyphs) {
		return 0, false, nil
	}

	// Match backtrack.
	for i := 0; i < nBack; i++ {
		bCov, err := layout.ParseCoverage(sub, backOffs[i])
		if err != nil {
			return 0, false, err
		}
		if bCov.Index(glyphs[at-1-i]) < 0 {
			return 0, false, nil
		}
	}
	// Match lookahead.
	for i := 0; i < nAhead; i++ {
		aCov, err := layout.ParseCoverage(sub, aheadOffs[i])
		if err != nil {
			return 0, false, err
		}
		if aCov.Index(glyphs[at+1+i]) < 0 {
			return 0, false, nil
		}
	}
	if idx >= glyphCount {
		return 0, false, nil
	}
	return u16(sub, p+2*idx), true, nil
}
