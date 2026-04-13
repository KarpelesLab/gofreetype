// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package raster

import "image"

// LCD subpixel rendering exploits the fact that an LCD pixel is really
// three physical stripes (R, G, B) laid out in a fixed order. By
// independently computing coverage for each stripe, we can triple the
// effective horizontal resolution of antialiased text at the cost of
// having to render into an oversampled intermediate buffer and then
// apply a color-fringing-minimizing FIR filter across each row.
//
// The conventional FreeType filter is a 5-tap (8/77, 24/77, 24/77,
// 16/77, 5/77) on the three stripes plus one neighbor on each side
// (per https://freetype.org/freetype2/docs/reference/ft2-lcd_rendering.html).
// We use the normalized form so the coefficients sum to 256 without
// rollover.

// LCDOrientation selects how the subpixel geometry maps onto physical
// display pixels.
type LCDOrientation int

const (
	// LCDHorizontalRGB: each physical pixel is R|G|B left-to-right.
	// Most common orientation on LCD monitors.
	LCDHorizontalRGB LCDOrientation = iota
	// LCDHorizontalBGR: each physical pixel is B|G|R left-to-right.
	LCDHorizontalBGR
	// LCDVerticalRGB: each physical pixel is R|G|B top-to-bottom.
	LCDVerticalRGB
	// LCDVerticalBGR: each physical pixel is B|G|R top-to-bottom.
	LCDVerticalBGR
)

// LCDFilter coefficients (sum == 256) applied symmetrically around the
// center stripe. Matches FreeType's "default" LCD filter.
var defaultLCDFilter = [5]uint16{16, 40, 144, 40, 16}

// RenderLCD rasterizes an outline at 3x oversampling along the subpixel
// axis and filters the result with the LCD filter to produce an RGBA
// image of (physWidth x physHeight) pixels where each color channel
// carries its stripe's coverage.
//
// outlineFn is called to emit path operations into a Rasterizer. Its
// coordinates must be expressed in the oversampled space — i.e. the
// outline must have its X coordinates tripled for horizontal LCD and
// its Y coordinates tripled for vertical LCD. This matches FreeType's
// convention: the caller scales, the LCD primitive filters.
//
// Callers typically use this via a higher-level face.GlyphLCD method
// that handles the coordinate scaling; this function is the raster-
// level primitive.
func RenderLCD(outlineFn func(r *Rasterizer), physWidth, physHeight int, orient LCDOrientation) *image.RGBA {
	horizontal := orient == LCDHorizontalRGB || orient == LCDHorizontalBGR
	bgr := orient == LCDHorizontalBGR || orient == LCDVerticalBGR

	w, h := physWidth, physHeight
	if horizontal {
		w = physWidth * 3
	} else {
		h = physHeight * 3
	}
	r := NewRasterizer(w, h)
	outlineFn(r)
	buf := image.NewAlpha(image.Rect(0, 0, w, h))
	r.Rasterize(NewAlphaSrcPainter(buf))

	out := image.NewRGBA(image.Rect(0, 0, physWidth, physHeight))
	filter := defaultLCDFilter

	if horizontal {
		for y := 0; y < physHeight; y++ {
			for x := 0; x < physWidth; x++ {
				// Sample 3 stripes centered on the pixel.
				stripeCenter := x * 3
				channels := [3]uint16{}
				for s := 0; s < 3; s++ {
					center := stripeCenter + s
					sum := uint32(0)
					for k := -2; k <= 2; k++ {
						sx := center + k
						if sx < 0 || sx >= w {
							continue
						}
						sum += uint32(buf.Pix[y*buf.Stride+sx]) * uint32(filter[k+2])
					}
					channels[s] = uint16((sum + 128) >> 8)
				}
				r, g, b := channels[0], channels[1], channels[2]
				if bgr {
					r, b = b, r
				}
				off := out.PixOffset(x, y)
				out.Pix[off+0] = byte(r)
				out.Pix[off+1] = byte(g)
				out.Pix[off+2] = byte(b)
				// Alpha: the max of the three channels so composition
				// over a colored background still makes sense.
				maxA := r
				if g > maxA {
					maxA = g
				}
				if b > maxA {
					maxA = b
				}
				out.Pix[off+3] = byte(maxA)
			}
		}
	} else {
		for x := 0; x < physWidth; x++ {
			for y := 0; y < physHeight; y++ {
				stripeCenter := y * 3
				channels := [3]uint16{}
				for s := 0; s < 3; s++ {
					center := stripeCenter + s
					sum := uint32(0)
					for k := -2; k <= 2; k++ {
						sy := center + k
						if sy < 0 || sy >= h {
							continue
						}
						sum += uint32(buf.Pix[sy*buf.Stride+x]) * uint32(filter[k+2])
					}
					channels[s] = uint16((sum + 128) >> 8)
				}
				r, g, b := channels[0], channels[1], channels[2]
				if bgr {
					r, b = b, r
				}
				off := out.PixOffset(x, y)
				out.Pix[off+0] = byte(r)
				out.Pix[off+1] = byte(g)
				out.Pix[off+2] = byte(b)
				maxA := r
				if g > maxA {
					maxA = g
				}
				if b > maxA {
					maxA = b
				}
				out.Pix[off+3] = byte(maxA)
			}
		}
	}
	return out
}
