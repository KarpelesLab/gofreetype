// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gpos

import "github.com/KarpelesLab/gofreetype/layout"

// lookupSingle decodes a GPOS Type-1 (Single Adjustment) subtable and
// returns the ValueRecord for g, or an unset ValueRecord if g is not
// covered.
//
// Format 1 applies the same ValueRecord to every glyph in Coverage:
//
//	uint16 format (1)
//	Offset16 coverageOffset
//	uint16 valueFormat
//	ValueRecord value
//
// Format 2 gives each glyph its own ValueRecord (indexed by coverage
// index):
//
//	uint16 format (2)
//	Offset16 coverageOffset
//	uint16 valueFormat
//	uint16 valueCount
//	ValueRecord values[valueCount]
func lookupSingle(sub []byte, g uint16) (ValueRecord, bool, error) {
	if len(sub) < 6 {
		return ValueRecord{}, false, FormatError("single-pos subtable header truncated")
	}
	format := u16(sub, 0)
	covOff := int(u16(sub, 2))
	valueFormat := u16(sub, 4)
	cov, err := layout.ParseCoverage(sub, covOff)
	if err != nil {
		return ValueRecord{}, false, err
	}
	idx := cov.Index(g)
	if idx < 0 {
		return ValueRecord{}, false, nil
	}
	switch format {
	case 1:
		v, _, err := decodeValueRecord(sub, 6, valueFormat)
		if err != nil {
			return ValueRecord{}, false, err
		}
		return v, true, nil
	case 2:
		if len(sub) < 8 {
			return ValueRecord{}, false, FormatError("single-pos format 2 truncated")
		}
		count := int(u16(sub, 6))
		if idx >= count {
			return ValueRecord{}, false, nil
		}
		recSize := valueRecordSize(valueFormat)
		off := 8 + idx*recSize
		v, _, err := decodeValueRecord(sub, off, valueFormat)
		if err != nil {
			return ValueRecord{}, false, err
		}
		return v, true, nil
	}
	return ValueRecord{}, false, UnsupportedError("single-pos format " + intCount(int(format)))
}

// Single looks up a GPOS Type-1 adjustment for a single glyph via the named
// Lookup index. If the lookup is not Type 1 or the glyph is not covered,
// ok is false.
func (t *Table) Single(lookupIndex uint16, g uint16) (v ValueRecord, ok bool) {
	if int(lookupIndex) >= len(t.Lookups) {
		return ValueRecord{}, false
	}
	lk := t.Lookups[lookupIndex]
	actualType, subtables := resolveExtension(lk.Type, lk.SubtableData)
	if actualType != 1 {
		return ValueRecord{}, false
	}
	for _, sub := range subtables {
		got, found, err := lookupSingle(sub, g)
		if err != nil {
			continue
		}
		if found {
			return got, true
		}
	}
	return ValueRecord{}, false
}
