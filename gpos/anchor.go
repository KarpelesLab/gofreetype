// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package gpos

// Anchor is a point on a glyph used by cursive and mark-attachment lookups.
// The X and Y coordinates are in font design units.
//
// OpenType specifies three on-disk formats:
//
//	Format 1: plain (x, y)
//	Format 2: (x, y, anchorPointIndex)  — the anchor rides a glyph point
//	          index for hinted placement
//	Format 3: (x, y, xDeviceOffset, yDeviceOffset) — hinting deltas
//
// We currently only preserve the (x, y) coordinates: hinting-dependent
// anchor adjustments are a later phase.
type Anchor struct {
	X, Y int16
}

// parseAnchor decodes an Anchor table at sub[off:]. It returns the anchor
// and whether it is present (off == 0 means "no anchor").
func parseAnchor(sub []byte, off int) (Anchor, bool, error) {
	if off == 0 {
		return Anchor{}, false, nil
	}
	if off+6 > len(sub) {
		return Anchor{}, false, FormatError("Anchor table truncated")
	}
	format := u16(sub, off)
	if format < 1 || format > 3 {
		return Anchor{}, false, UnsupportedError("Anchor format " + intCount(int(format)))
	}
	return Anchor{
		X: int16(u16(sub, off+2)),
		Y: int16(u16(sub, off+4)),
	}, true, nil
}
