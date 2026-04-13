// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package color

import "fmt"

// Sbix is a parsed sbix (Standard Bitmap Graphics) table. Apple uses this
// for color emoji: each "strike" carries pre-rendered glyph images at a
// specific ppem size, typically as PNG.
type Sbix struct {
	data    []byte
	Strikes []SbixStrike
}

// SbixStrike is one ppem-specific set of glyph images.
type SbixStrike struct {
	PPEM uint16
	PPI  uint16

	// start is the absolute offset of the Strike header within Sbix.data.
	start    int
	numGlyphs int
	data     []byte
}

// SbixGlyph is one pre-rendered glyph image from an sbix strike.
type SbixGlyph struct {
	OriginOffsetX int16
	OriginOffsetY int16
	GraphicType   [4]byte // e.g. "png ", "jpg ", "tiff"
	Data          []byte  // Raw image bytes (PNG, JPEG, etc.)
}

// ParseSbix decodes an sbix table. numGlyphs is the font's glyph count
// (from maxp) — needed because strike offset arrays are sized to numGlyphs+1.
func ParseSbix(data []byte, numGlyphs int) (*Sbix, error) {
	if len(data) < 8 {
		return nil, FormatError("sbix table too short")
	}
	version := u16(data, 0)
	if version != 1 {
		return nil, FormatError(fmt.Sprintf("sbix version %d (expected 1)", version))
	}
	// flags at offset 2 (unused).
	numStrikes := int(u32(data, 4))
	if 8+4*numStrikes > len(data) {
		return nil, FormatError("sbix strike offsets truncated")
	}

	sb := &Sbix{data: data, Strikes: make([]SbixStrike, numStrikes)}
	for i := 0; i < numStrikes; i++ {
		off := int(u32(data, 8+4*i))
		if off+4 > len(data) {
			return nil, FormatError("sbix strike header out of bounds")
		}
		ppem := u16(data, off)
		ppi := u16(data, off+2)
		sb.Strikes[i] = SbixStrike{
			PPEM:      ppem,
			PPI:       ppi,
			start:     off,
			numGlyphs: numGlyphs,
			data:      data,
		}
	}
	return sb, nil
}

// FindStrike returns the strike whose ppem is closest to the requested
// size. Returns nil if there are no strikes.
func (sb *Sbix) FindStrike(ppem uint16) *SbixStrike {
	if sb == nil || len(sb.Strikes) == 0 {
		return nil
	}
	best := &sb.Strikes[0]
	bestDiff := absDiffU16(best.PPEM, ppem)
	for i := 1; i < len(sb.Strikes); i++ {
		d := absDiffU16(sb.Strikes[i].PPEM, ppem)
		if d < bestDiff {
			best = &sb.Strikes[i]
			bestDiff = d
		}
	}
	return best
}

func absDiffU16(a, b uint16) uint16 {
	if a > b {
		return a - b
	}
	return b - a
}

// Glyph returns the pre-rendered image for the given glyph id, or nil
// if no image exists (empty glyphs have a zero-length data offset pair).
// A "dupe" graphic type is resolved by following one level of indirection.
func (s *SbixStrike) Glyph(gid int) *SbixGlyph {
	if s == nil || gid < 0 || gid >= s.numGlyphs {
		return nil
	}
	offArrayStart := s.start + 4 // past ppem + ppi
	if offArrayStart+4*(gid+2) > len(s.data) {
		return nil
	}
	dataOff := int(u32(s.data, offArrayStart+4*gid))
	dataEnd := int(u32(s.data, offArrayStart+4*(gid+1)))
	if dataOff == dataEnd {
		return nil
	}
	abs := s.start + dataOff
	absEnd := s.start + dataEnd
	if abs+8 > absEnd || absEnd > len(s.data) {
		return nil
	}
	g := &SbixGlyph{
		OriginOffsetX: int16(u16(s.data, abs)),
		OriginOffsetY: int16(u16(s.data, abs+2)),
		Data:          s.data[abs+8 : absEnd],
	}
	copy(g.GraphicType[:], s.data[abs+4:abs+8])

	// Resolve "dupe" by following one level of indirection.
	if g.GraphicType == [4]byte{'d', 'u', 'p', 'e'} && len(g.Data) >= 2 {
		dupeGID := int(u16(g.Data, 0))
		if dupeGID != gid {
			return s.Glyph(dupeGID)
		}
	}
	return g
}
