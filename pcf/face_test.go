// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package pcf

import (
	"image"
	"image/draw"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// TestFaceDrawsViaDrawer sanity-checks the full PCF -> font.Drawer path
// by rendering 'A' onto a white canvas via the synthetic 3x3 square font
// built in pcf_test.go.
func TestFaceDrawsViaDrawer(t *testing.T) {
	f, err := Parse(buildMinimalPCF())
	if err != nil {
		t.Fatal(err)
	}
	face := NewFace(f)

	dst := image.NewRGBA(image.Rect(0, 0, 20, 20))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.Black,
		Face: face,
		Dot:  fixed.P(5, 10),
	}
	d.DrawString("A")

	nonWhite := 0
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
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

func TestFaceGlyphAdvance(t *testing.T) {
	f, err := Parse(buildMinimalPCF())
	if err != nil {
		t.Fatal(err)
	}
	face := NewFace(f)
	adv, ok := face.GlyphAdvance('A')
	if !ok {
		t.Fatal("GlyphAdvance('A') ok=false")
	}
	if adv != fixed.I(4) {
		t.Errorf("advance: got %v, want %v", adv, fixed.I(4))
	}
	if _, ok := face.GlyphAdvance('Z'); ok {
		t.Error("GlyphAdvance('Z') ok=true for missing glyph")
	}
}
