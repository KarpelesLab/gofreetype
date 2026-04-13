// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gsub

import "github.com/KarpelesLab/gofreetype/layout"

// GSUB Type 2: multiple substitution. Replace one input glyph with N
// output glyphs (N may be zero per the spec, though that case is rare).
//
//	uint16 format (1)
//	Offset16 coverageOffset
//	uint16 sequenceCount
//	Offset16 sequenceOffsets[sequenceCount]
//
// Each Sequence:
//
//	uint16 glyphCount
//	uint16 substituteGlyphIDs[glyphCount]

// Multiple returns the replacement sequence for g via the named Type-2
// lookup.
func (t *Table) Multiple(lookupIndex uint16, g uint16) ([]uint16, bool) {
	if int(lookupIndex) >= len(t.Lookups) {
		return nil, false
	}
	lk := t.Lookups[lookupIndex]
	actualType, subtables := resolveExtension(lk.Type, lk.SubtableData)
	if actualType != 2 {
		return nil, false
	}
	for _, sub := range subtables {
		out, found, err := lookupMultiple(sub, g)
		if err != nil || !found {
			continue
		}
		return out, true
	}
	return nil, false
}

func lookupMultiple(sub []byte, g uint16) ([]uint16, bool, error) {
	if len(sub) < 6 {
		return nil, false, FormatError("multiple subtable header truncated")
	}
	format := u16(sub, 0)
	if format != 1 {
		return nil, false, UnsupportedError("multiple format " + intToStr(int(format)))
	}
	covOff := int(u16(sub, 2))
	count := int(u16(sub, 4))
	if 6+2*count > len(sub) {
		return nil, false, FormatError("multiple sequenceOffsets truncated")
	}
	cov, err := layout.ParseCoverage(sub, covOff)
	if err != nil {
		return nil, false, err
	}
	idx := cov.Index(g)
	if idx < 0 || idx >= count {
		return nil, false, nil
	}
	seqOff := int(u16(sub, 6+2*idx))
	if seqOff+2 > len(sub) {
		return nil, false, FormatError("Sequence header out of bounds")
	}
	glyphCount := int(u16(sub, seqOff))
	if seqOff+2+2*glyphCount > len(sub) {
		return nil, false, FormatError("Sequence body out of bounds")
	}
	out := make([]uint16, glyphCount)
	for i := 0; i < glyphCount; i++ {
		out[i] = u16(sub, seqOff+2+2*i)
	}
	return out, true, nil
}
