// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package color

import "fmt"

// Layer describes one rendering layer in a COLR v0 color glyph.
// The glyph is drawn by rasterizing GlyphID (which names a regular
// outline glyph) and filling it with Palette[PaletteIndex].
// PaletteIndex 0xFFFF means "use the current text foreground color".
type Layer struct {
	GlyphID      uint16
	PaletteIndex uint16
}

// COLR is a parsed COLR v0 table (color glyph layers).
//
// v0 layout:
//
//	uint16 version (0)
//	uint16 numBaseGlyphRecords
//	Offset32 baseGlyphRecordsOffset
//	Offset32 layerRecordsOffset
//	uint16 numLayerRecords
//
// BaseGlyphRecord (6 bytes each, sorted by glyphID):
//
//	uint16 glyphID
//	uint16 firstLayerIndex
//	uint16 numLayers
//
// LayerRecord (4 bytes each):
//
//	uint16 glyphID       (the outline to rasterize)
//	uint16 paletteIndex  (CPAL entry, or 0xFFFF = foreground)
type COLR struct {
	data      []byte
	bases     []baseGlyphRecord
	layerData []byte
	numLayers int
}

type baseGlyphRecord struct {
	glyphID    uint16
	firstLayer uint16
	numLayers  uint16
}

// ParseCOLR decodes a COLR v0 table.
func ParseCOLR(data []byte) (*COLR, error) {
	if len(data) < 14 {
		return nil, FormatError("COLR table too short")
	}
	version := u16(data, 0)
	if version != 0 {
		return nil, FormatError(fmt.Sprintf("COLR version %d (only v0 supported)", version))
	}
	numBases := int(u16(data, 2))
	baseOff := int(u32(data, 4))
	layerOff := int(u32(data, 8))
	numLayers := int(u16(data, 12))

	if baseOff+6*numBases > len(data) {
		return nil, FormatError("COLR base glyph records out of bounds")
	}
	if layerOff+4*numLayers > len(data) {
		return nil, FormatError("COLR layer records out of bounds")
	}

	bases := make([]baseGlyphRecord, numBases)
	for i := 0; i < numBases; i++ {
		off := baseOff + 6*i
		bases[i] = baseGlyphRecord{
			glyphID:    u16(data, off),
			firstLayer: u16(data, off+2),
			numLayers:  u16(data, off+4),
		}
	}

	return &COLR{
		data:      data,
		bases:     bases,
		layerData: data[layerOff:],
		numLayers: numLayers,
	}, nil
}

// ColorLayers returns the layer stack for the given base glyph, or nil
// if the glyph is not a color glyph. Layers are ordered bottom-to-top
// (paint the first layer first, then overlay subsequent layers).
func (c *COLR) ColorLayers(glyphID uint16) []Layer {
	if c == nil {
		return nil
	}
	// Binary search — base glyph records are sorted by glyphID.
	lo, hi := 0, len(c.bases)
	for lo < hi {
		m := lo + (hi-lo)/2
		mid := c.bases[m].glyphID
		if mid < glyphID {
			lo = m + 1
		} else if mid > glyphID {
			hi = m
		} else {
			first := int(c.bases[m].firstLayer)
			n := int(c.bases[m].numLayers)
			if first+n > c.numLayers {
				return nil
			}
			layers := make([]Layer, n)
			for j := 0; j < n; j++ {
				off := 4 * (first + j)
				layers[j] = Layer{
					GlyphID:      u16(c.layerData, off),
					PaletteIndex: u16(c.layerData, off+2),
				}
			}
			return layers
		}
	}
	return nil
}

// IsColorGlyph reports whether glyphID has color layers.
func (c *COLR) IsColorGlyph(glyphID uint16) bool {
	return len(c.ColorLayers(glyphID)) > 0
}
