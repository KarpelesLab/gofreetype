// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import "testing"

func TestPostLuxiSans(t *testing.T) {
	f, _, err := parseTestdataFont("luxisr")
	if err != nil {
		t.Fatal(err)
	}
	pi := f.Post()
	if pi == nil {
		t.Fatal("Post() is nil for luxisr (which has a post 2.0 table)")
	}
	if pi.Version != postVersion20 {
		t.Errorf("Version: got %#x, want %#x (2.0)", pi.Version, postVersion20)
	}
	if pi.ItalicAngle != 0 {
		t.Errorf("ItalicAngle: got %v, want 0", pi.ItalicAngle)
	}

	// Standard-Mac names: glyph index 36 is 'A', glyph index 57 is 'V' per
	// the existing TestParse checks.
	if got := f.GlyphName(36); got != "A" {
		t.Errorf("GlyphName(36): got %q, want %q", got, "A")
	}
	if got := f.GlyphName('V' - 'A' + 36); got != "V" {
		t.Errorf("GlyphName for V: got %q, want %q", got, "V")
	}
	// .notdef is glyph 0.
	if got := f.GlyphName(0); got != ".notdef" {
		t.Errorf("GlyphName(0): got %q, want %q", got, ".notdef")
	}
	// Out-of-range index returns empty.
	if got := f.GlyphName(Index(f.nGlyph + 10)); got != "" {
		t.Errorf("GlyphName(out-of-range): got %q, want empty", got)
	}
}

// TestPostVersion10 verifies that version 1.0 fonts fall back to the
// full standard-Mac name list for glyphs 0..257.
func TestPostVersion10(t *testing.T) {
	// Build a minimal post 1.0 header.
	post := make([]byte, 32)
	post[0], post[1], post[2], post[3] = 0, 1, 0, 0 // version 1.0
	f := &Font{nGlyph: 10}
	if err := f.parsePost(post); err != nil {
		t.Fatal(err)
	}
	pi := f.Post()
	if pi == nil {
		t.Fatal("Post() nil after parsing 1.0")
	}
	if pi.Version != postVersion10 {
		t.Errorf("Version: got %#x, want %#x", pi.Version, postVersion10)
	}
	if got := f.GlyphName(0); got != ".notdef" {
		t.Errorf("GlyphName(0): got %q, want .notdef", got)
	}
	if got := f.GlyphName(3); got != "space" {
		t.Errorf("GlyphName(3): got %q, want space", got)
	}
}

// TestPostVersion30 verifies that version 3.0 fonts yield empty glyph
// names but still expose the header fields.
func TestPostVersion30(t *testing.T) {
	post := make([]byte, 32)
	post[0], post[1], post[2], post[3] = 0, 3, 0, 0 // version 3.0
	post[8], post[9] = 0xff, 0x70                   // underlinePosition = -144
	post[10], post[11] = 0, 20                      // underlineThickness = 20
	f := &Font{nGlyph: 10}
	if err := f.parsePost(post); err != nil {
		t.Fatal(err)
	}
	pi := f.Post()
	if pi == nil {
		t.Fatal("Post() nil")
	}
	if pi.UnderlinePosition != -144 {
		t.Errorf("UnderlinePosition: got %d, want -144", pi.UnderlinePosition)
	}
	if pi.UnderlineThickness != 20 {
		t.Errorf("UnderlineThickness: got %d, want 20", pi.UnderlineThickness)
	}
	if got := f.GlyphName(0); got != "" {
		t.Errorf("GlyphName(0) on v3.0 font: got %q, want empty", got)
	}
}
