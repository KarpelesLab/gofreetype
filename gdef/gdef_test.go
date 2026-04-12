// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gdef

import (
	"encoding/binary"
	"testing"
)

// buildGDEF constructs a minimal valid GDEF with the given sub-tables inlined.
// Each input is either nil (absent, offset = 0) or a byte slice that will be
// appended and referenced by its relative offset from the GDEF start.
func buildGDEF(minor uint16, glyphClass, markAttachClass []byte, markSets []byte) []byte {
	headerLen := 12
	if minor >= 2 {
		headerLen = 14
	}
	header := make([]byte, headerLen)
	binary.BigEndian.PutUint16(header[0:], 1)     // major
	binary.BigEndian.PutUint16(header[2:], minor)

	cursor := headerLen
	if glyphClass != nil {
		binary.BigEndian.PutUint16(header[4:], uint16(cursor))
		cursor += len(glyphClass)
	}
	// offset 6 = AttachList — unused, 0
	// offset 8 = LigCaretList — unused, 0
	if markAttachClass != nil {
		binary.BigEndian.PutUint16(header[10:], uint16(cursor))
		cursor += len(markAttachClass)
	}
	if minor >= 2 && markSets != nil {
		binary.BigEndian.PutUint16(header[12:], uint16(cursor))
		cursor += len(markSets)
	}

	out := make([]byte, 0, cursor)
	out = append(out, header...)
	if glyphClass != nil {
		out = append(out, glyphClass...)
	}
	if markAttachClass != nil {
		out = append(out, markAttachClass...)
	}
	if minor >= 2 && markSets != nil {
		out = append(out, markSets...)
	}
	return out
}

// classDef1 builds a ClassDef format 1 at startGID with the given class array.
func classDef1(startGID uint16, classes []uint16) []byte {
	b := []byte{0, 1, byte(startGID >> 8), byte(startGID), byte(len(classes) >> 8), byte(len(classes))}
	for _, c := range classes {
		b = append(b, byte(c>>8), byte(c))
	}
	return b
}

func TestParseGDEFGlyphClass(t *testing.T) {
	// glyph 1 = base, 2 = ligature, 3 = mark, 4 = component.
	gcd := classDef1(1, []uint16{1, 2, 3, 4})
	data := buildGDEF(0, gcd, nil, nil)
	gd, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for gid, want := range map[uint16]uint16{
		0: 0,
		1: ClassBase,
		2: ClassLigature,
		3: ClassMark,
		4: ClassComponent,
		5: 0,
	} {
		if got := gd.Class(gid); got != want {
			t.Errorf("Class(%d): got %d, want %d", gid, got, want)
		}
	}
}

func TestParseGDEFMarkAttachClass(t *testing.T) {
	// Glyphs 10..12 are in mark attach classes 1, 2, 1.
	mac := classDef1(10, []uint16{1, 2, 1})
	data := buildGDEF(0, nil, mac, nil)
	gd, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for gid, want := range map[uint16]uint16{9: 0, 10: 1, 11: 2, 12: 1, 13: 0} {
		if got := gd.MarkAttachClass(gid); got != want {
			t.Errorf("MarkAttachClass(%d): got %d, want %d", gid, got, want)
		}
	}
}

func TestParseGDEFMarkGlyphSets(t *testing.T) {
	// MarkGlyphSetsDef: format 1, 2 sets.
	// Set 0: Coverage format 1 with {100, 101}.
	// Set 1: Coverage format 1 with {200}.
	set0 := []byte{0, 1, 0, 2, 0, 100, 0, 101}
	set1 := []byte{0, 1, 0, 1, 0, 200}

	mgsHeader := []byte{
		0, 1, // format
		0, 2, // markGlyphSetCount
	}
	// Placeholder 32-bit offsets (will be filled in after knowing layout).
	mgsHeaderLen := len(mgsHeader) + 8
	_ = mgsHeaderLen
	mgs := make([]byte, 0)
	mgs = append(mgs, mgsHeader...)
	// coverageOffset[0] = offset to set0 from start of MarkGlyphSetsDef.
	set0Off := uint32(len(mgs) + 8) // we still need to append 8 more bytes for the two offsets
	mgs = append(mgs, 0, 0, 0, 0)
	set1Off := set0Off + uint32(len(set0))
	mgs = append(mgs, 0, 0, 0, 0)
	binary.BigEndian.PutUint32(mgs[4:], set0Off)
	binary.BigEndian.PutUint32(mgs[8:], set1Off)
	mgs = append(mgs, set0...)
	mgs = append(mgs, set1...)

	data := buildGDEF(2, nil, nil, mgs)
	gd, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(gd.MarkGlyphSets) != 2 {
		t.Fatalf("MarkGlyphSets: got %d sets, want 2", len(gd.MarkGlyphSets))
	}
	// Set 0 membership.
	for _, g := range []uint16{100, 101} {
		if !gd.IsMarkInSet(0, g) {
			t.Errorf("IsMarkInSet(0, %d): got false, want true", g)
		}
	}
	if gd.IsMarkInSet(0, 102) {
		t.Error("IsMarkInSet(0, 102): got true, want false")
	}
	// Set 1 membership.
	if !gd.IsMarkInSet(1, 200) {
		t.Error("IsMarkInSet(1, 200): got false, want true")
	}
	if gd.IsMarkInSet(1, 100) {
		t.Error("IsMarkInSet(1, 100): got true, want false")
	}
	// Out-of-range set.
	if gd.IsMarkInSet(5, 100) {
		t.Error("IsMarkInSet(5, 100): got true, want false")
	}
}
