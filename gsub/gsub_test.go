// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gsub

import (
	"encoding/binary"
	"testing"
)

func encU16(b *[]byte, v uint16) { *b = append(*b, byte(v>>8), byte(v)) }
func encI16(b *[]byte, v int16)  { encU16(b, uint16(v)) }
func encU32(b *[]byte, v uint32) { *b = append(*b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v)) }

// buildCoverageFormat1 builds a Coverage table with the given sorted glyph ids.
func buildCoverageFormat1(gids []uint16) []byte {
	var b []byte
	encU16(&b, 1)
	encU16(&b, uint16(len(gids)))
	for _, g := range gids {
		encU16(&b, g)
	}
	return b
}

// buildGSUBWithSubtable wraps one GSUB subtable in a full table with one
// script, one default LangSys pointing at feature 0, one feature "test",
// and one lookup of the given type.
func buildGSUBWithSubtable(lookupType uint16, sub []byte) []byte {
	header := make([]byte, 10)
	binary.BigEndian.PutUint16(header[0:], 1)
	binary.BigEndian.PutUint16(header[2:], 0)

	var scriptList []byte
	encU16(&scriptList, 1)
	scriptList = append(scriptList, 'l', 'a', 't', 'n')
	encU16(&scriptList, 8)
	encU16(&scriptList, 4)
	encU16(&scriptList, 0)
	encU16(&scriptList, 0)
	encU16(&scriptList, 0xffff)
	encU16(&scriptList, 1)
	encU16(&scriptList, 0)

	var featureList []byte
	encU16(&featureList, 1)
	featureList = append(featureList, 't', 'e', 's', 't')
	encU16(&featureList, 8)
	encU16(&featureList, 0)
	encU16(&featureList, 1)
	encU16(&featureList, 0)

	var lookupList []byte
	encU16(&lookupList, 1)
	encU16(&lookupList, 4)
	encU16(&lookupList, lookupType)
	encU16(&lookupList, 0)
	encU16(&lookupList, 1)
	encU16(&lookupList, 8)
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

func TestGSUBSingleFormat1(t *testing.T) {
	// Single format 1: deltaGlyphID = +100. Coverage = {10, 20}.
	// Layout: format + covOff + delta = 6 bytes; coverage follows.
	cov := buildCoverageFormat1([]uint16{10, 20})
	var sub []byte
	encU16(&sub, 1)
	encU16(&sub, 6) // covOff (past header)
	encI16(&sub, 100)
	sub = append(sub, cov...)

	data := buildGSUBWithSubtable(1, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		g    uint16
		want uint16
		ok   bool
	}{
		{10, 110, true},
		{20, 120, true},
		{15, 0, false},
	} {
		got, ok := tbl.Single(0, tc.g)
		if ok != tc.ok {
			t.Errorf("Single(%d) ok: got %v, want %v", tc.g, ok, tc.ok)
		}
		if got != tc.want {
			t.Errorf("Single(%d) out: got %d, want %d", tc.g, got, tc.want)
		}
	}
}

func TestGSUBSingleFormat2(t *testing.T) {
	// Format 2: each covered glyph has an explicit replacement id.
	cov := buildCoverageFormat1([]uint16{5, 6, 7})
	var sub []byte
	encU16(&sub, 2)
	// Coverage goes after the replacement array.
	glyphCount := 3
	covOff := 6 + 2*glyphCount
	encU16(&sub, uint16(covOff))
	encU16(&sub, uint16(glyphCount))
	encU16(&sub, 55)
	encU16(&sub, 66)
	encU16(&sub, 77)
	sub = append(sub, cov...)

	data := buildGSUBWithSubtable(1, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for in, want := range map[uint16]uint16{5: 55, 6: 66, 7: 77} {
		got, ok := tbl.Single(0, in)
		if !ok || got != want {
			t.Errorf("Single(%d): got (%d, %v), want (%d, true)", in, got, ok, want)
		}
	}
}

func TestGSUBMultiple(t *testing.T) {
	// Coverage = {77}. Replace 77 with [70, 71, 72].
	cov := buildCoverageFormat1([]uint16{77})
	var seq []byte
	encU16(&seq, 3)
	encU16(&seq, 70)
	encU16(&seq, 71)
	encU16(&seq, 72)

	// Layout: format(2) + covOff(2) + seqCount(2) + seqOffs(2) +
	//         coverage + sequence.
	headerLen := 8
	covOff := headerLen
	seqOff := covOff + len(cov)

	var sub []byte
	encU16(&sub, 1)
	encU16(&sub, uint16(covOff))
	encU16(&sub, 1)
	encU16(&sub, uint16(seqOff))
	sub = append(sub, cov...)
	sub = append(sub, seq...)

	data := buildGSUBWithSubtable(2, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	out, ok := tbl.Multiple(0, 77)
	if !ok {
		t.Fatal("Multiple(77): ok=false")
	}
	if len(out) != 3 || out[0] != 70 || out[1] != 71 || out[2] != 72 {
		t.Errorf("Multiple(77): got %v, want [70 71 72]", out)
	}
	if _, ok := tbl.Multiple(0, 99); ok {
		t.Error("Multiple(99) for uncovered glyph: ok=true, want false")
	}
}

func TestGSUBAlternate(t *testing.T) {
	// Coverage = {11}. Alternates for 11 = [101, 102, 103].
	cov := buildCoverageFormat1([]uint16{11})
	var altSet []byte
	encU16(&altSet, 3)
	encU16(&altSet, 101)
	encU16(&altSet, 102)
	encU16(&altSet, 103)

	headerLen := 8
	covOff := headerLen
	altOff := covOff + len(cov)

	var sub []byte
	encU16(&sub, 1)
	encU16(&sub, uint16(covOff))
	encU16(&sub, 1)
	encU16(&sub, uint16(altOff))
	sub = append(sub, cov...)
	sub = append(sub, altSet...)

	data := buildGSUBWithSubtable(3, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	alts, ok := tbl.Alternates(0, 11)
	if !ok {
		t.Fatal("Alternates(11): ok=false")
	}
	if len(alts) != 3 || alts[0] != 101 || alts[1] != 102 || alts[2] != 103 {
		t.Errorf("Alternates(11): got %v, want [101 102 103]", alts)
	}
}

func TestGSUBLigature(t *testing.T) {
	// Coverage on first glyph = {1}. Two ligatures rooted at gid 1:
	//   [1, 2] -> 100
	//   [1, 2, 3] -> 200
	// Longest-match semantics are the caller's job; the lookup returns the
	// first declared ligature whose full component list matches a prefix.
	// Per spec, fonts usually list the longest first.
	cov := buildCoverageFormat1([]uint16{1})

	// Ligature record: ligGlyph(2) + componentCount(2) + componentGlyphIDs[count-1]
	buildLig := func(ligGID uint16, tail []uint16) []byte {
		var b []byte
		encU16(&b, ligGID)
		encU16(&b, uint16(1+len(tail))) // componentCount
		for _, c := range tail {
			encU16(&b, c)
		}
		return b
	}
	lig1 := buildLig(200, []uint16{2, 3}) // longest first
	lig2 := buildLig(100, []uint16{2})

	// LigatureSet: count(2) + offsets[count](2 each) + Ligature bodies.
	setHeaderLen := 2 + 2*2
	setBodyLen := len(lig1) + len(lig2)
	_ = setBodyLen
	off1 := setHeaderLen
	off2 := off1 + len(lig1)

	var set []byte
	encU16(&set, 2)
	encU16(&set, uint16(off1))
	encU16(&set, uint16(off2))
	set = append(set, lig1...)
	set = append(set, lig2...)

	// Subtable: format(2) + covOff(2) + setCount(2) + setOff(2) + coverage + LigatureSet.
	headerLen := 8
	covOff := headerLen
	ligSetOff := covOff + len(cov)

	var sub []byte
	encU16(&sub, 1)
	encU16(&sub, uint16(covOff))
	encU16(&sub, 1)
	encU16(&sub, uint16(ligSetOff))
	sub = append(sub, cov...)
	sub = append(sub, set...)

	data := buildGSUBWithSubtable(4, sub)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	// Input [1, 2, 3] -> match [1, 2, 3], glyph 200, consumed 3.
	g, n, ok := tbl.Ligature(0, []uint16{1, 2, 3, 4})
	if !ok || g != 200 || n != 3 {
		t.Errorf("Ligature([1,2,3,...]): got (%d, %d, %v), want (200, 3, true)", g, n, ok)
	}
	// Input [1, 2] alone -> match [1, 2], glyph 100, consumed 2.
	g, n, ok = tbl.Ligature(0, []uint16{1, 2})
	if !ok || g != 100 || n != 2 {
		t.Errorf("Ligature([1,2]): got (%d, %d, %v), want (100, 2, true)", g, n, ok)
	}
	// Input [1, 5] -> no match.
	if _, _, ok := tbl.Ligature(0, []uint16{1, 5}); ok {
		t.Error("Ligature([1,5]): ok=true, want false")
	}
	// Input starting with uncovered glyph -> no match.
	if _, _, ok := tbl.Ligature(0, []uint16{9, 2, 3}); ok {
		t.Error("Ligature([9,2,3]): ok=true, want false")
	}
}

func TestGSUBExtension(t *testing.T) {
	// Wrap a Single format-1 subtable in Extension (Type 7) with inner type 1.
	cov := buildCoverageFormat1([]uint16{42})
	var inner []byte
	encU16(&inner, 1)
	encU16(&inner, 6)
	encI16(&inner, 10)
	inner = append(inner, cov...)

	var ext []byte
	encU16(&ext, 1) // extension format
	encU16(&ext, 1) // innerLookupType
	encU32(&ext, 8) // offset to inner subtable
	ext = append(ext, inner...)

	data := buildGSUBWithSubtable(7, ext)
	tbl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := tbl.Single(0, 42)
	if !ok || got != 52 {
		t.Errorf("Single via Extension: got (%d, %v), want (52, true)", got, ok)
	}
}
