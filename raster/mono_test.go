// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package raster

import (
	"image"
	"testing"
)

func TestBitmapSetGet(t *testing.T) {
	b := NewBitmap(image.Rect(0, 0, 16, 8))
	// Check invariants on the buffer shape.
	if b.Stride != 2 { // 16 pixels / 8 = 2 bytes per row
		t.Errorf("Stride: got %d, want 2", b.Stride)
	}
	if len(b.Pix) != 16 {
		t.Errorf("Pix length: got %d, want 16", len(b.Pix))
	}
	b.SetBit(0, 0, true)
	b.SetBit(7, 0, true)
	b.SetBit(15, 7, true)
	if !b.BitAt(0, 0) || !b.BitAt(7, 0) || !b.BitAt(15, 7) {
		t.Error("bits should be set after SetBit(true)")
	}
	// Leftmost bit of row 0 is high bit of byte 0.
	if b.Pix[0]&0x80 == 0 {
		t.Error("SetBit(0,0) should set the high bit of byte 0")
	}
	// Pixel (7,0) is the low bit of byte 0.
	if b.Pix[0]&0x01 == 0 {
		t.Error("SetBit(7,0) should set the low bit of byte 0")
	}
	// Pixel (15,7) is the low bit of byte 15.
	if b.Pix[15]&0x01 == 0 {
		t.Error("SetBit(15,7) should set the low bit of byte 15")
	}
	b.SetBit(0, 0, false)
	if b.BitAt(0, 0) {
		t.Error("SetBit(0,0,false) should clear the bit")
	}
	// Out-of-bounds queries return false and are no-ops.
	if b.BitAt(-1, 0) || b.BitAt(100, 0) {
		t.Error("BitAt outside bounds should return false")
	}
}

// TestBitmapPainterSquare rasterizes a filled rectangle via the antialiased
// rasterizer, feeds the spans into BitmapPainter, and verifies the interior
// pixels are set and the exterior pixels are not.
func TestBitmapPainterSquare(t *testing.T) {
	const size = 20
	r := NewRasterizer(size, size)
	r.Start(pt(5, 5))
	r.Add1(pt(15, 5))
	r.Add1(pt(15, 15))
	r.Add1(pt(5, 15))
	r.Add1(pt(5, 5))

	b := NewBitmap(image.Rect(0, 0, size, size))
	r.Rasterize(NewBitmapPainter(b))

	if !b.BitAt(10, 10) {
		t.Error("interior pixel (10,10) should be set")
	}
	if b.BitAt(2, 2) {
		t.Error("exterior pixel (2,2) should be clear")
	}
	if b.BitAt(18, 18) {
		t.Error("exterior pixel (18,18) should be clear")
	}
}
