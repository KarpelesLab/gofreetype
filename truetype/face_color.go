// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import (
	"image"
	imgcolor "image/color"

	"github.com/KarpelesLab/gofreetype/raster"
	"golang.org/x/image/math/fixed"
)

// ColorGlyph renders a COLR v0 color glyph into an RGBA image by
// rasterizing each layer's outline, applying the named palette entry as
// the fill color, and compositing the layers bottom-to-top with source-
// over blending.
//
// If the glyph r has no color layers in the font's COLR table, ColorGlyph
// returns ok=false; the caller should fall back to face.Glyph in that
// case. If the font lacks a COLR or CPAL table, ok=false is returned
// immediately.
//
// The paletteIdx argument selects which CPAL palette to use (0 is almost
// always the right choice). The foreground argument is used when a layer
// names palette index 0xFFFF ("use current text color"); pass nil for
// the conventional opaque-black default.
//
// Returns (dr, rgba, maskp, advance, ok) mirroring face.Glyph — dr is
// the destination rectangle in pixel space, rgba is the image to draw
// over dst through draw.Draw (NOT DrawMask, since the image already
// carries color), maskp is the origin of the glyph within rgba, and
// advance is the glyph's advance width in 26.6 pixels.
func (a *face) ColorGlyph(dot fixed.Point26_6, r rune, paletteIdx int, foreground imgcolor.Color) (
	dr image.Rectangle, rgba *image.RGBA, maskp image.Point, advance fixed.Int26_6, ok bool) {

	colr := a.f.COLR()
	cpal := a.f.CPAL()
	if colr == nil || cpal == nil {
		return image.Rectangle{}, nil, image.Point{}, 0, false
	}

	index := a.index(r)
	layers := colr.ColorLayers(uint16(index))
	if len(layers) == 0 {
		return image.Rectangle{}, nil, image.Point{}, 0, false
	}

	// Pull the requested palette. Clamp to a valid index; if absent, use
	// the first palette.
	var palette []imgcolor.NRGBA
	if paletteIdx >= 0 && paletteIdx < len(cpal.Palettes) {
		palette = cpal.Palettes[paletteIdx].Colors
	} else if len(cpal.Palettes) > 0 {
		palette = cpal.Palettes[0].Colors
	}

	fg := imgcolor.NRGBA{A: 0xff}
	if foreground != nil {
		r, g, b, alpha := foreground.RGBA()
		if alpha > 0 {
			fg = imgcolor.NRGBA{
				R: uint8(r * 0xff / alpha),
				G: uint8(g * 0xff / alpha),
				B: uint8(b * 0xff / alpha),
				A: uint8(alpha >> 8),
			}
		}
	}

	// Quantize to subpixel granularity as in face.Glyph.
	dotX := (dot.X + a.subPixelBiasX) & a.subPixelMaskX
	dotY := (dot.Y + a.subPixelBiasY) & a.subPixelMaskY
	ix, fx := int(dotX>>6), dotX&0x3f
	iy, fy := int(dotY>>6), dotY&0x3f

	// Determine the composed bounds by loading the base glyph. We'll
	// enlarge if a layer turns out to need more room, but this is almost
	// always the tight answer.
	if err := a.glyphBuf.Load(a.f, a.scale, index, a.hinting); err != nil {
		return image.Rectangle{}, nil, image.Point{}, 0, false
	}
	xmin := int(fx+a.glyphBuf.Bounds.Min.X) >> 6
	ymin := int(fy-a.glyphBuf.Bounds.Max.Y) >> 6
	xmax := int(fx+a.glyphBuf.Bounds.Max.X+0x3f) >> 6
	ymax := int(fy-a.glyphBuf.Bounds.Min.Y+0x3f) >> 6
	advance = a.glyphBuf.AdvanceWidth

	// Compute per-layer bounds too so we don't clip.
	for _, l := range layers {
		if err := a.glyphBuf.Load(a.f, a.scale, Index(l.GlyphID), a.hinting); err != nil {
			continue
		}
		lxmin := int(fx+a.glyphBuf.Bounds.Min.X) >> 6
		lymin := int(fy-a.glyphBuf.Bounds.Max.Y) >> 6
		lxmax := int(fx+a.glyphBuf.Bounds.Max.X+0x3f) >> 6
		lymax := int(fy-a.glyphBuf.Bounds.Min.Y+0x3f) >> 6
		if lxmin < xmin {
			xmin = lxmin
		}
		if lymin < ymin {
			ymin = lymin
		}
		if lxmax > xmax {
			xmax = lxmax
		}
		if lymax > ymax {
			ymax = lymax
		}
	}

	w, h := xmax-xmin, ymax-ymin
	if w <= 0 || h <= 0 {
		return image.Rectangle{}, nil, image.Point{}, 0, false
	}
	rgba = image.NewRGBA(image.Rect(0, 0, w, h))

	// Rasterize each layer with its palette color into rgba.
	lr := raster.NewRasterizer(w, h)
	alphaBuf := image.NewAlpha(image.Rect(0, 0, w, h))
	for _, l := range layers {
		if err := a.glyphBuf.Load(a.f, a.scale, Index(l.GlyphID), a.hinting); err != nil {
			continue
		}
		lfx := fx - fixed.Int26_6(xmin<<6)
		lfy := fy - fixed.Int26_6(ymin<<6)

		lr.Clear()
		// Zero alphaBuf between layers.
		for i := range alphaBuf.Pix {
			alphaBuf.Pix[i] = 0
		}
		e0 := 0
		for _, e1 := range a.glyphBuf.Ends {
			a.drawContourAt(lr, a.glyphBuf.Points[e0:e1], lfx, lfy)
			e0 = e1
		}
		lr.Rasterize(raster.NewAlphaSrcPainter(alphaBuf))

		// Choose the fill color.
		var c imgcolor.NRGBA
		if l.PaletteIndex == 0xFFFF {
			c = fg
		} else if int(l.PaletteIndex) < len(palette) {
			c = palette[l.PaletteIndex]
		} else {
			continue
		}

		// Composite with source-over blending.
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				srcA := uint32(alphaBuf.Pix[y*alphaBuf.Stride+x])
				if srcA == 0 {
					continue
				}
				// Premultiply source by (srcA * colorAlpha / 255).
				a8 := srcA * uint32(c.A) / 255
				if a8 == 0 {
					continue
				}
				sr := uint32(c.R) * a8 / 255
				sg := uint32(c.G) * a8 / 255
				sb := uint32(c.B) * a8 / 255
				off := rgba.PixOffset(x, y)
				dr := uint32(rgba.Pix[off])
				dg := uint32(rgba.Pix[off+1])
				db := uint32(rgba.Pix[off+2])
				da := uint32(rgba.Pix[off+3])
				inv := 255 - a8
				rgba.Pix[off] = byte(sr + dr*inv/255)
				rgba.Pix[off+1] = byte(sg + dg*inv/255)
				rgba.Pix[off+2] = byte(sb + db*inv/255)
				rgba.Pix[off+3] = byte(a8 + da*inv/255)
			}
		}
	}

	dr = image.Rect(ix+xmin, iy+ymin, ix+xmax, iy+ymax)
	maskp = image.Point{}
	return dr, rgba, maskp, advance, true
}

