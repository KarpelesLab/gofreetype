// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package cff

import "testing"

func TestParseCharsetFormat0(t *testing.T) {
	// Prefix with 4 filler bytes so offset 4 != predefined-charset sentinel.
	data := []byte{
		0xDE, 0xAD, 0xBE, 0xEF, // filler so charset starts at offset 4
		0,                                  // format
		0x00, 0x64, 0x00, 0xC8, 0x01, 0x2C, // SIDs for glyphs 1..3
	}
	sids, err := parseCharset(data, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []uint16{0, 100, 200, 300}
	for i, w := range want {
		if sids[i] != w {
			t.Errorf("sids[%d]: got %d, want %d", i, sids[i], w)
		}
	}
}

func TestParseCharsetFormat1(t *testing.T) {
	data := []byte{
		0xDE, 0xAD, 0xBE, 0xEF,
		1,                // format
		0x00, 0x0A, 0x02, // first=10, nLeft=2
		0x00, 0x32, 0x00, // first=50, nLeft=0
	}
	sids, err := parseCharset(data, 4, 5)
	if err != nil {
		t.Fatal(err)
	}
	want := []uint16{0, 10, 11, 12, 50}
	for i, w := range want {
		if sids[i] != w {
			t.Errorf("sids[%d]: got %d, want %d", i, sids[i], w)
		}
	}
}

func TestParseCharsetPredefined(t *testing.T) {
	// Offset 0 = ISO-Adobe predefined; identity mapping.
	sids, err := parseCharset(nil, 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	for i, sid := range sids {
		if sid != uint16(i) {
			t.Errorf("ISO-Adobe sids[%d]: got %d, want %d", i, sid, i)
		}
	}
}

func TestGlyphNameStandardStrings(t *testing.T) {
	f := &Font{
		NumGlyphs: 3,
		charset:   []uint16{0, 1, 5}, // .notdef, space, dollar
	}
	cases := []struct {
		gid  int
		want string
	}{
		{0, ".notdef"},
		{1, "space"},
		{2, "dollar"},
		{3, ""},  // out of range
		{-1, ""}, // negative
	}
	for _, tc := range cases {
		if got := f.GlyphName(tc.gid); got != tc.want {
			t.Errorf("GlyphName(%d): got %q, want %q", tc.gid, got, tc.want)
		}
	}
}

func TestGlyphNameCustomStrings(t *testing.T) {
	// Custom SIDs start right after the standard-strings list.
	base := uint16(len(standardStrings))
	f := &Font{
		NumGlyphs: 3,
		charset:   []uint16{0, base, base + 1},
		strings:   [][]byte{[]byte("myglyph"), []byte("anotherglyph")},
	}
	if got := f.GlyphName(1); got != "myglyph" {
		t.Errorf("GlyphName(1): got %q, want myglyph", got)
	}
	if got := f.GlyphName(2); got != "anotherglyph" {
		t.Errorf("GlyphName(2): got %q, want anotherglyph", got)
	}
}

func TestGlyphNameCIDKeyed(t *testing.T) {
	f := &Font{
		NumGlyphs:  2,
		IsCIDKeyed: true,
		charset:    []uint16{0, 1234},
	}
	if got := f.GlyphName(1); got != "cid01234" {
		t.Errorf("GlyphName(1): got %q, want cid01234", got)
	}
}
