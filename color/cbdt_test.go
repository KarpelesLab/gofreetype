// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package color

import (
	"encoding/binary"
	"testing"
)

// buildCBLC_CBDT builds a synthetic CBLC + CBDT table pair with one
// BitmapSize carrying image format 17 (SmallGlyphMetrics + PNG).
func buildCBLC_CBDT(ppem uint8, numGlyphs int, glyphs map[uint16]struct {
	w, h    uint8
	bx, by  int8
	adv     uint8
	pngData []byte
}) (cblc, cbdt []byte) {
	// CBDT: version header (4) + per-glyph data.
	var cbdtBuf []byte
	cbdtBuf = append(cbdtBuf, 0, 3, 0, 0) // version 3.0

	// Build per-glyph data and track offsets.
	type glyphEntry struct {
		gid         uint16
		offsetStart int
		offsetEnd   int
	}
	var entries []glyphEntry
	var sortedGIDs []uint16
	for gid := range glyphs {
		sortedGIDs = append(sortedGIDs, gid)
	}
	// Simple sort.
	for i := 1; i < len(sortedGIDs); i++ {
		for j := i; j > 0 && sortedGIDs[j-1] > sortedGIDs[j]; j-- {
			sortedGIDs[j-1], sortedGIDs[j] = sortedGIDs[j], sortedGIDs[j-1]
		}
	}

	for _, gid := range sortedGIDs {
		g := glyphs[gid]
		start := len(cbdtBuf)
		// SmallGlyphMetrics (5 bytes).
		cbdtBuf = append(cbdtBuf, g.h, g.w, byte(g.bx), byte(g.by), g.adv)
		// uint32 dataLen.
		cbdtBuf = binary.BigEndian.AppendUint32(cbdtBuf, uint32(len(g.pngData)))
		cbdtBuf = append(cbdtBuf, g.pngData...)
		entries = append(entries, glyphEntry{gid, start, len(cbdtBuf)})
	}

	// CBLC: one BitmapSize with one IndexSubTable (format 1) covering all
	// entries in a contiguous glyph range.
	if len(entries) == 0 {
		cblc = []byte{0, 3, 0, 0, 0, 0, 0, 0}
		cbdt = cbdtBuf
		return
	}
	firstGlyph := entries[0].gid
	lastGlyph := entries[len(entries)-1].gid

	// Index subtable format 1: header(8) + uint32 offsets[count+1].
	count := int(lastGlyph-firstGlyph) + 1
	idxSubLen := 8 + 4*(count+1)
	idxSubTable := make([]byte, idxSubLen)
	binary.BigEndian.PutUint16(idxSubTable[0:], 1)  // indexFormat
	binary.BigEndian.PutUint16(idxSubTable[2:], 17) // imageFormat
	binary.BigEndian.PutUint32(idxSubTable[4:], 0)  // imageDataOffset (offsets in the per-glyph array are absolute into CBDT)

	// Fill offsets. Missing glyphs have the same start as the next.
	offArr := idxSubTable[8:]
	ei := 0
	for i := 0; i <= count; i++ {
		gid := firstGlyph + uint16(i)
		if ei < len(entries) && entries[ei].gid == gid {
			binary.BigEndian.PutUint32(offArr[4*i:], uint32(entries[ei].offsetStart))
			if i < count {
				ei++
			}
		} else if ei < len(entries) {
			binary.BigEndian.PutUint32(offArr[4*i:], uint32(entries[min(ei, len(entries)-1)].offsetStart))
		} else {
			binary.BigEndian.PutUint32(offArr[4*i:], uint32(entries[len(entries)-1].offsetEnd))
		}
	}
	// Final sentinel.
	binary.BigEndian.PutUint32(offArr[4*count:], uint32(entries[len(entries)-1].offsetEnd))

	// IndexSubTableArray: one record.
	var idxArray []byte
	idxArray = binary.BigEndian.AppendUint16(idxArray, firstGlyph)
	idxArray = binary.BigEndian.AppendUint16(idxArray, lastGlyph)
	idxArray = binary.BigEndian.AppendUint32(idxArray, uint32(len(idxArray)+4)) // offset from idxArray start to subtable

	// BitmapSize record (48 bytes).
	bitmapSizeRec := make([]byte, 48)
	idxArrayOff := 8 + 48 // after CBLC header + 1 BitmapSize
	binary.BigEndian.PutUint32(bitmapSizeRec[0:], uint32(idxArrayOff))
	binary.BigEndian.PutUint32(bitmapSizeRec[4:], uint32(len(idxArray)+idxSubLen)) // indexTablesSize
	binary.BigEndian.PutUint32(bitmapSizeRec[8:], 1)                               // numberOfIndexSubTables
	bitmapSizeRec[44] = ppem
	bitmapSizeRec[45] = ppem
	bitmapSizeRec[46] = 32 // bitDepth

	// Assemble CBLC.
	var cblcBuf []byte
	cblcBuf = append(cblcBuf, 0, 3, 0, 0)               // version 3.0
	cblcBuf = binary.BigEndian.AppendUint32(cblcBuf, 1) // numSizes
	cblcBuf = append(cblcBuf, bitmapSizeRec...)
	cblcBuf = append(cblcBuf, idxArray...)
	cblcBuf = append(cblcBuf, idxSubTable...)

	return cblcBuf, cbdtBuf
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestCBLCCBDT(t *testing.T) {
	pngData := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 't', 'e', 's', 't'}

	cblcData, cbdtData := buildCBLC_CBDT(128, 10, map[uint16]struct {
		w, h    uint8
		bx, by  int8
		adv     uint8
		pngData []byte
	}{
		5: {w: 20, h: 20, bx: 0, by: 20, adv: 22, pngData: pngData},
	})
	tbl, err := ParseCBLC(cblcData, cbdtData)
	if err != nil {
		t.Fatalf("ParseCBLC: %v", err)
	}
	if len(tbl.Sets) == 0 {
		t.Fatal("no BitmapSets parsed")
	}
	set := tbl.FindSet(128)
	if set == nil {
		t.Fatal("FindSet(128) returned nil")
	}
	g := set.Glyph(5)
	if g == nil {
		t.Fatal("Glyph(5) is nil")
	}
	if g.Width != 20 || g.Height != 20 {
		t.Errorf("dimensions: got %dx%d, want 20x20", g.Width, g.Height)
	}
	if g.Advance != 22 {
		t.Errorf("advance: got %d, want 22", g.Advance)
	}
	if len(g.Data) != len(pngData) {
		t.Errorf("data length: got %d, want %d", len(g.Data), len(pngData))
	}
	if set.Glyph(0) != nil {
		t.Error("Glyph(0) should be nil for missing bitmap")
	}
}
