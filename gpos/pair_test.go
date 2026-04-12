// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gpos

import (
	"encoding/binary"
	"testing"

	"github.com/KarpelesLab/gofreetype/layout"
)

// encU16 / encU32 are tiny big-endian encoders.
func encU16(b *[]byte, v uint16) {
	*b = append(*b, byte(v>>8), byte(v))
}
func encI16(b *[]byte, v int16) {
	encU16(b, uint16(v))
}
func encU32(b *[]byte, v uint32) {
	*b = append(*b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// buildCoverageFormat1 builds a Coverage table containing the given sorted
// glyph IDs.
func buildCoverageFormat1(gids []uint16) []byte {
	var b []byte
	encU16(&b, 1) // format
	encU16(&b, uint16(len(gids)))
	for _, g := range gids {
		encU16(&b, g)
	}
	return b
}

// buildPairPosFormat1 builds a PairPos format-1 subtable. Each PairSet is
// ordered by secondGlyph. valueFormat uses only XAdvance (0x0004).
func buildPairPosFormat1(pairsByFirst map[uint16]map[uint16]int16) []byte {
	// Order first glyphs.
	var firsts []uint16
	for g := range pairsByFirst {
		firsts = append(firsts, g)
	}
	sortU16(firsts)

	// We'll compute offsets after knowing sizes.
	cov := buildCoverageFormat1(firsts)

	// Each PairValueRecord: 2 bytes second + 2 bytes xAdvance.
	pairSets := make([][]byte, len(firsts))
	for i, g := range firsts {
		pairs := pairsByFirst[g]
		var seconds []uint16
		for s := range pairs {
			seconds = append(seconds, s)
		}
		sortU16(seconds)
		var ps []byte
		encU16(&ps, uint16(len(seconds)))
		for _, s := range seconds {
			encU16(&ps, s)
			encI16(&ps, pairs[s])
		}
		pairSets[i] = ps
	}

	headerLen := 10 + 2*len(firsts)
	covOff := headerLen
	psOffs := make([]int, len(firsts))
	cursor := covOff + len(cov)
	for i := range pairSets {
		psOffs[i] = cursor
		cursor += len(pairSets[i])
	}

	var b []byte
	encU16(&b, 1) // format
	encU16(&b, uint16(covOff))
	encU16(&b, 0x0004) // valueFormat1 = XAdvance
	encU16(&b, 0x0000) // valueFormat2 = none
	encU16(&b, uint16(len(firsts)))
	for _, off := range psOffs {
		encU16(&b, uint16(off))
	}
	b = append(b, cov...)
	for _, ps := range pairSets {
		b = append(b, ps...)
	}
	return b
}

func sortU16(s []uint16) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// buildGPOSWithPairLookup wraps a single PairPos subtable in a full GPOS
// table with one script, one DefaultLangSys with feature "kern", and one
// Lookup (type 2).
func buildGPOSWithPairLookup(sub []byte) []byte {
	// Header.
	header := make([]byte, 10)
	binary.BigEndian.PutUint16(header[0:], 1) // major
	binary.BigEndian.PutUint16(header[2:], 0)

	// ScriptList.
	var scriptList []byte
	encU16(&scriptList, 1) // scriptCount
	// latn record with offset to Script.
	scriptList = append(scriptList, 'l', 'a', 't', 'n')
	encU16(&scriptList, 8)
	// Script table at offset 8 from ScriptList start.
	encU16(&scriptList, 4) // defaultLangSysOffset
	encU16(&scriptList, 0) // langSysCount
	// DefaultLangSys at +4.
	encU16(&scriptList, 0)      // lookupOrderOffset
	encU16(&scriptList, 0xffff) // requiredFeatureIndex = none
	encU16(&scriptList, 1)      // featureIndexCount
	encU16(&scriptList, 0)      // featureIndex[0]

	// FeatureList.
	var featureList []byte
	encU16(&featureList, 1) // featureCount
	featureList = append(featureList, 'k', 'e', 'r', 'n')
	encU16(&featureList, 8) // offset to Feature
	// Feature table:
	encU16(&featureList, 0) // featureParamsOffset
	encU16(&featureList, 1) // lookupIndexCount
	encU16(&featureList, 0) // lookupIndex[0]

	// LookupList.
	var lookupList []byte
	encU16(&lookupList, 1)  // lookupCount
	encU16(&lookupList, 4)  // offset to Lookup[0]
	// Lookup table.
	encU16(&lookupList, 2) // type
	encU16(&lookupList, 0) // flag
	encU16(&lookupList, 1) // subtableCount
	// Subtable offset (relative to the Lookup table) = Lookup header (6) +
	// subtableOffsets array (2 bytes for 1 subtable) = 8.
	encU16(&lookupList, 8)
	// Subtable.
	lookupList = append(lookupList, sub...)

	// Patch header offsets.
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

func TestGPOSPairFormat1(t *testing.T) {
	pairs := map[uint16]map[uint16]int16{
		10: {20: -40, 30: -25},
		11: {20: 15},
	}
	sub := buildPairPosFormat1(pairs)
	data := buildGPOSWithPairLookup(sub)

	tbl, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(tbl.Lookups) != 1 {
		t.Fatalf("Lookups: got %d, want 1", len(tbl.Lookups))
	}

	for _, tc := range []struct {
		first, second uint16
		wantAdj       int16
		wantOK        bool
	}{
		{10, 20, -40, true},
		{10, 30, -25, true},
		{11, 20, 15, true},
		{10, 21, 0, false}, // second not in pairset for 10
		{12, 20, 0, false}, // first not in coverage
	} {
		v1, _, ok := tbl.Pair(0, tc.first, tc.second)
		if ok != tc.wantOK {
			t.Errorf("Pair(%d,%d) ok: got %v, want %v", tc.first, tc.second, ok, tc.wantOK)
		}
		if v1.XAdvance != tc.wantAdj {
			t.Errorf("Pair(%d,%d) XAdvance: got %d, want %d", tc.first, tc.second, v1.XAdvance, tc.wantAdj)
		}
	}

	// KernFeatureIndex finds the "kern" feature.
	idx := tbl.KernFeatureIndex(layout.MakeTag("latn"), layout.MakeTag("dflt"))
	if idx != 0 {
		t.Errorf("KernFeatureIndex: got %d, want 0", idx)
	}

	// PairKernAdvance convenience.
	if got, want := tbl.PairKernAdvance(0, 10, 20), int16(-40); got != want {
		t.Errorf("PairKernAdvance: got %d, want %d", got, want)
	}
}

// buildPairPosFormat2 builds a PairPos format-2 subtable. coverage contains
// only firstClass > 0 glyphs; class definitions are passed separately.
func buildPairPosFormat2(
	firstGIDs []uint16,
	class1Count, class2Count int,
	classDef1, classDef2 []byte,
	adjustments [][]int16, // [class1][class2] -> xAdv
) []byte {
	cov := buildCoverageFormat1(firstGIDs)
	headerLen := 16
	covOff := headerLen
	cd1Off := covOff + len(cov)
	cd2Off := cd1Off + len(classDef1)
	bodyOff := cd2Off + len(classDef2)
	_ = bodyOff

	var b []byte
	encU16(&b, 2) // format
	encU16(&b, uint16(covOff))
	encU16(&b, 0x0004) // valueFormat1
	encU16(&b, 0x0000) // valueFormat2
	encU16(&b, uint16(cd1Off))
	encU16(&b, uint16(cd2Off))
	encU16(&b, uint16(class1Count))
	encU16(&b, uint16(class2Count))
	// Class1Records immediately follow the header… but we said bodyOff is
	// after the classdefs. Actually, the class1Records start at offset 16
	// (right after the header) — classdefs are referenced by offsets that
	// can be anywhere. Let me put classdefs AFTER the class1Records.
	// Rebuild: header(16) + class1Records(class1Count*class2Count*2) +
	//          cov + classDef1 + classDef2.
	b = b[:0]
	recSize := 2
	class1RecordsLen := class1Count * class2Count * recSize
	newCovOff := 16 + class1RecordsLen
	newCD1Off := newCovOff + len(cov)
	newCD2Off := newCD1Off + len(classDef1)
	encU16(&b, 2)
	encU16(&b, uint16(newCovOff))
	encU16(&b, 0x0004)
	encU16(&b, 0x0000)
	encU16(&b, uint16(newCD1Off))
	encU16(&b, uint16(newCD2Off))
	encU16(&b, uint16(class1Count))
	encU16(&b, uint16(class2Count))
	for c1 := 0; c1 < class1Count; c1++ {
		for c2 := 0; c2 < class2Count; c2++ {
			encI16(&b, adjustments[c1][c2])
		}
	}
	b = append(b, cov...)
	b = append(b, classDef1...)
	b = append(b, classDef2...)
	return b
}

// classDef2Build builds a ClassDef format 2 with the given sorted ranges.
func classDef2Build(rs []struct{ start, end, class uint16 }) []byte {
	var b []byte
	encU16(&b, 2) // format
	encU16(&b, uint16(len(rs)))
	for _, r := range rs {
		encU16(&b, r.start)
		encU16(&b, r.end)
		encU16(&b, r.class)
	}
	return b
}

func TestGPOSPairFormat2(t *testing.T) {
	// firstGlyphs 10..12 are all in class 1.
	// secondGlyphs 20 -> class 1, 21 -> class 2.
	// Adjustments: class1=1,class2=1 -> -30; class1=1,class2=2 -> -50.
	firstGIDs := []uint16{10, 11, 12}
	classDef1 := classDef2Build([]struct{ start, end, class uint16 }{
		{10, 12, 1},
	})
	classDef2 := classDef2Build([]struct{ start, end, class uint16 }{
		{20, 20, 1},
		{21, 21, 2},
	})
	adj := [][]int16{
		{0, 0, 0},    // class1 = 0 (unclassified)
		{0, -30, -50}, // class1 = 1
	}
	sub := buildPairPosFormat2(firstGIDs, 2, 3, classDef1, classDef2, adj)
	data := buildGPOSWithPairLookup(sub)

	tbl, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, tc := range []struct {
		first, second uint16
		wantAdj       int16
		wantOK        bool
	}{
		{10, 20, -30, true},
		{11, 21, -50, true},
		{10, 21, -50, true},
		{10, 22, 0, false}, // second in no class
		{15, 20, 0, false}, // first not in coverage
	} {
		v1, _, ok := tbl.Pair(0, tc.first, tc.second)
		if ok != tc.wantOK {
			t.Errorf("Pair(%d,%d) ok: got %v, want %v", tc.first, tc.second, ok, tc.wantOK)
		}
		if v1.XAdvance != tc.wantAdj {
			t.Errorf("Pair(%d,%d) XAdvance: got %d, want %d", tc.first, tc.second, v1.XAdvance, tc.wantAdj)
		}
	}
}
