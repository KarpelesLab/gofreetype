// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

// Package pcf parses X11 "Portable Compiled Format" bitmap fonts. PCF
// is the binary format X11 keeps in /usr/share/fonts/misc/... and the
// compiled output of bdftopcf. Use this package when you want the
// same glyphs that BDF provides but loaded without re-parsing text.
//
// Spec: Xorg sources (xc/fonts/lib/fontfile/pcfread.c and friends).
package pcf

import (
	"encoding/binary"
	"fmt"
	"image"

	"github.com/KarpelesLab/gofreetype/raster"
)

// FormatError reports a malformed PCF file.
type FormatError string

func (e FormatError) Error() string { return "pcf: invalid: " + string(e) }

// Table-of-contents type identifiers (bit mask selecting which table).
const (
	tocProperties      = 1 << 0
	tocAccelerators    = 1 << 1
	tocMetrics         = 1 << 2
	tocBitmaps         = 1 << 3
	tocInkMetrics      = 1 << 4
	tocBdfEncodings    = 1 << 5
	tocSWidths         = 1 << 6
	tocGlyphNames      = 1 << 7
	tocBdfAccelerators = 1 << 8
)

// Format flags — low 4 bits are the format type, high bits are modifiers.
const (
	fmtDefault           = 0x00000000
	fmtInkbounds         = 0x00000200
	fmtAccelWithInk      = 0x00000100
	fmtCompressedMetrics = 0x00000100
	fmtByteMask          = 1 << 2 // big-endian byte order
	fmtBitMask           = 1 << 3 // most significant bit first
	fmtGlyphPadMask      = 3 << 0 // scanline pad in bytes: 1/2/4/8
	fmtScanUnitMask      = 3 << 4
)

// Font is a parsed PCF font.
type Font struct {
	Glyphs []Glyph

	runeToIndex map[rune]int

	// Default / min metrics read from the accelerators table, if present.
	FontAscent  int
	FontDescent int
}

// Glyph is a single bitmap glyph.
type Glyph struct {
	Encoding rune // -1 if unmapped
	Advance  int
	BBX      int
	BBY      int
	BBOx     int
	BBOy     int
	Bitmap   *raster.Bitmap
}

// Parse decodes a PCF file.
func Parse(data []byte) (*Font, error) {
	if len(data) < 8 {
		return nil, FormatError("PCF header too short")
	}
	// PCF starts with "\x01fcp" and a uint32 count of TOC entries (both LE).
	if data[0] != 0x01 || data[1] != 'f' || data[2] != 'c' || data[3] != 'p' {
		return nil, FormatError("bad PCF magic")
	}
	tocCount := int(binary.LittleEndian.Uint32(data[4:8]))
	if 8+16*tocCount > len(data) {
		return nil, FormatError("PCF TOC truncated")
	}

	// Each TOC entry: uint32 type, uint32 format, uint32 size, uint32 offset.
	var metricsOff, bitmapsOff, encodingsOff, accelOff int
	haveMetrics, haveBitmaps, haveEncodings := false, false, false
	haveAccel := false
	for i := 0; i < tocCount; i++ {
		entry := 8 + 16*i
		typ := binary.LittleEndian.Uint32(data[entry : entry+4])
		off := int(binary.LittleEndian.Uint32(data[entry+12 : entry+16]))
		switch typ {
		case tocMetrics:
			metricsOff = off
			haveMetrics = true
		case tocBitmaps:
			bitmapsOff = off
			haveBitmaps = true
		case tocBdfEncodings:
			encodingsOff = off
			haveEncodings = true
		case tocAccelerators, tocBdfAccelerators:
			accelOff = off
			haveAccel = true
		}
	}
	if !haveMetrics {
		return nil, FormatError("PCF metrics table missing")
	}
	if !haveBitmaps {
		return nil, FormatError("PCF bitmaps table missing")
	}

	font := &Font{runeToIndex: make(map[rune]int)}

	// Metrics.
	metrics, err := parseMetrics(data, metricsOff)
	if err != nil {
		return nil, fmt.Errorf("metrics: %w", err)
	}
	font.Glyphs = make([]Glyph, len(metrics))
	for i, m := range metrics {
		font.Glyphs[i] = Glyph{
			Encoding: -1,
			Advance:  int(m.characterWidth),
			BBX:      int(m.rightSideBearing) - int(m.leftSideBearing),
			BBY:      int(m.characterAscent) + int(m.characterDescent),
			BBOx:     int(m.leftSideBearing),
			BBOy:     -int(m.characterDescent),
		}
	}

	// Bitmaps.
	if err := parseBitmaps(data, bitmapsOff, font.Glyphs, metrics); err != nil {
		return nil, fmt.Errorf("bitmaps: %w", err)
	}

	// Encodings — maps codepoints to glyph indexes.
	if haveEncodings {
		if err := parseEncodings(data, encodingsOff, font); err != nil {
			return nil, fmt.Errorf("encodings: %w", err)
		}
	}

	// Accelerators (optional) provide font-wide metrics.
	if haveAccel {
		parseAccelerators(data, accelOff, font)
	}

	return font, nil
}

