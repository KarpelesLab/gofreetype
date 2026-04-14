// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package bdf

import (
	"image"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// NewFace returns a font.Face wrapping a BDF Font so callers can feed
// it directly into font.Drawer and friends. The face renders glyphs at
// their native BDF pixel size; unlike vector faces there is no size
// scaling, so a BDF 7-pixel font always renders at 7 pixels regardless
// of what the Drawer's Face thinks.
//
// The returned Face is not safe for concurrent use.
func NewFace(f *Font) font.Face {
	return &face{f: f}
}

type face struct {
	f *Font
}

func (*face) Close() error { return nil }

// Metrics satisfies font.Face. Values are reported in 26.6 fixed pixels.
func (a *face) Metrics() font.Metrics {
	ascent := fixed.I(a.f.FontAscent)
	descent := fixed.I(a.f.FontDescent)
	return font.Metrics{
		Height:  ascent + descent,
		Ascent:  ascent,
		Descent: descent,
	}
}

// Kern returns zero — bitmap fonts do not carry kern pairs.
func (*face) Kern(r0, r1 rune) fixed.Int26_6 { return 0 }

// Glyph satisfies font.Face. The returned image is the glyph's bitmap
// re-interpreted as an *image.Alpha (0xff or 0x00 per pixel) so
// draw.DrawMask handles it the same as a vector-rasterized glyph.
func (a *face) Glyph(dot fixed.Point26_6, r rune) (
	dr image.Rectangle, mask image.Image, maskp image.Point,
	advance fixed.Int26_6, ok bool) {

	g := a.f.Glyph(r)
	if g == nil || g.Bitmap == nil {
		return image.Rectangle{}, nil, image.Point{}, 0, false
	}

	ix, iy := int(dot.X>>6), int(dot.Y>>6)
	x0 := ix + g.BBOx
	y0 := iy - (g.BBY + g.BBOy)
	dr = image.Rect(x0, y0, x0+g.BBX, y0+g.BBY)

	// Convert the 1-bit bitmap to an 8-bit alpha mask with the same
	// bounds; this is what draw.DrawMask expects.
	alpha := image.NewAlpha(image.Rect(0, 0, g.BBX, g.BBY))
	for y := 0; y < g.BBY; y++ {
		for x := 0; x < g.BBX; x++ {
			if g.Bitmap.BitAt(x, y) {
				alpha.Pix[y*alpha.Stride+x] = 0xff
			}
		}
	}
	return dr, alpha, image.Point{}, fixed.I(g.Advance), true
}

// GlyphBounds returns the bounding rectangle of the glyph relative to
// the origin.
func (a *face) GlyphBounds(r rune) (bounds fixed.Rectangle26_6, advance fixed.Int26_6, ok bool) {
	g := a.f.Glyph(r)
	if g == nil {
		return fixed.Rectangle26_6{}, 0, false
	}
	return fixed.Rectangle26_6{
		Min: fixed.Point26_6{X: fixed.I(g.BBOx), Y: fixed.I(-g.BBY - g.BBOy)},
		Max: fixed.Point26_6{X: fixed.I(g.BBOx + g.BBX), Y: fixed.I(-g.BBOy)},
	}, fixed.I(g.Advance), true
}

// GlyphAdvance returns the glyph's advance width in 26.6 pixels.
func (a *face) GlyphAdvance(r rune) (advance fixed.Int26_6, ok bool) {
	g := a.f.Glyph(r)
	if g == nil {
		return 0, false
	}
	return fixed.I(g.Advance), true
}
