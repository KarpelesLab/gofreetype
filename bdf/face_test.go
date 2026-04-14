// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package bdf

import (
	"image"
	"image/draw"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

func TestFaceDrawsViaDrawer(t *testing.T) {
	f, err := Parse([]byte(miniBDF))
	if err != nil {
		t.Fatal(err)
	}
	face := NewFace(f)

	// Metrics look right.
	m := face.Metrics()
	if m.Ascent == 0 || m.Height == 0 {
		t.Errorf("Metrics: got %+v, want non-zero values", m)
	}

	// Full round-trip: draw "A" via font.Drawer onto an RGBA canvas and
	// check that some pixels are actually set.
	dst := image.NewRGBA(image.Rect(0, 0, 40, 20))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.Black,
		Face: face,
		Dot:  fixed.P(2, 10),
	}
	d.DrawString("A")

	nonWhite := 0
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			r, g, b, _ := dst.At(x, y).RGBA()
			if r < 0xff00 || g < 0xff00 || b < 0xff00 {
				nonWhite++
			}
		}
	}
	if nonWhite == 0 {
		t.Error("font.Drawer drew no non-white pixels for 'A'")
	}
}

func TestFaceGlyphAdvanceAndBounds(t *testing.T) {
	f, err := Parse([]byte(miniBDF))
	if err != nil {
		t.Fatal(err)
	}
	face := NewFace(f)

	adv, ok := face.GlyphAdvance('A')
	if !ok {
		t.Fatal("GlyphAdvance('A') ok=false")
	}
	if adv != fixed.I(6) { // DWIDTH=6 in miniBDF
		t.Errorf("advance: got %v, want %v", adv, fixed.I(6))
	}
	bounds, _, ok := face.GlyphBounds('A')
	if !ok {
		t.Fatal("GlyphBounds('A') ok=false")
	}
	if bounds.Min.X == bounds.Max.X || bounds.Min.Y == bounds.Max.Y {
		t.Errorf("bounds: got %+v, want non-degenerate", bounds)
	}
}

func TestFaceUnknownGlyph(t *testing.T) {
	f, err := Parse([]byte(miniBDF))
	if err != nil {
		t.Fatal(err)
	}
	face := NewFace(f)
	if _, _, ok := face.GlyphBounds('Z'); ok {
		t.Error("GlyphBounds('Z') ok=true for missing glyph")
	}
	if _, ok := face.GlyphAdvance('Z'); ok {
		t.Error("GlyphAdvance('Z') ok=true for missing glyph")
	}
}
