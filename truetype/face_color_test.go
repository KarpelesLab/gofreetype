// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import (
	"encoding/binary"
	imgcolor "image/color"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// TestColorGlyphReturnsFalseForMonochrome exercises the negative path:
// with no COLR/CPAL tables present, ColorGlyph must report ok=false so
// callers know to fall back to the regular alpha pipeline.
func TestColorGlyphReturnsFalseForMonochrome(t *testing.T) {
	f, _, err := parseTestdataFont("luxisr")
	if err != nil {
		t.Fatal(err)
	}
	face := NewFace(f, &Options{Size: 12, DPI: 72}).(*face)
	_, _, _, _, ok := face.ColorGlyph(fixed.Point26_6{}, 'A', 0, nil)
	if ok {
		t.Error("ColorGlyph on a mono font: ok=true, want false")
	}
}

// TestColorGlyphRendersLayers builds a synthetic TTF with COLR + CPAL
// tables and verifies ColorGlyph composites at least one layer's worth
// of colored pixels into the output RGBA.
func TestColorGlyphRendersLayers(t *testing.T) {
	otf := buildTTFWithColor(t)
	f, err := Parse(otf)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.COLR() == nil || f.CPAL() == nil {
		t.Fatal("COLR/CPAL not parsed from synthetic font")
	}

	face := NewFace(f, &Options{Size: float64(f.FUnitsPerEm()), DPI: 72, Hinting: font.HintingNone}).(*face)
	dot := fixed.Point26_6{X: fixed.I(100), Y: fixed.I(800)}
	dr, rgba, _, _, ok := face.ColorGlyph(dot, 'A', 0, nil)
	if !ok {
		t.Fatal("ColorGlyph: ok=false, want true")
	}
	if dr.Empty() {
		t.Fatal("ColorGlyph dr is empty")
	}
	if rgba == nil {
		t.Fatal("ColorGlyph rgba is nil")
	}

	// Count pixels with non-zero alpha. The test font has a solid red
	// base glyph, so we should see many red pixels.
	redPixels := 0
	for y := 0; y < rgba.Bounds().Dy(); y++ {
		for x := 0; x < rgba.Bounds().Dx(); x++ {
			off := rgba.PixOffset(x, y)
			r, g, b, a := rgba.Pix[off], rgba.Pix[off+1], rgba.Pix[off+2], rgba.Pix[off+3]
			if a > 0 && r > 0 && g == 0 && b == 0 {
				redPixels++
			}
		}
	}
	if redPixels == 0 {
		t.Error("no red pixels rendered — COLR layer didn't composite")
	}
}

