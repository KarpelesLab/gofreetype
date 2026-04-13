// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package color

import "fmt"

// CBDT/CBLC are the Google-originated bitmap tables used primarily by the
// Noto Color Emoji font. CBLC is the "Color Bitmap Location" table that
// indexes into CBDT ("Color Bitmap Data Table"). The structure mirrors the
// older EBLC/EBDT pair used for monochrome bitmaps but carries PNG images
// instead.
//
// The tables are complex — multi-level index:
//   CBLC header → BitmapSize array → IndexSubTableArray → index subtable
//   → offset into CBDT where the glyph data lives.
//
// We focus on the most common format pair: index subtable format 1
// (variable-size metrics, image format 17 = PNG with metrics).

// BitmapSet is one ppem-specific set of glyph bitmaps parsed from
// CBLC + CBDT.
type BitmapSet struct {
	PPEM    uint8
	BitDepth uint8

	// entries maps glyph id to raw image data. We pre-parse the index at
	// load time so Glyph() is a simple map lookup.
	entries map[uint16]BitmapGlyph
}

// BitmapGlyph is a single pre-rendered glyph image from a CBDT table.
type BitmapGlyph struct {
	Width, Height       uint8
	BearingX, BearingY  int8
	Advance             uint8
	Format              uint8 // 17 = PNG, 18 = PNG with big metrics, 19 = PNG with no metrics
	Data                []byte
}

// CBLC is a parsed Color Bitmap Location Table. It references the CBDT
// data to locate individual glyph images.
type CBLC struct {
	Sets []BitmapSet
}

// ParseCBLC decodes a CBLC + CBDT table pair. Both byte slices must be
// provided.
func ParseCBLC(cblc, cbdt []byte) (*CBLC, error) {
	if len(cblc) < 8 {
		return nil, FormatError("CBLC table too short")
	}
	major := u16(cblc, 0)
	if major != 3 {
		return nil, FormatError(fmt.Sprintf("CBLC version %d.%d (expected 3.x)", major, u16(cblc, 2)))
	}
	numSizes := int(u32(cblc, 4))
	sizeRecOff := 8
	if sizeRecOff+48*numSizes > len(cblc) {
		return nil, FormatError("CBLC BitmapSize records truncated")
	}

	result := &CBLC{}
	for s := 0; s < numSizes; s++ {
		rec := sizeRecOff + 48*s
		idxSubTableArrayOff := int(u32(cblc, rec))
		// idxTableSize at rec+4 (unused here)
		// numberofIndexSubTables at rec+8
		numSubTables := int(u32(cblc, rec+8))
		ppem := cblc[rec+44]
		bitDepth := cblc[rec+46]

		entries := make(map[uint16]BitmapGlyph)
		if err := parseIndexSubTableArray(cblc, cbdt, idxSubTableArrayOff, numSubTables, entries); err != nil {
			continue // skip broken sizes rather than failing the whole parse
		}
		if len(entries) > 0 {
			result.Sets = append(result.Sets, BitmapSet{
				PPEM:     ppem,
				BitDepth: bitDepth,
				entries:  entries,
			})
		}
	}
	return result, nil
}

func parseIndexSubTableArray(cblc, cbdt []byte, off, n int, entries map[uint16]BitmapGlyph) error {
	if off+8*n > len(cblc) {
		return FormatError("IndexSubTableArray truncated")
	}
	for i := 0; i < n; i++ {
		rec := off + 8*i
		firstGlyph := u16(cblc, rec)
		lastGlyph := u16(cblc, rec+2)
		addlOff := int(u32(cblc, rec+4))
		subOff := off + addlOff
		if subOff+8 > len(cblc) {
			continue
		}
		indexFormat := u16(cblc, subOff)
		imageFormat := u16(cblc, subOff+2)
		imageDataOffset := int(u32(cblc, subOff+4))

		switch indexFormat {
		case 1:
			parseIndexFormat1(cblc, cbdt, subOff+8, imageDataOffset, imageFormat, firstGlyph, lastGlyph, entries)
		case 3:
			parseIndexFormat3(cblc, cbdt, subOff+8, imageDataOffset, imageFormat, firstGlyph, lastGlyph, entries)
		}
	}
	return nil
}

