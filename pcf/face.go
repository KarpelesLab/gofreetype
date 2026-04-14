// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package pcf

import (
	"image"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// NewFace returns a font.Face over a PCF Font. See bdf.NewFace for the
// design notes; this is the compiled-binary counterpart.
//
// The returned Face is not safe for concurrent use.
func NewFace(f *Font) font.Face {
	return &face{f: f}
}

type face struct {
	f *Font
}

func (*face) Close() error { return nil }

func (a *face) Metrics() font.Metrics {
	ascent := fixed.I(a.f.FontAscent)
	descent := fixed.I(a.f.FontDescent)
	return font.Metrics{
		Height:  ascent + descent,
		Ascent:  ascent,
		Descent: descent,
	}
}

func (*face) Kern(r0, r1 rune) fixed.Int26_6 { return 0 }

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

func (a *face) GlyphAdvance(r rune) (advance fixed.Int26_6, ok bool) {
	g := a.f.Glyph(r)
	if g == nil {
		return 0, false
	}
	return fixed.I(g.Advance), true
}
