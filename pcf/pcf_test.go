// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package pcf

import (
	"encoding/binary"
	"testing"
)

func TestParseBadMagic(t *testing.T) {
	_, err := Parse([]byte("notpcf!!"))
	if err == nil {
		t.Fatal("expected error for bad magic")
	}
	if _, ok := err.(FormatError); !ok {
		t.Errorf("want FormatError, got %T: %v", err, err)
	}
}

func TestParseEmpty(t *testing.T) {
	_, err := Parse(nil)
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

// buildMinimalPCF synthesizes a one-glyph PCF file with a 3x3 filled
// square for codepoint 'A' (encoding 65). Single-byte encoded, big-
// endian format for the non-header fields, MSB-first bit order, 1-byte
// glyph padding.
func buildMinimalPCF() []byte {
	// Helpers to encode values in chosen byte order.
	bo := binary.BigEndian
	le := binary.LittleEndian

	// --- Metrics table (uncompressed, BE, 1 glyph) ---
	metrics := make([]byte, 8+12)
	le.PutUint32(metrics[0:4], fmtByteMask) // format: BE
	bo.PutUint32(metrics[4:8], 1)           // count
	// One glyph: LSB=0, RSB=3, width=4, ascent=3, descent=0, attrs=0.
	bo.PutUint16(metrics[8:10], 0)  // LSB
	bo.PutUint16(metrics[10:12], 3) // RSB
	bo.PutUint16(metrics[12:14], 4) // width
	bo.PutUint16(metrics[14:16], 3) // ascent
	bo.PutUint16(metrics[16:18], 0) // descent
	bo.PutUint16(metrics[18:20], 0) // attrs

	// --- Bitmap table (BE, 1 glyph, all rows 0xE0 = 11100000 for 3 pixels) ---
	// Format flags: BE + glyphPad=0 (1 byte).
	bitmapFormat := uint32(fmtByteMask)
	// header: format(4) + count(4) + offsets(4*1) + sizes(16) + data
	// Each row is stride=1 byte; h=3 rows; total bitmap bytes = 3.
	bitmaps := make([]byte, 8+4+16+3)
	le.PutUint32(bitmaps[0:4], bitmapFormat)
	bo.PutUint32(bitmaps[4:8], 1) // count
	bo.PutUint32(bitmaps[8:12], 0) // offset[0]
	bo.PutUint32(bitmaps[12:16], 3)  // sizes[0] (pad=1)
	bo.PutUint32(bitmaps[16:20], 3)  // sizes[1]
	bo.PutUint32(bitmaps[20:24], 3)  // sizes[2]
	bo.PutUint32(bitmaps[24:28], 3)  // sizes[3]
	// Bitmap bytes. 3 rows × 1 stride = 3 bytes, MSB-first.
	// We want a 3x3 square: all bits in the top 3 columns of each row set.
	// 0xE0 = 11100000 → pixels (0,y), (1,y), (2,y) set.
	bitmaps[28] = 0xE0
	bitmaps[29] = 0xE0
	bitmaps[30] = 0xE0

	// Default format already has MSB-first = fmtBitMask. Let me set it.
	bitmapFormat |= fmtBitMask
	le.PutUint32(bitmaps[0:4], bitmapFormat)

	// --- Encodings table (BE, single-byte, mapping 'A'=65 -> glyph 0) ---
	// cellsPerRow = maxCharOrByte2 - minCharOrByte2 + 1
	// We'll use minCharOrByte2=65 maxCharOrByte2=65 minByte1=0 maxByte1=0.
	encodings := make([]byte, 14+2)
	le.PutUint32(encodings[0:4], fmtByteMask)
	bo.PutUint16(encodings[4:6], 65)      // minCharOrByte2
	bo.PutUint16(encodings[6:8], 65)      // maxCharOrByte2
	bo.PutUint16(encodings[8:10], 0)      // minByte1
	bo.PutUint16(encodings[10:12], 0)     // maxByte1
	bo.PutUint16(encodings[12:14], 0xFFFF) // defaultChar
	bo.PutUint16(encodings[14:16], 0)     // glyph index for 'A'

	// --- Assemble TOC + body ---
	// TOC entries (16 bytes each): type, format, size, offset.
	tocCount := 3
	tocLen := 8 + 16*tocCount
	metricsOff := tocLen
	bitmapsOff := metricsOff + len(metrics)
	encodingsOff := bitmapsOff + len(bitmaps)
	total := encodingsOff + len(encodings)

	out := make([]byte, total)
	// Magic \x01 fcp.
	out[0] = 0x01
	out[1] = 'f'
	out[2] = 'c'
	out[3] = 'p'
	le.PutUint32(out[4:8], uint32(tocCount))

	putEntry := func(i, typ int, format uint32, size, off int) {
		p := 8 + 16*i
		le.PutUint32(out[p:p+4], uint32(typ))
		le.PutUint32(out[p+4:p+8], format)
		le.PutUint32(out[p+8:p+12], uint32(size))
		le.PutUint32(out[p+12:p+16], uint32(off))
	}
	putEntry(0, tocMetrics, fmtByteMask, len(metrics), metricsOff)
	putEntry(1, tocBitmaps, bitmapFormat, len(bitmaps), bitmapsOff)
	putEntry(2, tocBdfEncodings, fmtByteMask, len(encodings), encodingsOff)

	copy(out[metricsOff:], metrics)
	copy(out[bitmapsOff:], bitmaps)
	copy(out[encodingsOff:], encodings)
	return out
}

func TestParseMinimalPCF(t *testing.T) {
	data := buildMinimalPCF()
	f, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Glyphs) != 1 {
		t.Fatalf("glyphs: got %d, want 1", len(f.Glyphs))
	}
	g := f.Glyph('A')
	if g == nil {
		t.Fatal("Glyph('A') is nil")
	}
	if g.Advance != 4 {
		t.Errorf("advance: got %d, want 4", g.Advance)
	}
	if g.BBX != 3 || g.BBY != 3 {
		t.Errorf("BBX/BBY: got %dx%d, want 3x3", g.BBX, g.BBY)
	}
	if g.Bitmap == nil {
		t.Fatal("Bitmap is nil")
	}
	// All 3x3 pixels should be set.
	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			if !g.Bitmap.BitAt(x, y) {
				t.Errorf("pixel (%d,%d) should be set", x, y)
			}
		}
	}
	// Unmapped codepoint returns nil.
	if f.Glyph('Z') != nil {
		t.Error("Glyph('Z') should be nil")
	}
}
