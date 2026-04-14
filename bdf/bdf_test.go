// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package bdf

import "testing"

// A hand-crafted minimal BDF font with one glyph (the letter A), a 5x7
// bitmap. Header fields taken from the X11 6x12 font style.
const miniBDF = `STARTFONT 2.1
FONT -misc-test-medium-r-normal--7-70-75-75-c-50-iso10646-1
SIZE 7 75 75
FONTBOUNDINGBOX 5 7 0 -1
STARTPROPERTIES 4
PIXEL_SIZE 7
POINT_SIZE 70
FONT_ASCENT 6
FONT_DESCENT 1
ENDPROPERTIES
CHARS 1
STARTCHAR A
ENCODING 65
SWIDTH 571 0
DWIDTH 6 0
BBX 5 7 0 -1
BITMAP
70
88
88
F8
88
88
88
ENDCHAR
ENDFONT
`

func TestParseMinimalBDF(t *testing.T) {
	f, err := Parse([]byte(miniBDF))
	if err != nil {
		t.Fatal(err)
	}
	if f.PixelSize != 7 {
		t.Errorf("PixelSize: got %d, want 7", f.PixelSize)
	}
	if f.FontAscent != 6 || f.FontDescent != 1 {
		t.Errorf("ascent/descent: got %d/%d, want 6/1", f.FontAscent, f.FontDescent)
	}
	if f.BoundingBoxX != 5 || f.BoundingBoxY != 7 {
		t.Errorf("bounding box: got %dx%d, want 5x7", f.BoundingBoxX, f.BoundingBoxY)
	}

	g := f.Glyph('A')
	if g == nil {
		t.Fatal("Glyph('A') is nil")
	}
	if g.Advance != 6 {
		t.Errorf("advance: got %d, want 6", g.Advance)
	}
	if g.BBX != 5 || g.BBY != 7 {
		t.Errorf("glyph BBX/BBY: got %dx%d, want 5x7", g.BBX, g.BBY)
	}
	if g.Bitmap == nil {
		t.Fatal("Bitmap is nil")
	}

	// Row 0 = 0x70 = 01110000 → pixels at (1,0), (2,0), (3,0) set, rest clear.
	if !g.Bitmap.BitAt(1, 0) || !g.Bitmap.BitAt(2, 0) || !g.Bitmap.BitAt(3, 0) {
		t.Error("row 0 pixels (1,0)-(3,0) should be set")
	}
	if g.Bitmap.BitAt(0, 0) || g.Bitmap.BitAt(4, 0) {
		t.Error("row 0 pixels (0,0) and (4,0) should be clear")
	}
	// Row 3 = 0xF8 = 11111000 → all 5 pixels set.
	for x := 0; x < 5; x++ {
		if !g.Bitmap.BitAt(x, 3) {
			t.Errorf("row 3 pixel (%d,3) should be set (0xF8)", x)
		}
	}
	// Row 1 = 0x88 = 10001000 → (0,1) and (4,1) set.
	if !g.Bitmap.BitAt(0, 1) || !g.Bitmap.BitAt(4, 1) {
		t.Error("row 1 pixels (0,1) and (4,1) should be set")
	}
	if g.Bitmap.BitAt(1, 1) || g.Bitmap.BitAt(2, 1) || g.Bitmap.BitAt(3, 1) {
		t.Error("row 1 pixels (1,1)-(3,1) should be clear")
	}
}

// TestParseBDFUnknownGlyph verifies that asking for an unmapped codepoint
// returns nil.
func TestParseBDFUnknownGlyph(t *testing.T) {
	f, err := Parse([]byte(miniBDF))
	if err != nil {
		t.Fatal(err)
	}
	if g := f.Glyph('Z'); g != nil {
		t.Errorf("Glyph('Z') should be nil, got %+v", g)
	}
}
