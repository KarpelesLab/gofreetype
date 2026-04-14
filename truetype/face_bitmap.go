// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import (
	"bytes"
	"image"
	"image/png"
	_ "image/jpeg" // registers JPEG decoder for sbix "jpg " graphic type

	"golang.org/x/image/math/fixed"
)

// BitmapGlyph renders a pre-rasterized bitmap glyph (from sbix, CBDT, or
// another PNG-carrying table) into an RGBA image at the target pixel
// size. Returns ok=false when the glyph has no bitmap representation;
// the caller can fall back to face.Glyph (or face.ColorGlyph) in that
// case.
//
// The returned image has already been decoded from PNG (or JPEG, for
// sbix "jpg " entries) and is positioned such that the baseline sits at
// the dot's pixel coordinate, after applying the per-strike origin
// offsets.
func (a *face) BitmapGlyph(dot fixed.Point26_6, r rune) (
	dr image.Rectangle, rgba image.Image, maskp image.Point, advance fixed.Int26_6, ok bool) {

	index := a.index(r)

	// Pick the ppem closest to our pixel size (scale is 26.6 pixels per em;
	// scale >> 6 = pixels per em).
	ppem := uint16(a.scale >> 6)

	// Try sbix first — Apple fonts. Each strike is PNG/JPEG bytes.
	if sb := a.f.Sbix(); sb != nil {
		if strike := sb.FindStrike(ppem); strike != nil {
			if g := strike.Glyph(int(index)); g != nil && len(g.Data) > 0 {
				if img := decodeImage(g.Data, g.GraphicType); img != nil {
					return placeBitmap(dot, img, int(g.OriginOffsetX), int(g.OriginOffsetY),
						a.f.UnscaledHMetric(index).AdvanceWidth)
				}
			}
		}
	}

	// Then CBDT/CBLC — Google emoji fonts.
	if cb := a.f.CBLC(); cb != nil {
		if set := cb.FindSet(uint8(ppem)); set != nil {
			if g := set.Glyph(uint16(index)); g != nil && len(g.Data) > 0 {
				// CBDT format 17/18/19 — PNG data.
				if img := decodePNG(g.Data); img != nil {
					return placeBitmap(dot, img, int(g.BearingX), int(g.BearingY),
						fixed.Int26_6(g.Advance)<<6)
				}
			}
		}
	}

	return image.Rectangle{}, nil, image.Point{}, 0, false
}

// placeBitmap positions a decoded image at the glyph's dot + origin
// offset and returns the standard face.Glyph 5-tuple.
func placeBitmap(dot fixed.Point26_6, img image.Image, originX, originY int, advance fixed.Int26_6) (
	image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	ix := int(dot.X >> 6)
	iy := int(dot.Y >> 6)
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	// Origin in sbix / CBDT specifies the bottom-left offset of the bitmap
	// relative to the glyph's pen position. In pixel space with Y-down,
	// that maps to: the top of the image sits at (dot + originX, dot -
	// (originY + h)).
	x0 := ix + originX
	y0 := iy - originY - h
	dr := image.Rect(x0, y0, x0+w, y0+h)
	maskp := img.Bounds().Min
	return dr, img, maskp, advance, true
}

// decodeImage picks the right decoder based on the sbix graphicType tag.
func decodeImage(data []byte, gtype [4]byte) image.Image {
	switch gtype {
	case [4]byte{'p', 'n', 'g', ' '}:
		return decodePNG(data)
	case [4]byte{'j', 'p', 'g', ' '}:
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil
		}
		return img
	}
	return nil
}

func decodePNG(data []byte) image.Image {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return img
}
