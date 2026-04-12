// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gpos

import "github.com/KarpelesLab/gofreetype/layout"

// lookupPair dispatches on the subtable format of a GPOS Type-2 subtable
// and looks up the pair (firstGlyph, secondGlyph). It returns the two
// ValueRecords, whether a match was found, and any error encountered while
// decoding.
func lookupPair(sub []byte, firstGlyph, secondGlyph uint16) (v1, v2 ValueRecord, found bool, err error) {
	if len(sub) < 4 {
		return ValueRecord{}, ValueRecord{}, false, FormatError("pair subtable header truncated")
	}
	format := u16(sub, 0)
	switch format {
	case 1:
		return lookupPairFormat1(sub, firstGlyph, secondGlyph)
	case 2:
		return lookupPairFormat2(sub, firstGlyph, secondGlyph)
	}
	return ValueRecord{}, ValueRecord{}, false, UnsupportedError("pair subtable format " + intCount(int(format)))
}

// lookupPairFormat1 handles Pair Adjustment Positioning, Format 1:
//
//	uint16 format (1)
//	Offset16 coverageOffset
//	uint16 valueFormat1
//	uint16 valueFormat2
//	uint16 pairSetCount
//	Offset16 pairSetOffsets[pairSetCount]
//
// Each PairSet is:
//
//	uint16 pairValueCount
//	PairValueRecord pairValueRecords[pairValueCount]
//
// and each PairValueRecord is:
//
//	uint16 secondGlyph
//	ValueRecord valueRecord1
//	ValueRecord valueRecord2
func lookupPairFormat1(sub []byte, firstGlyph, secondGlyph uint16) (ValueRecord, ValueRecord, bool, error) {
	if len(sub) < 10 {
		return ValueRecord{}, ValueRecord{}, false, FormatError("PairPos format 1 header truncated")
	}
	covOff := int(u16(sub, 2))
	valueFormat1 := u16(sub, 4)
	valueFormat2 := u16(sub, 6)
	nPairSets := int(u16(sub, 8))
	if 10+2*nPairSets > len(sub) {
		return ValueRecord{}, ValueRecord{}, false, FormatError("PairPos format 1 pair sets truncated")
	}
	cov, err := layout.ParseCoverage(sub, covOff)
	if err != nil {
		return ValueRecord{}, ValueRecord{}, false, err
	}
	idx := cov.Index(firstGlyph)
	if idx < 0 || idx >= nPairSets {
		return ValueRecord{}, ValueRecord{}, false, nil
	}
	psOff := int(u16(sub, 10+2*idx))
	if psOff+2 > len(sub) {
		return ValueRecord{}, ValueRecord{}, false, FormatError("PairSet offset out of bounds")
	}
	nPairs := int(u16(sub, psOff))
	recSize := 2 + valueRecordSize(valueFormat1) + valueRecordSize(valueFormat2)
	body := psOff + 2
	if body+nPairs*recSize > len(sub) {
		return ValueRecord{}, ValueRecord{}, false, FormatError("PairSet body truncated")
	}
	// Binary search by secondGlyph — pairValueRecords are spec-ordered.
	lo, hi := 0, nPairs
	for lo < hi {
		m := lo + (hi-lo)/2
		recOff := body + m*recSize
		g := u16(sub, recOff)
		switch {
		case g < secondGlyph:
			lo = m + 1
		case g > secondGlyph:
			hi = m
		default:
			v1, n, err := decodeValueRecord(sub, recOff+2, valueFormat1)
			if err != nil {
				return ValueRecord{}, ValueRecord{}, false, err
			}
			v2, _, err := decodeValueRecord(sub, recOff+2+n, valueFormat2)
			if err != nil {
				return ValueRecord{}, ValueRecord{}, false, err
			}
			return v1, v2, true, nil
		}
	}
	return ValueRecord{}, ValueRecord{}, false, nil
}

// lookupPairFormat2 handles Pair Adjustment Positioning, Format 2:
//
//	uint16 format (2)
//	Offset16 coverageOffset
//	uint16 valueFormat1
//	uint16 valueFormat2
//	Offset16 classDef1Offset
//	Offset16 classDef2Offset
//	uint16 class1Count
//	uint16 class2Count
//	Class1Record class1Records[class1Count]
//
// Each Class1Record has class2Count Class2Records; each Class2Record is
// two ValueRecords.
func lookupPairFormat2(sub []byte, firstGlyph, secondGlyph uint16) (ValueRecord, ValueRecord, bool, error) {
	if len(sub) < 16 {
		return ValueRecord{}, ValueRecord{}, false, FormatError("PairPos format 2 header truncated")
	}
	covOff := int(u16(sub, 2))
	valueFormat1 := u16(sub, 4)
	valueFormat2 := u16(sub, 6)
	cd1Off := int(u16(sub, 8))
	cd2Off := int(u16(sub, 10))
	class1Count := int(u16(sub, 12))
	class2Count := int(u16(sub, 14))

	cov, err := layout.ParseCoverage(sub, covOff)
	if err != nil {
		return ValueRecord{}, ValueRecord{}, false, err
	}
	if cov.Index(firstGlyph) < 0 {
		// First glyph must be in the coverage — otherwise no adjustment.
		return ValueRecord{}, ValueRecord{}, false, nil
	}
	cd1, err := layout.ParseClassDef(sub, cd1Off)
	if err != nil {
		return ValueRecord{}, ValueRecord{}, false, err
	}
	cd2, err := layout.ParseClassDef(sub, cd2Off)
	if err != nil {
		return ValueRecord{}, ValueRecord{}, false, err
	}
	c1 := int(cd1.Class(firstGlyph))
	c2 := int(cd2.Class(secondGlyph))
	if c1 >= class1Count || c2 >= class2Count {
		return ValueRecord{}, ValueRecord{}, false, nil
	}
	recSize := valueRecordSize(valueFormat1) + valueRecordSize(valueFormat2)
	body := 16
	recOff := body + (c1*class2Count+c2)*recSize
	if recOff+recSize > len(sub) {
		return ValueRecord{}, ValueRecord{}, false, FormatError("PairPos format 2 Class2Record out of bounds")
	}
	v1, n, err := decodeValueRecord(sub, recOff, valueFormat1)
	if err != nil {
		return ValueRecord{}, ValueRecord{}, false, err
	}
	v2, _, err := decodeValueRecord(sub, recOff+n, valueFormat2)
	if err != nil {
		return ValueRecord{}, ValueRecord{}, false, err
	}
	if v1.IsZero() && v2.IsZero() {
		// Spec: "The class1Records... array... includes a Class2Record for
		// every class2 class, even if no adjustments are applied." So we
		// only report "found" when something non-zero is there.
		return v1, v2, false, nil
	}
	return v1, v2, true, nil
}