// buildTTFWithColor builds a minimal TrueType font that maps 'A' to a
// color glyph whose single layer is a filled rectangle colored by the
// first CPAL palette entry (pure red).
//
// Structure:
//   - glyph 0: .notdef (empty)
//   - glyph 1: a filled rectangle (acts as the outline for the color layer)
//   - cmap maps 'A' (gid) to 1 via a minimal format 4
//   - COLR v0 with one BaseGlyphRecord for gid 1 -> one layer referencing gid 1, palette index 0
//   - CPAL with one palette of two colors (red, blue)
func buildTTFWithColor(t *testing.T) []byte {
	t.Helper()
	// Build the glyf table: two glyphs. glyf entries in the table are
	// concatenated; the loca table indexes them.
	//
	// Glyph 0 (.notdef): empty glyph (0 bytes).
	// Glyph 1: one contour with 4 on-curve points forming a 400x400 square
	//   from (100,100) to (500,500) in FUnits.
	g0 := []byte{} // empty glyph per OpenType convention
	g1 := buildFilledRectGlyph(100, 100, 500, 500)

	// loca (short format): offsets in units of 2 bytes from glyf start.
	// We have 2 glyphs so loca has 3 entries. Pad g1 to even length.
	for len(g1)%2 != 0 {
		g1 = append(g1, 0)
	}
	loca := make([]byte, 6)
	binary.BigEndian.PutUint16(loca[0:], 0)
	binary.BigEndian.PutUint16(loca[2:], uint16(len(g0)/2))
	binary.BigEndian.PutUint16(loca[4:], uint16((len(g0)+len(g1))/2))
	glyfTable := append([]byte{}, g0...)
	glyfTable = append(glyfTable, g1...)

	// cmap: format 4, mapping 'A' (0x41) -> gid 1.
	cmap := buildCmapFormat4For('A', 1)

	// hmtx: 4 bytes per glyph (all glyphs as full hMetrics).
	hmtx := make([]byte, 8)
	binary.BigEndian.PutUint16(hmtx[0:], 600) // glyph 0 advance
	binary.BigEndian.PutUint16(hmtx[2:], 0)   // glyph 0 lsb
	binary.BigEndian.PutUint16(hmtx[4:], 600) // glyph 1 advance
	binary.BigEndian.PutUint16(hmtx[6:], 100) // glyph 1 lsb

	// head: 54 bytes with unitsPerEm=1000 and loose bbox.
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:], 1000) // unitsPerEm
	var xMin, yMin int16 = -100, -100
	binary.BigEndian.PutUint16(head[36:], uint16(xMin))
	binary.BigEndian.PutUint16(head[38:], uint16(yMin))
	binary.BigEndian.PutUint16(head[40:], 1000)
	binary.BigEndian.PutUint16(head[42:], 1000)
	// indexToLocFormat = 0 (short) at offset 50 — already zero.

	// hhea: ascent/descent/etc.
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[4:], 800)
	binary.BigEndian.PutUint16(hhea[6:], 0xff38) // -200
	binary.BigEndian.PutUint16(hhea[18:], 1)
	binary.BigEndian.PutUint16(hhea[34:], 2) // numberOfHMetrics = 2

	// maxp v1.0 (32 bytes).
	maxp := make([]byte, 32)
	binary.BigEndian.PutUint32(maxp[0:], 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:], 2) // numGlyphs

	// CPAL: 1 palette, 2 entries: (red, blue).
	cpal := buildMinimalCPAL()

	// COLR v0: glyph 1 -> one layer (glyph 1, palette 0).
	colr := buildMinimalCOLR()

	tables := []struct {
		tag  string
		data []byte
	}{
		{"cmap", cmap},
		{"head", head},
		{"hhea", hhea},
		{"hmtx", hmtx},
		{"loca", loca},
		{"maxp", maxp},
		{"glyf", glyfTable},
		{"CPAL", cpal},
		{"COLR", colr},
	}
	return buildMinimalSFNT(0x00010000, tables)
}

