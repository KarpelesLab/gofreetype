// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package layout

import (
	"encoding/binary"
	"testing"
)

func TestMakeTagAndString(t *testing.T) {
	for _, tc := range []struct {
		in  string
		str string
		raw uint32
	}{
		{"latn", "latn", 0x6c61746e},
		{"liga", "liga", 0x6c696761},
		{"DFLT", "DFLT", 0x44464c54},
		{"d", "d", 0x64202020},
		{"kern", "kern", 0x6b65726e},
	} {
		got := MakeTag(tc.in)
		if uint32(got) != tc.raw {
			t.Errorf("MakeTag(%q): got 0x%08x, want 0x%08x", tc.in, uint32(got), tc.raw)
		}
		if got.String() != tc.str {
			t.Errorf("Tag(%08x).String(): got %q, want %q", uint32(got), got.String(), tc.str)
		}
	}
}

func TestCoverageFormat1(t *testing.T) {
	// Coverage format 1 with glyphs [10, 20, 30, 40].
	data := []byte{
		0, 1, // format
		0, 4, // glyphCount
		0, 10, 0, 20, 0, 30, 0, 40,
	}
	c, err := ParseCoverage(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, g := range []uint16{10, 20, 30, 40} {
		if got := c.Index(g); got != i {
			t.Errorf("Index(%d): got %d, want %d", g, got, i)
		}
	}
	if got := c.Index(15); got != -1 {
		t.Errorf("Index(15) for non-covered glyph: got %d, want -1", got)
	}
	if got := c.Len(); got != 4 {
		t.Errorf("Len: got %d, want 4", got)
	}
}

func TestCoverageFormat2(t *testing.T) {
	// Two ranges: [100, 110] -> start idx 0 (covers 11 glyphs),
	//             [200, 202] -> start idx 11 (covers 3 glyphs).
	data := []byte{
		0, 2, // format
		0, 2, // rangeCount
		0, 100, 0, 110, 0, 0,
		0, 200, 0, 202, 0, 11,
	}
	c, err := ParseCoverage(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := c.Index(100), 0; got != want {
		t.Errorf("Index(100): got %d, want %d", got, want)
	}
	if got, want := c.Index(105), 5; got != want {
		t.Errorf("Index(105): got %d, want %d", got, want)
	}
	if got, want := c.Index(110), 10; got != want {
		t.Errorf("Index(110): got %d, want %d", got, want)
	}
	if got, want := c.Index(200), 11; got != want {
		t.Errorf("Index(200): got %d, want %d", got, want)
	}
	if got, want := c.Index(202), 13; got != want {
		t.Errorf("Index(202): got %d, want %d", got, want)
	}
	if got := c.Index(150); got != -1 {
		t.Errorf("Index(150) between ranges: got %d, want -1", got)
	}
	if got, want := c.Len(), 14; got != want {
		t.Errorf("Len: got %d, want %d", got, want)
	}
}

func TestClassDefFormat1(t *testing.T) {
	// startGID 50, classes [1, 2, 0, 3].
	data := []byte{
		0, 1, // format
		0, 50, // startGID
		0, 4, // glyphCount
		0, 1, 0, 2, 0, 0, 0, 3,
	}
	c, err := ParseClassDef(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	for gid, want := range map[uint16]uint16{
		49: 0, // below start -> 0
		50: 1,
		51: 2,
		52: 0,
		53: 3,
		54: 0, // above -> 0
	} {
		if got := c.Class(gid); got != want {
			t.Errorf("Class(%d): got %d, want %d", gid, got, want)
		}
	}
}

func TestClassDefFormat2(t *testing.T) {
	// Two ranges: [10, 19] -> class 1; [30, 30] -> class 5.
	data := []byte{
		0, 2, // format
		0, 2, // rangeCount
		0, 10, 0, 19, 0, 1,
		0, 30, 0, 30, 0, 5,
	}
	c, err := ParseClassDef(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	for gid, want := range map[uint16]uint16{
		9:  0,
		10: 1,
		15: 1,
		19: 1,
		20: 0,
		30: 5,
		31: 0,
	} {
		if got := c.Class(gid); got != want {
			t.Errorf("Class(%d): got %d, want %d", gid, got, want)
		}
	}
}

// buildLayoutTable assembles a minimal valid GSUB/GPOS-shape table with one
// script "latn", one DefaultLangSys containing feature index 0, one feature
// "kern" referring to lookup 0, and one Lookup (type 2, no subtables).
func buildLayoutTable() []byte {
	// We'll lay out the sections and patch offsets.
	// Order: header, ScriptList, FeatureList, LookupList.
	header := make([]byte, 10)
	binary.BigEndian.PutUint16(header[0:], 1) // majorVersion
	binary.BigEndian.PutUint16(header[2:], 0) // minorVersion

	// ScriptList.
	scriptList := []byte{
		0, 1, // scriptCount
		'l', 'a', 't', 'n', 0, 8, // 'latn' record -> offset 8 from ScriptList start
		// Script table at offset 8:
		0, 4, // defaultLangSysOffset (relative to Script start)
		0, 0, // langSysCount
		// DefaultLangSys at +4:
		0, 0, // lookupOrderOffset (reserved)
		0xff, 0xff, // requiredFeatureIndex = none
		0, 1, // featureIndexCount
		0, 0, // featureIndex[0] = 0
	}

	// FeatureList.
	featureList := []byte{
		0, 1, // featureCount
		'k', 'e', 'r', 'n', 0, 8, // 'kern' -> offset 8 from FeatureList start
		// Feature table:
		0, 0, // featureParamsOffset
		0, 1, // lookupIndexCount
		0, 0, // lookupIndex[0] = 0
	}

	// LookupList.
	lookupList := []byte{
		0, 1, // lookupCount
		0, 4, // offset to Lookup[0] from LookupList start
		// Lookup table:
		0, 2, // lookupType
		0, 0, // lookupFlag
		0, 0, // subtableCount
	}

	// Patch header offsets.
	scriptOff := len(header)
	featureOff := scriptOff + len(scriptList)
	lookupOff := featureOff + len(featureList)
	binary.BigEndian.PutUint16(header[4:], uint16(scriptOff))
	binary.BigEndian.PutUint16(header[6:], uint16(featureOff))
	binary.BigEndian.PutUint16(header[8:], uint16(lookupOff))

	var out []byte
	out = append(out, header...)
	out = append(out, scriptList...)
	out = append(out, featureList...)
	out = append(out, lookupList...)
	_ = lookupOff
	return out
}

func TestParseMinimalLayoutTable(t *testing.T) {
	data := buildLayoutTable()
	tbl, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(tbl.Scripts) != 1 {
		t.Fatalf("Scripts: got %d, want 1", len(tbl.Scripts))
	}
	if tbl.Scripts[0].Tag != MakeTag("latn") {
		t.Errorf("Scripts[0].Tag: got %s, want latn", tbl.Scripts[0].Tag)
	}
	if tbl.Scripts[0].DefaultLang == nil {
		t.Fatal("Scripts[0].DefaultLang is nil")
	}
	if got := tbl.Scripts[0].DefaultLang.RequiredFeatureIndex; got != 0xffff {
		t.Errorf("RequiredFeatureIndex: got %#x, want 0xffff", got)
	}
	if got := tbl.Scripts[0].DefaultLang.FeatureIndexes; len(got) != 1 || got[0] != 0 {
		t.Errorf("FeatureIndexes: got %v, want [0]", got)
	}
	if len(tbl.Features) != 1 {
		t.Fatalf("Features: got %d, want 1", len(tbl.Features))
	}
	if tbl.Features[0].Tag != MakeTag("kern") {
		t.Errorf("Features[0].Tag: got %s, want kern", tbl.Features[0].Tag)
	}
	if got := tbl.Features[0].LookupIndices; len(got) != 1 || got[0] != 0 {
		t.Errorf("LookupIndices: got %v, want [0]", got)
	}
	if len(tbl.Lookups) != 1 {
		t.Fatalf("Lookups: got %d, want 1", len(tbl.Lookups))
	}
	if tbl.Lookups[0].Type != 2 {
		t.Errorf("Lookups[0].Type: got %d, want 2", tbl.Lookups[0].Type)
	}

	// FindLanguage for (latn, dflt) should return the DefaultLang.
	ls := tbl.FindLanguage(MakeTag("latn"), MakeTag("dflt"))
	if ls == nil || ls.RequiredFeatureIndex != 0xffff {
		t.Errorf("FindLanguage(latn, dflt): got %+v, want DefaultLang", ls)
	}

	// FindLanguage for (grek, dflt) should fall back to nil (no DFLT
	// script provided).
	if ls := tbl.FindLanguage(MakeTag("grek"), MakeTag("dflt")); ls != nil {
		t.Errorf("FindLanguage(grek, dflt): got %+v, want nil", ls)
	}
}