// Index format 1: variable-size offsets, one per glyph + sentinel.
func parseIndexFormat1(cblc, cbdt []byte, offStart, imageBase int, imageFormat, first, last uint16, entries map[uint16]BitmapGlyph) {
	count := int(last-first) + 1
	if offStart+4*(count+1) > len(cblc) {
		return
	}
	for i := 0; i < count; i++ {
		gOff := int(u32(cblc, offStart+4*i))
		gEnd := int(u32(cblc, offStart+4*(i+1)))
		abs := imageBase + gOff
		absEnd := imageBase + gEnd
		if abs >= absEnd || absEnd > len(cbdt) {
			continue
		}
		gid := first + uint16(i)
		g := decodeBitmapGlyph(cbdt[abs:absEnd], imageFormat)
		if g != nil {
			entries[gid] = *g
		}
	}
}

// Index format 3: like format 1 but 16-bit offsets.
func parseIndexFormat3(cblc, cbdt []byte, offStart, imageBase int, imageFormat, first, last uint16, entries map[uint16]BitmapGlyph) {
	count := int(last-first) + 1
	if offStart+2*(count+1) > len(cblc) {
		return
	}
	for i := 0; i < count; i++ {
		gOff := int(u16(cblc, offStart+2*i))
		gEnd := int(u16(cblc, offStart+2*(i+1)))
		abs := imageBase + gOff
		absEnd := imageBase + gEnd
		if abs >= absEnd || absEnd > len(cbdt) {
			continue
		}
		gid := first + uint16(i)
		g := decodeBitmapGlyph(cbdt[abs:absEnd], imageFormat)
		if g != nil {
			entries[gid] = *g
		}
	}
}

func decodeBitmapGlyph(data []byte, imageFormat uint16) *BitmapGlyph {
	switch imageFormat {
	case 17:
		// SmallGlyphMetrics (5 bytes) + uint32 dataLen + PNG data.
		if len(data) < 9 {
			return nil
		}
		dataLen := int(u32(data, 5))
		if 9+dataLen > len(data) {
			dataLen = len(data) - 9
		}
		return &BitmapGlyph{
			Height:   data[0],
			Width:    data[1],
			BearingX: int8(data[2]),
			BearingY: int8(data[3]),
			Advance:  data[4],
			Format:   17,
			Data:     data[9 : 9+dataLen],
		}
	case 18:
		// BigGlyphMetrics (8 bytes) + uint32 dataLen + PNG data.
		if len(data) < 12 {
			return nil
		}
		dataLen := int(u32(data, 8))
		if 12+dataLen > len(data) {
			dataLen = len(data) - 12
		}
		return &BitmapGlyph{
			Height:   data[0],
			Width:    data[1],
			BearingX: int8(data[2]),
			BearingY: int8(data[3]),
			Advance:  data[7],
			Format:   18,
			Data:     data[12 : 12+dataLen],
		}
	case 19:
		// No per-glyph metrics — just uint32 dataLen + PNG data.
		if len(data) < 4 {
			return nil
		}
		dataLen := int(u32(data, 0))
		if 4+dataLen > len(data) {
			dataLen = len(data) - 4
		}
		return &BitmapGlyph{
			Format: 19,
			Data:   data[4 : 4+dataLen],
		}
	}
	return nil
}

// FindSet returns the BitmapSet whose ppem best matches the requested size.
func (c *CBLC) FindSet(ppem uint8) *BitmapSet {
	if c == nil || len(c.Sets) == 0 {
		return nil
	}
	best := &c.Sets[0]
	bestDiff := absDiffU8(best.PPEM, ppem)
	for i := 1; i < len(c.Sets); i++ {
		d := absDiffU8(c.Sets[i].PPEM, ppem)
		if d < bestDiff {
			best = &c.Sets[i]
			bestDiff = d
		}
	}
	return best
}

func absDiffU8(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

// Glyph returns the bitmap for the given glyph id, or nil if absent.
func (s *BitmapSet) Glyph(gid uint16) *BitmapGlyph {
	if s == nil {
		return nil
	}
	g, ok := s.entries[gid]
	if !ok {
		return nil
	}
	return &g
}
