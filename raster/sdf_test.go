// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package raster

import "testing"

// TestSDFSquare checks that a filled rectangle produces a field where
// interior pixels have values > 128 (inside), exterior pixels have
// values < 128 (outside), and pixels near the edge have values close
// to 128.
func TestSDFSquare(t *testing.T) {
	draw := func(r *Rasterizer) {
		r.Start(pt(10, 10))
		r.Add1(pt(30, 10))
		r.Add1(pt(30, 30))
		r.Add1(pt(10, 30))
		r.Add1(pt(10, 10))
	}
	img := RenderSDF(draw, 40, 40, SDFConfig{Spread: 4})

	// Center of the rectangle: well inside, value close to 255.
	center := img.Pix[20*img.Stride+20]
	if center < 250 {
		t.Errorf("center pixel: got %d, want >= 250 (deep inside)", center)
	}
	// Far outside: near 0.
	corner := img.Pix[0*img.Stride+0]
	if corner > 5 {
		t.Errorf("corner pixel: got %d, want <= 5 (deep outside)", corner)
	}
	// Just inside the top-left edge (~1 pixel in from the corner): should be
	// slightly above 128.
	edgeIn := img.Pix[11*img.Stride+11]
	if edgeIn < 140 || edgeIn > 200 {
		t.Errorf("just-inside edge pixel: got %d, want in (140, 200)", edgeIn)
	}
	// Just outside the edge: below 128.
	edgeOut := img.Pix[9*img.Stride+9]
	if edgeOut > 120 {
		t.Errorf("just-outside edge pixel: got %d, want < 120", edgeOut)
	}
}

// TestSDFSpreadClamping verifies the output clamps to 0 / 255 outside
// the spread window.
func TestSDFSpreadClamping(t *testing.T) {
	draw := func(r *Rasterizer) {
		r.Start(pt(20, 20))
		r.Add1(pt(40, 20))
		r.Add1(pt(40, 40))
		r.Add1(pt(20, 40))
		r.Add1(pt(20, 20))
	}
	img := RenderSDF(draw, 60, 60, SDFConfig{Spread: 2})
	// Pixel 10 units outside the rect: deep negative, clamped to 0.
	if got := img.Pix[5*img.Stride+5]; got != 0 {
		t.Errorf("far-outside pixel: got %d, want 0 (clamped)", got)
	}
	// Center of rect: deep inside, clamped to 255.
	if got := img.Pix[30*img.Stride+30]; got != 255 {
		t.Errorf("far-inside pixel: got %d, want 255 (clamped)", got)
	}
}