// buildFilledRectGlyph returns a glyf-format glyph body for a single
// filled rectangle with four on-curve corners.
func buildFilledRectGlyph(x1, y1, x2, y2 int16) []byte {
	// Header: numberOfContours(1) + xMin + yMin + xMax + yMax.
	hdr := make([]byte, 10)
	binary.BigEndian.PutUint16(hdr[0:], 1) // numberOfContours
	binary.BigEndian.PutUint16(hdr[2:], uint16(x1))
	binary.BigEndian.PutUint16(hdr[4:], uint16(y1))
	binary.BigEndian.PutUint16(hdr[6:], uint16(x2))
	binary.BigEndian.PutUint16(hdr[8:], uint16(y2))

	// endPtsOfContours[numberOfContours]: last point index = 3.
	ends := []byte{0, 3}

	// instructionLength = 0.
	instrLen := []byte{0, 0}

	// flags: 4 points, all on-curve (0x01). We'll use one repeated flag.
	// Flag byte 0x37 = on-curve (0x01) + X short (0x02) + Y short (0x04) +
	// repeat (0x08) + X positive (0x10) + Y positive (0x20).
	// Simpler: use full 4 flag bytes, one per point, 0x01 each (on-curve),
	// then X / Y as int16.
	flags := []byte{0x01, 0x01, 0x01, 0x01}

	// X coordinates (int16, each relative to previous).
	// Absolute: x1, x2, x2, x1. Relative: x1, x2-x1, 0, x1-x2.
	xs := make([]byte, 8)
	binary.BigEndian.PutUint16(xs[0:], uint16(x1))
	binary.BigEndian.PutUint16(xs[2:], uint16(x2-x1))
	binary.BigEndian.PutUint16(xs[4:], 0)
	binary.BigEndian.PutUint16(xs[6:], uint16(x1-x2))

	// Y: absolute y1, y1, y2, y2. Relative y1, 0, y2-y1, 0.
	ys := make([]byte, 8)
	binary.BigEndian.PutUint16(ys[0:], uint16(y1))
	binary.BigEndian.PutUint16(ys[2:], 0)
	binary.BigEndian.PutUint16(ys[4:], uint16(y2-y1))
	binary.BigEndian.PutUint16(ys[6:], 0)

	out := append([]byte{}, hdr...)
	out = append(out, ends...)
	out = append(out, instrLen...)
	out = append(out, flags...)
	out = append(out, xs...)
	out = append(out, ys...)
	return out
}

// buildCmapFormat4For returns a full cmap table mapping codepoint `r` to
// glyph id `gid`. Uses a sentinel [0xffff, 0xffff] segment as required by
// the format.
func buildCmapFormat4For(r rune, gid uint16) []byte {
	// cmap table: version + numSubtables + record + subtable body.
	//
	// Segments: [start=r, end=r, delta=gid-r], [start=0xffff, end=0xffff, delta=1].
	segs := []struct{ start, end, delta uint16 }{
		{uint16(r), uint16(r), gid - uint16(r)},
		{0xffff, 0xffff, 1},
	}
	segCount := len(segs)
	bodyLen := 16 + 8*segCount
	body := make([]byte, bodyLen)
	binary.BigEndian.PutUint16(body[0:], 4)
	binary.BigEndian.PutUint16(body[2:], uint16(bodyLen))
	binary.BigEndian.PutUint16(body[4:], 0) // language
	binary.BigEndian.PutUint16(body[6:], uint16(2*segCount))
	off := 14
	for _, s := range segs {
		binary.BigEndian.PutUint16(body[off:], s.end)
		off += 2
	}
	off += 2 // reservedPad
	for _, s := range segs {
		binary.BigEndian.PutUint16(body[off:], s.start)
		off += 2
	}
	for _, s := range segs {
		binary.BigEndian.PutUint16(body[off:], s.delta)
		off += 2
	}

	// cmap table: version + numSubtables + (PID, PSID, offset) + body.
	out := []byte{
		0, 0, // version
		0, 1, // numSubtables
		0, 0, 0, 3, // PID=0 PSID=3 (Unicode BMP)
		0, 0, 0, 12, // offset
	}
	out = append(out, body...)
	return out
}

// buildMinimalCPAL returns a CPAL v0 with one palette of two entries:
// pure red (palette index 0) and pure blue (palette index 1).
func buildMinimalCPAL() []byte {
	// Header: version + numPaletteEntries(2) + numPalettes(1) + numColorRecords(2) + colorRecordsArrayOffset + colorRecordIndices[1].
	headerLen := 14 // 12 + 2 (one palette index)
	colorRecordsOff := headerLen
	totalLen := headerLen + 8 // 2 colors * 4 bytes

	b := make([]byte, totalLen)
	binary.BigEndian.PutUint16(b[0:], 0)
	binary.BigEndian.PutUint16(b[2:], 2)
	binary.BigEndian.PutUint16(b[4:], 1)
	binary.BigEndian.PutUint16(b[6:], 2)
	binary.BigEndian.PutUint32(b[8:], uint32(colorRecordsOff))
	binary.BigEndian.PutUint16(b[12:], 0)

	// Colors: BGRA on disk. Red = (0,0,255,255). Blue = (255,0,0,255).
	b[14] = 0
	b[15] = 0
	b[16] = 255
	b[17] = 255
	b[18] = 255
	b[19] = 0
	b[20] = 0
	b[21] = 255
	return b
}