// metric is one decoded glyph metric.
type metric struct {
	leftSideBearing  int16
	rightSideBearing int16
	characterWidth   int16
	characterAscent  int16
	characterDescent int16
}

func parseMetrics(data []byte, off int) ([]metric, error) {
	if off+8 > len(data) {
		return nil, FormatError("metrics header truncated")
	}
	format := binary.LittleEndian.Uint32(data[off : off+4])
	bo := byteOrder(format)
	var count int
	var start int
	compressed := format&0xFFFFFF00 == fmtCompressedMetrics
	if compressed {
		count = int(bo.Uint16(data[off+4 : off+6]))
		start = off + 6
	} else {
		count = int(bo.Uint32(data[off+4 : off+8]))
		start = off + 8
	}

	if count < 0 {
		return nil, FormatError("negative metric count")
	}
	metrics := make([]metric, count)
	if compressed {
		if start+5*count > len(data) {
			return nil, FormatError("compressed metrics truncated")
		}
		for i := 0; i < count; i++ {
			base := start + 5*i
			metrics[i] = metric{
				leftSideBearing:  int16(int(data[base]) - 0x80),
				rightSideBearing: int16(int(data[base+1]) - 0x80),
				characterWidth:   int16(int(data[base+2]) - 0x80),
				characterAscent:  int16(int(data[base+3]) - 0x80),
				characterDescent: int16(int(data[base+4]) - 0x80),
			}
		}
	} else {
		if start+12*count > len(data) {
			return nil, FormatError("metrics truncated")
		}
		for i := 0; i < count; i++ {
			base := start + 12*i
			metrics[i] = metric{
				leftSideBearing:  int16(bo.Uint16(data[base : base+2])),
				rightSideBearing: int16(bo.Uint16(data[base+2 : base+4])),
				characterWidth:   int16(bo.Uint16(data[base+4 : base+6])),
				characterAscent:  int16(bo.Uint16(data[base+6 : base+8])),
				characterDescent: int16(bo.Uint16(data[base+8 : base+10])),
			}
		}
	}
	return metrics, nil
}

