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

// pt returns a fixed.Point26_6 at integer pixel coordinates (x, y).
func pt(x, y int) fixed.Point26_6 {
	return fixed.Point26_6{X: fixed.Int26_6(x << 6), Y: fixed.Int26_6(y << 6)}
}

// TestRasterizeFilledSquare draws a 10x10 rectangle and checks that the
// interior pixels are fully covered (alpha 0xff) and the exterior pixels
// are fully uncovered (alpha 0x00).
func TestRasterizeFilledSquare(t *testing.T) {
	const size = 20
	r := NewRasterizer(size, size)
	r.Start(pt(5, 5))
	r.Add1(pt(15, 5))
	r.Add1(pt(15, 15))
	r.Add1(pt(5, 15))
	r.Add1(pt(5, 5))

	img := image.NewAlpha(image.Rect(0, 0, size, size))
	r.Rasterize(NewAlphaSrcPainter(img))

	// Strictly interior pixel: must be 0xff.
	if got := img.AlphaAt(10, 10).A; got != 0xff {
		t.Errorf("interior alpha at (10,10): got %#02x, want 0xff", got)
	}
	// Strictly exterior pixel: must be 0x00.
	if got := img.AlphaAt(2, 2).A; got != 0x00 {
		t.Errorf("exterior alpha at (2,2): got %#02x, want 0x00", got)
	}
	if got := img.AlphaAt(18, 18).A; got != 0x00 {
		t.Errorf("exterior alpha at (18,18): got %#02x, want 0x00", got)
	}
}

// TestRasterizeTriangle covers the quadratic-free triangle case and checks
// coverage plausibility along a diagonal.
func TestRasterizeTriangle(t *testing.T) {
	const size = 20
	r := NewRasterizer(size, size)
	r.Start(pt(0, 19))
	r.Add1(pt(19, 19))
	r.Add1(pt(19, 0))
	r.Add1(pt(0, 19))

	img := image.NewAlpha(image.Rect(0, 0, size, size))
	r.Rasterize(NewAlphaSrcPainter(img))

	// The bottom-right corner is well inside the triangle.
	if got := img.AlphaAt(17, 18).A; got != 0xff {
		t.Errorf("interior alpha at (17,18): got %#02x, want 0xff", got)
	}
	// The top-left corner is outside the triangle.
	if got := img.AlphaAt(0, 0).A; got != 0x00 {
		t.Errorf("exterior alpha at (0,0): got %#02x, want 0x00", got)
	}
}

// TestRasterizerClear verifies that Clear wipes accumulated state so that
// re-rasterizing a smaller shape does not leak fill from the prior run.
func TestRasterizerClear(t *testing.T) {
	const size = 20
	r := NewRasterizer(size, size)

	r.Start(pt(0, 0))
	r.Add1(pt(20, 0))
	r.Add1(pt(20, 20))
	r.Add1(pt(0, 20))
	r.Add1(pt(0, 0))

	r.Clear()

	r.Start(pt(2, 2))
	r.Add1(pt(6, 2))
	r.Add1(pt(6, 6))
	r.Add1(pt(2, 6))
	r.Add1(pt(2, 2))

	img := image.NewAlpha(image.Rect(0, 0, size, size))
	r.Rasterize(NewAlphaSrcPainter(img))

	if got := img.AlphaAt(4, 4).A; got != 0xff {
		t.Errorf("interior alpha at (4,4): got %#02x, want 0xff after Clear", got)
	}
	if got := img.AlphaAt(15, 15).A; got != 0x00 {
		t.Errorf("alpha at (15,15): got %#02x, want 0x00 (old rect should have been cleared)", got)
	}
}
