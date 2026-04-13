// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

// Package color provides parsers for OpenType color font tables: CPAL
// (Color Palette Table) and COLR (Color Glyph Table).
package color

import (
	"fmt"
	"image/color"
)

// FormatError reports a malformed color table.
type FormatError string

func (e FormatError) Error() string { return "color: invalid: " + string(e) }

// Palette is one named palette from a CPAL table. Each element is a
// pre-multiplied BGRA color (stored in the font as BGRA, converted here
// to standard Go color.NRGBA).
type Palette struct {
	Colors []color.NRGBA
	Label  uint16 // NameID for the palette name (0xFFFF = no name). v1 only.
	Type   uint32 // Palette type flags (v1 only): 0x01 = usable with light bg, 0x02 = dark bg.
}

// CPAL is a parsed Color Palette Table.
type CPAL struct {
	Palettes []Palette
}

// ParseCPAL decodes a CPAL v0 or v1 table.
//
// v0 header (12 bytes):
//
//	uint16 version (0 or 1)
//	uint16 numPaletteEntries   (entries per palette)
//	uint16 numPalettes
//	uint16 numColorRecords
//	Offset32 colorRecordsArrayOffset
//	uint16 colorRecordIndices[numPalettes]
//
// v1 adds (after the indices):
//
//	Offset32 paletteTypesArrayOffset  (may be 0)
//	Offset32 paletteLabelsArrayOffset (may be 0)
//	Offset32 paletteEntryLabelsArrayOffset (may be 0)
func ParseCPAL(data []byte) (*CPAL, error) {
	if len(data) < 12 {
		return nil, FormatError("CPAL table too short")
	}
	version := u16(data, 0)
	if version > 1 {
		return nil, FormatError(fmt.Sprintf("CPAL version %d (expected 0 or 1)", version))
	}
	numEntries := int(u16(data, 2))
	numPalettes := int(u16(data, 4))
	numColors := int(u16(data, 6))
	colorOff := int(u32(data, 8))
	indicesOff := 12

	if indicesOff+2*numPalettes > len(data) {
		return nil, FormatError("CPAL colorRecordIndices truncated")
	}
	if colorOff+4*numColors > len(data) {
		return nil, FormatError("CPAL color records out of bounds")
	}

	// Optional v1 arrays.
	var typesOff, labelsOff int
	if version == 1 {
		v1Start := indicesOff + 2*numPalettes
		if v1Start+12 > len(data) {
			return nil, FormatError("CPAL v1 extension truncated")
		}
		typesOff = int(u32(data, v1Start))
		labelsOff = int(u32(data, v1Start+4))
		// paletteEntryLabelsArrayOffset at v1Start+8 — not needed for our API.
	}

	cpal := &CPAL{Palettes: make([]Palette, numPalettes)}
	for i := 0; i < numPalettes; i++ {
		firstColor := int(u16(data, indicesOff+2*i))
		if firstColor+numEntries > numColors {
			return nil, FormatError("palette color index out of range")
		}
		colors := make([]color.NRGBA, numEntries)
		for j := 0; j < numEntries; j++ {
			off := colorOff + 4*(firstColor+j)
			// CPAL stores BGRA (blue first).
			b, g, r, a := data[off], data[off+1], data[off+2], data[off+3]
			colors[j] = color.NRGBA{R: r, G: g, B: b, A: a}
		}
		cpal.Palettes[i].Colors = colors

		if version == 1 {
			if typesOff != 0 && typesOff+4*(i+1) <= len(data) {
				cpal.Palettes[i].Type = u32(data, typesOff+4*i)
			}
			if labelsOff != 0 && labelsOff+2*(i+1) <= len(data) {
				cpal.Palettes[i].Label = u16(data, labelsOff+2*i)
			} else {
				cpal.Palettes[i].Label = 0xFFFF
			}
		} else {
			cpal.Palettes[i].Label = 0xFFFF
		}
	}
	return cpal, nil
}

func u16(b []byte, i int) uint16 {
	return uint16(b[i])<<8 | uint16(b[i+1])
}

func u32(b []byte, i int) uint32 {
	return uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3])
}