func parseBitmaps(data []byte, off int, glyphs []Glyph, metrics []metric) error {
	if off+8 > len(data) {
		return FormatError("bitmaps header truncated")
	}
	format := binary.LittleEndian.Uint32(data[off : off+4])
	bo := byteOrder(format)
	count := int(bo.Uint32(data[off+4 : off+8]))
	if count != len(glyphs) {
		return FormatError("bitmaps count mismatch")
	}

	headerEnd := off + 8
	if headerEnd+4*count+16 > len(data) {
		return FormatError("bitmaps offsets truncated")
	}
	offsets := make([]uint32, count)
	for i := 0; i < count; i++ {
		offsets[i] = bo.Uint32(data[headerEnd+4*i : headerEnd+4*i+4])
	}
	sizesOff := headerEnd + 4*count
	// bitmapSizes[4] lists total bytes for each of the four pad variants.
	padIdx := format & fmtGlyphPadMask
	bitmapsStart := sizesOff + 16

	padBytes := 1 << padIdx
	msbFirst := format&fmtBitMask != 0

	for i := 0; i < count; i++ {
		m := metrics[i]
		w := int(m.rightSideBearing - m.leftSideBearing)
		h := int(m.characterAscent + m.characterDescent)
		if w <= 0 || h <= 0 {
			continue
		}
		stride := ((w + 8*padBytes - 1) / (8 * padBytes)) * padBytes
		start := bitmapsStart + int(offsets[i])
		end := start + stride*h
		if end > len(data) {
			continue
		}
		bm := raster.NewBitmap(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			row := data[start+y*stride : start+y*stride+stride]
			for x := 0; x < w; x++ {
				byteIdx := x / 8
				bitIdx := uint(x % 8)
				var bit byte
				if msbFirst {
					bit = row[byteIdx] >> (7 - bitIdx) & 1
				} else {
					bit = row[byteIdx] >> bitIdx & 1
				}
				if bit != 0 {
					bm.SetBit(x, y, true)
				}
			}
		}
		glyphs[i].Bitmap = bm
	}
	return nil
}

func parseEncodings(data []byte, off int, font *Font) error {
	if off+14 > len(data) {
		return FormatError("encodings header truncated")
	}
	format := binary.LittleEndian.Uint32(data[off : off+4])
	bo := byteOrder(format)
	minCharOrByte2 := int(bo.Uint16(data[off+4 : off+6]))
	maxCharOrByte2 := int(bo.Uint16(data[off+6 : off+8]))
	minByte1 := int(bo.Uint16(data[off+8 : off+10]))
	maxByte1 := int(bo.Uint16(data[off+10 : off+12]))
	// defaultChar at off+12 — not needed.

	cellsPerRow := maxCharOrByte2 - minCharOrByte2 + 1
	rows := maxByte1 - minByte1 + 1
	total := cellsPerRow * rows
	start := off + 14
	if start+2*total > len(data) {
		return FormatError("encodings body truncated")
	}
	for r := 0; r < rows; r++ {
		for c := 0; c < cellsPerRow; c++ {
			gid := bo.Uint16(data[start+2*(r*cellsPerRow+c) : start+2*(r*cellsPerRow+c)+2])
			if gid == 0xFFFF {
				continue
			}
			// Reconstruct the codepoint: for single-byte fonts minByte1 == 0
			// and the encoding is just (minCharOrByte2 + c). For two-byte
			// fonts it's ((minByte1+r) << 8) | (minCharOrByte2 + c).
			var codepoint int
			if rows == 1 && minByte1 == 0 {
				codepoint = minCharOrByte2 + c
			} else {
				codepoint = ((minByte1+r)&0xFF)<<8 | ((minCharOrByte2 + c) & 0xFF)
			}
			if int(gid) < len(font.Glyphs) {
				font.Glyphs[gid].Encoding = rune(codepoint)
				font.runeToIndex[rune(codepoint)] = int(gid)
			}
		}
	}
	return nil
}

func parseAccelerators(data []byte, off int, font *Font) {
	if off+100 > len(data) {
		return
	}
	format := binary.LittleEndian.Uint32(data[off : off+4])
	bo := byteOrder(format)
	// Layout (partial): noOverlap(1), constantMetrics(1), terminalFont(1),
	// constantWidth(1), inkInside(1), inkMetrics(1), drawDirection(1),
	// padding(1), fontAscent(4), fontDescent(4), ...
	font.FontAscent = int(int32(bo.Uint32(data[off+12 : off+16])))
	font.FontDescent = int(int32(bo.Uint32(data[off+16 : off+20])))
}

// byteOrder picks little- or big-endian based on the PCF format flag.
func byteOrder(format uint32) binary.ByteOrder {
	if format&fmtByteMask != 0 {
		return binary.BigEndian
	}
	return binary.LittleEndian
}

// Glyph returns the glyph for rune r, or nil if absent.
func (f *Font) Glyph(r rune) *Glyph {
	if f == nil {
		return nil
	}
	idx, ok := f.runeToIndex[r]
	if !ok {
		return nil
	}
	return &f.Glyphs[idx]
}
