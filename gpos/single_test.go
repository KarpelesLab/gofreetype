// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gpos

import (
	"encoding/binary"
	"testing"
)

// buildSinglePosFormat1 builds a SinglePos format-1 subtable that applies
// xAdvance to every glyph in coverage.
func buildSinglePosFormat1(coverageGIDs []uint16, xAdvance int16) []byte {
	cov := buildCoverageFormat1(coverageGIDs)
	// ValueFormat = XAdvance (0x0004).
	var b []byte
	encU16(&b, 1)               // format
	covOff := 8                 // 6 header bytes + 2 valueRecord (xAdvance uint16)
	encU16(&b, uint16(covOff))
	encU16(&b, 0x0004) // valueFormat
	encI16(&b, xAdvance)
	b = append(b, cov...)
	return b
}

// buildSinglePosFormat2 builds a SinglePos format-2 subtable with per-glyph
// xAdvance values in the same order as coverage.
func buildSinglePosFormat2(coverageGIDs []uint16, xAdvances []int16) []byte {
	cov := buildCoverageFormat1(coverageGIDs)
	var b []byte
	encU16(&b, 2)  // format
	// Coverage comes after header + per-glyph values.
	headerLen := 8 // 2 format + 2 covOff + 2 valueFormat + 2 valueCount
	valuesLen := 2 * len(xAdvances)
	covOff := headerLen + valuesLen
	encU16(&b, uint16(covOff))
	encU16(&b, 0x0004) // valueFormat
	encU16(&b, uint16(len(xAdvances)))
	for _, v := range xAdvances {
		encI16(&b, v)
	}
	b = append(b, cov...)
	return b
}

// buildGPOSWithSubtable is a generalization of buildGPOSWithPairLookup that
// takes the lookup type.
func buildGPOSWithSubtable(lookupType uint16, sub []byte) []byte {
	header := make([]byte, 10)
	binary.BigEndian.PutUint16(header[0:], 1)
	binary.BigEndian.PutUint16(header[2:], 0)

	var scriptList []byte
	encU16(&scriptList, 1)
	scriptList = append(scriptList, 'l', 'a', 't', 'n')
	encU16(&scriptList, 8)
	encU16(&scriptList, 4)      // defaultLangSysOffset
	encU16(&scriptList, 0)      // langSysCount
	encU16(&scriptList, 0)      // lookupOrderOffset
	encU16(&scriptList, 0xffff) // requiredFeatureIndex
	encU16(&scriptList, 1)      // featureIndexCount
	encU16(&scriptList, 0)      // featureIndex

	var featureList []byte
	encU16(&featureList, 1) // featureCount
	featureList = append(featureList, 't', 'e', 's', 't')
	encU16(&featureList, 8) // offset to Feature
	encU16(&featureList, 0) // featureParamsOffset
	encU16(&featureList, 1) // lookupIndexCount
	encU16(&featureList, 0) // lookupIndex[0]

	var lookupList []byte
	encU16(&lookupList, 1) // lookupCount
	encU16(&lookupList, 4) // offset to Lookup[0]
	encU16(&lookupList, lookupType)
	encU16(&lookupList, 0) // flag
	encU16(&lookupList, 1) // subtableCount
	encU16(&lookupList, 8) // subtable offset (past the lookup header)
	lookupList = append(lookupList, sub...)

	scriptOff := 10
	featureOff := scriptOff + len(scriptList)
	lookupOff := featureOff + len(featureList)
	binary.BigEndian.PutUint16(header[4:], uint16(scriptOff))
	binary.BigEndian.PutUint16(header[6:], uint16(featureOff))
	binary.BigEndian.PutUint16(header[8:], uint16(lookupOff))

	out := append([]byte{}, header...)
	out = append(out, scriptList...)
	out = append(out, featureList...)
	out = append(out, lookupList...)
	return out
}

func TestGPOSSingleFormat1(t *testing.T) {
	sub := buildSinglePosFormat1([]uint16{10, 20, 30}, -15)
	data := buildGPOSWithSubtable(1, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range []uint16{10, 20, 30} {
		v, ok := tbl.Single(0, g)
		if !ok {
			t.Errorf("Single(0, %d) ok: got false, want true", g)
		}
		if v.XAdvance != -15 {
			t.Errorf("Single(0, %d) XAdvance: got %d, want -15", g, v.XAdvance)
		}
	}
	if _, ok := tbl.Single(0, 99); ok {
		t.Error("Single(0, 99) ok: got true, want false")
	}
}

func TestGPOSSingleFormat2(t *testing.T) {
	sub := buildSinglePosFormat2([]uint16{10, 11, 12}, []int16{-5, -10, -15})
	data := buildGPOSWithSubtable(1, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for i, g := range []uint16{10, 11, 12} {
		v, ok := tbl.Single(0, g)
		if !ok {
			t.Errorf("Single(0, %d) ok: got false, want true", g)
		}
		wantAdv := int16(-5 - 5*i)
		if v.XAdvance != wantAdv {
			t.Errorf("Single(0, %d) XAdvance: got %d, want %d", g, v.XAdvance, wantAdv)
		}
	}
}

func TestGPOSExtension(t *testing.T) {
	// Wrap a SinglePos format-1 subtable in an Extension (Type 9) subtable.
	inner := buildSinglePosFormat1([]uint16{42}, -7)
	var ext []byte
	encU16(&ext, 1) // extension format
	encU16(&ext, 1) // extensionLookupType = 1 (single)
	// extensionOffset from this subtable's start. We'll place the inner
	// subtable right after the 8-byte header.
	encU32(&ext, 8)
	ext = append(ext, inner...)

	data := buildGPOSWithSubtable(9, ext)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := tbl.Single(0, 42)
	if !ok {
		t.Fatal("Single(0, 42) via Extension: got ok=false")
	}
	if v.XAdvance != -7 {
		t.Errorf("Single via Extension XAdvance: got %d, want -7", v.XAdvance)
	}

	// Extension reporting the wrong inner type (Pair=2) should not be
	// dispatched by Single().
	var extPair []byte
	encU16(&extPair, 1)
	encU16(&extPair, 2) // pair
	encU32(&extPair, 8)
	extPair = append(extPair, inner...)
	data2 := buildGPOSWithSubtable(9, extPair)
	tbl2, err := Parse(data2)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tbl2.Single(0, 42); ok {
		t.Error("Single() matched a pair-typed extension: want false")
	}
}
