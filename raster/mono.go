// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package raster

import "image"

// Bitmap is a 1-bit packed bitmap. Each row is Stride bytes wide and
// each byte holds 8 horizontally adjacent pixels with the leftmost pixel
// in the high bit (MSB-first, matching the on-disk bitmap conventions
// used by BDF, PCF, and the OpenType EBDT/CBDT tables).
//
// Bitmap intentionally has a slim API rather than implementing
// image/draw.Image, since 1-bit drawing is almost always done via
// BitmapPainter (from the rasterizer) or by the caller manually.
type Bitmap struct {
	Pix    []byte
	Stride int
	Rect   image.Rectangle
}

// NewBitmap allocates a Bitmap covering r with Stride = ceil(w/8) bytes.
func NewBitmap(r image.Rectangle) *Bitmap {
	w := r.Dx()
	if w <= 0 {
		w = 1
	}
	stride := (w + 7) / 8
	return &Bitmap{
		Pix:    make([]byte, stride*r.Dy()),
		Stride: stride,
		Rect:   r,
	}
}

// Bounds returns the bitmap's bounds.
func (b *Bitmap) Bounds() image.Rectangle { return b.Rect }

// BitAt returns whether the pixel at (x, y) is set.
func (b *Bitmap) BitAt(x, y int) bool {
	if !(image.Point{x, y}).In(b.Rect) {
		return false
	}
	x -= b.Rect.Min.X
	y -= b.Rect.Min.Y
	return b.Pix[y*b.Stride+x/8]&(1<<(7-uint(x%8))) != 0
}

// SetBit sets (or clears, if v is false) the pixel at (x, y).
func (b *Bitmap) SetBit(x, y int, v bool) {
	if !(image.Point{x, y}).In(b.Rect) {
		return
	}
	x -= b.Rect.Min.X
	y -= b.Rect.Min.Y
	idx := y*b.Stride + x/8
	mask := byte(1 << (7 - uint(x%8)))
	if v {
		b.Pix[idx] |= mask
	} else {
		b.Pix[idx] &^= mask
	}
}

// BitmapPainter paints rasterized spans onto a Bitmap, setting a pixel
// iff its accumulated alpha coverage is at or above 50%.
type BitmapPainter struct {
	B *Bitmap
}

// Paint satisfies the Painter interface.
func (p BitmapPainter) Paint(ss []Span, done bool) {
	b := p.B
	for _, s := range ss {
		if s.Alpha < 0x8000 {
			continue
		}
		if s.Y < b.Rect.Min.Y || s.Y >= b.Rect.Max.Y {
			continue
		}
		x0, x1 := s.X0, s.X1
		if x0 < b.Rect.Min.X {
			x0 = b.Rect.Min.X
		}
		if x1 > b.Rect.Max.X {
			x1 = b.Rect.Max.X
		}
		if x0 >= x1 {
			continue
		}
		y := s.Y - b.Rect.Min.Y
		row := b.Pix[y*b.Stride:]
		for x := x0; x < x1; x++ {
			col := x - b.Rect.Min.X
			row[col/8] |= 1 << (7 - uint(col%8))
		}
	}
}

// NewBitmapPainter returns a BitmapPainter that writes to b.
func NewBitmapPainter(b *Bitmap) BitmapPainter {
	return BitmapPainter{B: b}
}
