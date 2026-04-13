// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gsub

import "github.com/KarpelesLab/gofreetype/layout"

// GSUB Type 3: alternate substitution. Each input glyph has a set of
// alternate glyphs; the shaper or application picks one (typically by
// feature parameter, or by default taking index 0).
//
//	uint16 format (1)
//	Offset16 coverageOffset
//	uint16 alternateSetCount
//	Offset16 alternateSetOffsets[alternateSetCount]
//
// AlternateSet:
//
//	uint16 glyphCount
//	uint16 alternateGlyphIDs[glyphCount]

// Alternates returns the list of alternate glyph ids for g via the named
// Type-3 lookup. The application is expected to choose one; by default
// alternates[0] is the first alternate.
func (t *Table) Alternates(lookupIndex uint16, g uint16) ([]uint16, bool) {
	if int(lookupIndex) >= len(t.Lookups) {
		return nil, false
	}
	lk := t.Lookups[lookupIndex]
	actualType, subtables := resolveExtension(lk.Type, lk.SubtableData)
	if actualType != 3 {
		return nil, false
	}
	for _, sub := range subtables {
		out, found, err := lookupAlternate(sub, g)
		if err != nil || !found {
			continue
		}
		return out, true
	}
	return nil, false
}

func lookupAlternate(sub []byte, g uint16) ([]uint16, bool, error) {
	if len(sub) < 6 {
		return nil, false, FormatError("alternate subtable header truncated")
	}
	format := u16(sub, 0)
	if format != 1 {
		return nil, false, UnsupportedError("alternate format " + intToStr(int(format)))
	}
	covOff := int(u16(sub, 2))
	count := int(u16(sub, 4))
	if 6+2*count > len(sub) {
		return nil, false, FormatError("alternate set offsets truncated")
	}
	cov, err := layout.ParseCoverage(sub, covOff)
	if err != nil {
		return nil, false, err
	}
	idx := cov.Index(g)
	if idx < 0 || idx >= count {
		return nil, false, nil
	}
	setOff := int(u16(sub, 6+2*idx))
	if setOff+2 > len(sub) {
		return nil, false, FormatError("AlternateSet header out of bounds")
	}
	glyphCount := int(u16(sub, setOff))
	if setOff+2+2*glyphCount > len(sub) {
		return nil, false, FormatError("AlternateSet body out of bounds")
	}
	out := make([]uint16, glyphCount)
	for i := 0; i < glyphCount; i++ {
		out[i] = u16(sub, setOff+2+2*i)
	}
	return out, true, nil
}
