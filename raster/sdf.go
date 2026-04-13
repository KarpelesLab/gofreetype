// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package raster

import (
	"image"
	"math"
)

// SDFConfig controls signed-distance-field rasterization.
type SDFConfig struct {
	// Spread is the maximum distance (in output pixels) that the SDF
	// encodes. Pixels further than Spread from the edge clamp to the
	// respective extremum of the output range.
	Spread float64
}

// DefaultSDFConfig returns a sensible default (spread = 4 pixels), which
// fits well with typical GPU text shaders.
func DefaultSDFConfig() SDFConfig { return SDFConfig{Spread: 4} }

// RenderSDF rasterizes an outline into a signed distance field encoded
// as an 8-bit *image.Alpha. The output buffer has the given width and
// height. The outline is expected to already be at the target resolution.
//
// Encoding: 128 represents the edge, values > 128 are inside the glyph
// (brighter = deeper inside), values < 128 are outside. The scaling is
// such that a pixel at distance `Spread` from the edge maps to 0 (fully
// outside) or 255 (fully inside), with linear interpolation in between.
func RenderSDF(outlineFn func(r *Rasterizer), width, height int, cfg SDFConfig) *image.Alpha {
	if cfg.Spread <= 0 {
		cfg = DefaultSDFConfig()
	}

	// Step 1: rasterize to a monochrome in/out mask.
	r := NewRasterizer(width, height)
	outlineFn(r)
	mask := NewBitmap(image.Rect(0, 0, width, height))
	r.Rasterize(NewBitmapPainter(mask))

	// Step 2: 8SSEDT-style sequential Euclidean distance transform.
	// We track each pixel's offset to the nearest "inside" pixel (for the
	// outside field) and to the nearest "outside" pixel (for the inside
	// field). The signed distance is the difference.
	const bigD = 1e9
	inside := make([]float64, width*height)
	outside := make([]float64, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			if mask.BitAt(x, y) {
				inside[idx] = 0
				outside[idx] = bigD
			} else {
				inside[idx] = bigD
				outside[idx] = 0
			}
		}
	}

	// 8-neighbor Chamfer pass — forward then backward.
	edt := func(buf []float64) {
		update := func(x, y, dx, dy int, d float64) {
			nx, ny := x+dx, y+dy
			if nx < 0 || nx >= width || ny < 0 || ny >= height {
				return
			}
			if cand := buf[ny*width+nx] + d; cand < buf[y*width+x] {
				buf[y*width+x] = cand
			}
		}
		const diag = math.Sqrt2
		// Forward pass.
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				update(x, y, -1, -1, diag)
				update(x, y, 0, -1, 1)
				update(x, y, 1, -1, diag)
				update(x, y, -1, 0, 1)
			}
		}
		// Backward pass.
		for y := height - 1; y >= 0; y-- {
			for x := width - 1; x >= 0; x-- {
				update(x, y, 1, 0, 1)
				update(x, y, -1, 1, diag)
				update(x, y, 0, 1, 1)
				update(x, y, 1, 1, diag)
			}
		}
	}
	edt(inside)
	edt(outside)

	// Step 3: convert to 8-bit output. signedDist = outside - inside.
	// inside  > 0 for outside pixels (distance to nearest inside pixel).
	// outside > 0 for inside pixels (distance to nearest outside pixel).
	out := image.NewAlpha(image.Rect(0, 0, width, height))
	spread := cfg.Spread
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			var d float64
			if mask.BitAt(x, y) {
				d = outside[idx]
			} else {
				d = -inside[idx]
			}
			// Map [-spread, +spread] -> [0, 255], clamped.
			v := 128 + d/spread*127
			if v < 0 {
				v = 0
			} else if v > 255 {
				v = 255
			}
			out.Pix[y*out.Stride+x] = byte(v)
		}
	}
	return out
}
