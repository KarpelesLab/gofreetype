// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package raster

import (
	"image"
	"testing"
)

// TestLCDHorizontalRGB renders a solid rectangle via the LCD pipeline and
// checks that the interior pixels have full R/G/B coverage and the
// exterior pixels do not.
func TestLCDHorizontalRGB(t *testing.T) {
	// Rectangle (4,4)-(16,16) in physical pixels; tripled on X for
	// horizontal oversampling.
	draw := func(r *Rasterizer) {
		r.Start(pt(12, 4))
		r.Add1(pt(48, 4))
		r.Add1(pt(48, 16))
		r.Add1(pt(12, 16))
		r.Add1(pt(12, 4))
	}
	img := RenderLCD(draw, 20, 20, LCDHorizontalRGB)
	if img.Rect != (image.Rect(0, 0, 20, 20)) {
		t.Fatalf("bounds: got %v", img.Rect)
	}
	// Interior pixel: all three channels should be strongly covered.
	off := img.PixOffset(10, 10)
	r, g, b := img.Pix[off], img.Pix[off+1], img.Pix[off+2]
	if r < 0xc0 || g < 0xc0 || b < 0xc0 {
		t.Errorf("interior pixel (10,10) = (%d,%d,%d), want each >= 0xc0", r, g, b)
	}
	// Exterior pixel: each channel should be zero or near-zero.
	off = img.PixOffset(1, 1)
	r, g, b = img.Pix[off], img.Pix[off+1], img.Pix[off+2]
	if r > 4 || g > 4 || b > 4 {
		t.Errorf("exterior pixel (1,1) = (%d,%d,%d), want near zero", r, g, b)
	}
}

// TestLCDOrientationBGRSwap verifies that BGR orientation swaps R and B
// without changing the geometric coverage.
func TestLCDOrientationBGRSwap(t *testing.T) {
	// Horizontal LCD: outline X is tripled.
	draw := func(r *Rasterizer) {
		r.Start(pt(9, 3))
		r.Add1(pt(45, 3))
		r.Add1(pt(45, 15))
		r.Add1(pt(9, 15))
		r.Add1(pt(9, 3))
	}
	rgb := RenderLCD(draw, 18, 18, LCDHorizontalRGB)
	bgr := RenderLCD(draw, 18, 18, LCDHorizontalBGR)

	for y := 0; y < 18; y++ {
		for x := 0; x < 18; x++ {
			rOff := rgb.PixOffset(x, y)
			bOff := bgr.PixOffset(x, y)
			// R↔B swapped, G unchanged, A unchanged.
			if rgb.Pix[rOff] != bgr.Pix[bOff+2] {
				t.Errorf("(%d,%d) RGB.R != BGR.B", x, y)
				return
			}
			if rgb.Pix[rOff+2] != bgr.Pix[bOff] {
				t.Errorf("(%d,%d) RGB.B != BGR.R", x, y)
				return
			}
			if rgb.Pix[rOff+1] != bgr.Pix[bOff+1] {
				t.Errorf("(%d,%d) G channel differs", x, y)
				return
			}
		}
	}
}

// TestLCDVerticalRGB performs a smoke test for the vertical orientation.
func TestLCDVerticalRGB(t *testing.T) {
	// Vertical LCD: outline Y is tripled.
	draw := func(r *Rasterizer) {
		r.Start(pt(3, 9))
		r.Add1(pt(15, 9))
		r.Add1(pt(15, 45))
		r.Add1(pt(3, 45))
		r.Add1(pt(3, 9))
	}
	img := RenderLCD(draw, 18, 18, LCDVerticalRGB)
	off := img.PixOffset(9, 9)
	if img.Pix[off] < 0xc0 || img.Pix[off+1] < 0xc0 || img.Pix[off+2] < 0xc0 {
		t.Errorf("interior pixel dim for vertical orientation: %d %d %d",
			img.Pix[off], img.Pix[off+1], img.Pix[off+2])
	}
}
