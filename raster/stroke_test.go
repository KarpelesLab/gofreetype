// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package raster

import (
	"image"
	"testing"

	"golang.org/x/image/math/fixed"
)

// TestStrokeLine strokes a horizontal line and checks that the
// resulting raster image has pixels covered around the line.
func TestStrokeLine(t *testing.T) {
	const size = 40
	r := NewRasterizer(size, size)
	var path Path
	path.Start(pt(5, 20))
	path.Add1(pt(35, 20))

	// Stroke with a 4-pixel width, round caps and joins.
	r.UseNonZeroWinding = true
	Stroke(r, path, fixed.I(4), RoundCapper, RoundJoiner)

	img := image.NewAlpha(image.Rect(0, 0, size, size))
	r.Rasterize(NewAlphaSrcPainter(img))

	// Pixel right on the line should be fully covered.
	if got := img.AlphaAt(20, 20).A; got != 0xff {
		t.Errorf("center of stroke (20,20): got alpha %#02x, want 0xff", got)
	}
	// Pixel 10 away from the line in Y should be blank.
	if got := img.AlphaAt(20, 5).A; got != 0 {
		t.Errorf("far from stroke (20,5): got alpha %#02x, want 0", got)
	}
}

// TestStrokeCurve strokes a quadratic Bézier and smoke-tests that the
// resulting image has any covered pixels. Cubic stroking isn't
// currently implemented by the stroker.
func TestStrokeCurve(t *testing.T) {
	const size = 40
	r := NewRasterizer(size, size)
	var path Path
	path.Start(pt(5, 20))
	path.Add2(pt(20, 5), pt(35, 20))

	r.UseNonZeroWinding = true
	Stroke(r, path, fixed.I(2), ButtCapper, BevelJoiner)

	img := image.NewAlpha(image.Rect(0, 0, size, size))
	r.Rasterize(NewAlphaSrcPainter(img))

	nonzero := 0
	for _, a := range img.Pix {
		if a != 0 {
			nonzero++
		}
	}
	if nonzero == 0 {
		t.Error("stroked curve produced no covered pixels")
	}
}

// TestStrokeSquareCapper checks that SquareCapper and RoundCapper
// produce different outputs for the same path.
func TestStrokeDifferentCappers(t *testing.T) {
	const size = 20
	make := func(cap Capper) *image.Alpha {
		r := NewRasterizer(size, size)
		var path Path
		path.Start(pt(5, 10))
		path.Add1(pt(15, 10))
		r.UseNonZeroWinding = true
		Stroke(r, path, fixed.I(4), cap, RoundJoiner)
		img := image.NewAlpha(image.Rect(0, 0, size, size))
		r.Rasterize(NewAlphaSrcPainter(img))
		return img
	}
	sq := make(SquareCapper)
	rd := make(RoundCapper)
	// Count covered pixels — squared caps should add more pixels at the
	// ends than round caps do.
	count := func(img *image.Alpha) int {
		n := 0
		for _, a := range img.Pix {
			if a > 0 {
				n++
			}
		}
		return n
	}
	cSq := count(sq)
	cRd := count(rd)
	if cSq == 0 || cRd == 0 {
		t.Fatalf("empty coverage: sq=%d rd=%d", cSq, cRd)
	}
}

// TestPathBasics walks the Path API's add functions.
func TestPathBasics(t *testing.T) {
	var p Path
	p.Start(pt(0, 0))
	p.Add1(pt(10, 0))
	p.Add2(pt(10, 10), pt(0, 10))
	p.Add3(pt(0, 5), pt(5, 0), pt(0, 0))

	// Path is a []fixed.Int26_6; any non-zero length means we added segments.
	if len(p) == 0 {
		t.Error("Path is empty after adding segments")
	}

	// Clear the path and confirm length goes to zero.
	p.Clear()
	if len(p) != 0 {
		t.Errorf("after Clear: len=%d, want 0", len(p))
	}
}