// buildMinimalCOLR returns a COLR v0 with one base glyph record mapping
// glyph 1 to one layer (glyph 1 rasterized with palette index 0).
func buildMinimalCOLR() []byte {
	headerLen := 14
	baseOff := headerLen
	layerOff := baseOff + 6
	totalLen := layerOff + 4

	out := make([]byte, totalLen)
	binary.BigEndian.PutUint16(out[0:], 0)
	binary.BigEndian.PutUint16(out[2:], 1) // numBaseGlyphRecords
	binary.BigEndian.PutUint32(out[4:], uint32(baseOff))
	binary.BigEndian.PutUint32(out[8:], uint32(layerOff))
	binary.BigEndian.PutUint16(out[12:], 1) // numLayerRecords

	// Base glyph record: glyphID=1, firstLayer=0, numLayers=1.
	binary.BigEndian.PutUint16(out[baseOff:], 1)
	binary.BigEndian.PutUint16(out[baseOff+2:], 0)
	binary.BigEndian.PutUint16(out[baseOff+4:], 1)

	// Layer: glyphID=1, paletteIndex=0.
	binary.BigEndian.PutUint16(out[layerOff:], 1)
	binary.BigEndian.PutUint16(out[layerOff+2:], 0)
	return out
}

// Verify that ColorGlyph's foreground fallback path is exercised when
// a layer names palette index 0xFFFF.
func TestColorGlyphForegroundFallback(t *testing.T) {
	// Patch a COLR table where the layer palette index is 0xFFFF.
	otf := buildTTFWithColor(t)
	// Replace the layer palette index with 0xFFFF. Find the COLR table
	// in the directory and patch in-place.
	// (Simpler: build a custom COLR here.)
	// Locate COLR in otf by scanning the 16-byte table records starting
	// at offset 12. Each record: tag(4) + checksum(4) + offset(4) + length(4).
	numTables := int(binary.BigEndian.Uint16(otf[4:6]))
	for i := 0; i < numTables; i++ {
		rec := 12 + 16*i
		if string(otf[rec:rec+4]) != "COLR" {
			continue
		}
		off := int(binary.BigEndian.Uint32(otf[rec+8 : rec+12]))
		// LayerRecord lives at off+20 (COLR header 14 + base record 6).
		// PaletteIndex is at LayerRecord+2.
		binary.BigEndian.PutUint16(otf[off+20+2:], 0xFFFF)
		break
	}

	f, err := Parse(otf)
	if err != nil {
		t.Fatal(err)
	}
	face := NewFace(f, &Options{Size: float64(f.FUnitsPerEm()), DPI: 72}).(*face)
	// Pass a green foreground color.
	green := imgcolor.NRGBA{R: 0, G: 255, B: 0, A: 255}
	_, rgba, _, _, ok := face.ColorGlyph(fixed.Point26_6{X: fixed.I(100), Y: fixed.I(800)}, 'A', 0, green)
	if !ok {
		t.Fatal("ColorGlyph: ok=false, want true")
	}
	// Expect some green pixels.
	greenPixels := 0
	for y := 0; y < rgba.Bounds().Dy(); y++ {
		for x := 0; x < rgba.Bounds().Dx(); x++ {
			off := rgba.PixOffset(x, y)
			r, g, b, a := rgba.Pix[off], rgba.Pix[off+1], rgba.Pix[off+2], rgba.Pix[off+3]
			if a > 0 && g > 0 && r == 0 && b == 0 {
				greenPixels++
			}
		}
	}
	if greenPixels == 0 {
		t.Error("no green pixels rendered — foreground color fallback broken")
	}
}