// drawContourAt is a free-standing variant of face.drawContour that emits
// onto an arbitrary rasterizer rather than face.r. It's used by
// ColorGlyph to rasterize each layer into a layer-local rasterizer.
func (a *face) drawContourAt(lr *raster.Rasterizer, ps []Point, dx, dy fixed.Int26_6) {
	if len(ps) == 0 {
		return
	}
	start := fixed.Point26_6{X: dx + ps[0].X, Y: dy - ps[0].Y}
	var others []Point
	if ps[0].Flags&0x01 != 0 {
		others = ps[1:]
	} else {
		last := fixed.Point26_6{
			X: dx + ps[len(ps)-1].X,
			Y: dy - ps[len(ps)-1].Y,
		}
		if ps[len(ps)-1].Flags&0x01 != 0 {
			start = last
			others = ps[:len(ps)-1]
		} else {
			start = fixed.Point26_6{
				X: (start.X + last.X) / 2,
				Y: (start.Y + last.Y) / 2,
			}
			others = ps
		}
	}
	lr.Start(start)
	q0, on0 := start, true
	for _, p := range others {
		q := fixed.Point26_6{X: dx + p.X, Y: dy - p.Y}
		on := p.Flags&0x01 != 0
		if on {
			if on0 {
				lr.Add1(q)
			} else {
				lr.Add2(q0, q)
			}
		} else {
			if on0 {
				// No-op.
			} else {
				mid := fixed.Point26_6{
					X: (q0.X + q.X) / 2,
					Y: (q0.Y + q.Y) / 2,
				}
				lr.Add2(q0, mid)
			}
		}
		q0, on0 = q, on
	}
	if on0 {
		lr.Add1(start)
	} else {
		lr.Add2(q0, start)
	}
}
