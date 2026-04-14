// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"

	"golang.org/x/image/math/fixed"
)

// TestBitmapGlyphNoTables verifies the negative path: a plain mono font
// returns ok=false from BitmapGlyph.
func TestBitmapGlyphNoTables(t *testing.T) {
	f, _, err := parseTestdataFont("luxisr")
	if err != nil {
		t.Fatal(err)
	}
	face := NewFace(f, &Options{Size: 12, DPI: 72}).(*face)
	_, _, _, _, ok := face.BitmapGlyph(fixed.Point26_6{}, 'A')
	if ok {
		t.Error("BitmapGlyph on mono font: ok=true, want false")
	}
}

// TestBitmapGlyphSbix builds a synthetic font with an sbix table
// containing a 5x5 all-red PNG for glyph 1 (mapped from 'A') and checks
// that face.BitmapGlyph decodes and positions it.
func TestBitmapGlyphSbix(t *testing.T) {
	otf := buildSFNTWithSbix(t)
	f, err := Parse(otf)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Sbix() == nil {
		t.Fatal("Sbix not parsed")
	}
	face := NewFace(f, &Options{Size: float64(f.FUnitsPerEm()), DPI: 72}).(*face)
	dr, img, _, _, ok := face.BitmapGlyph(fixed.Point26_6{X: fixed.I(100), Y: fixed.I(800)}, 'A')
	if !ok {
		t.Fatal("BitmapGlyph: ok=false")
	}
	if dr.Empty() {
		t.Fatal("dr is empty")
	}
	if img == nil {
		t.Fatal("img is nil")
	}
	if got := dr.Dx(); got != 5 {
		t.Errorf("dr width: got %d, want 5", got)
	}
}

// buildSFNTWithSbix constructs a minimal TTF with an sbix table. The
// glyph for 'A' resolves to glyph id 1 (outline is an empty rectangle;
// we only exercise the sbix path).
func buildSFNTWithSbix(t *testing.T) []byte {
	t.Helper()
	// Build a 5x5 all-red PNG.
	img := image.NewRGBA(image.Rect(0, 0, 5, 5))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+0] = 0xff
		img.Pix[i+1] = 0
		img.Pix[i+2] = 0
		img.Pix[i+3] = 0xff
	}
	_ = color.RGBA{} // force image/color import for clarity
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	pngBytes := buf.Bytes()

	// sbix table: version(2) + flags(2) + numStrikes(4) +
	// strikeOffsets[1] + strike.
	//
	// Strike: ppem(2) + ppi(2) + offsets[numGlyphs+1] + data.
	// numGlyphs = 2 (0 = .notdef, 1 = 'A').
	strikeHeaderLen := 4 + 4*(2+1) // 4 for ppem+ppi, 4 per offset
	// Glyph 0 has no data; glyph 1 has originX(2) + originY(2) + type(4) + data.
	glyph1 := make([]byte, 8+len(pngBytes))
	binary.BigEndian.PutUint16(glyph1[0:], 0) // originOffsetX
	binary.BigEndian.PutUint16(glyph1[2:], 0) // originOffsetY
	copy(glyph1[4:8], "png ")
	copy(glyph1[8:], pngBytes)

	offsets := []uint32{
		uint32(strikeHeaderLen),               // glyph 0 starts (empty)
		uint32(strikeHeaderLen),               // glyph 1 starts
		uint32(strikeHeaderLen + len(glyph1)), // end sentinel
	}
	strikeLen := strikeHeaderLen + len(glyph1)
	sbixHeaderLen := 8 + 4 // 1 strike offset
	strikeOff := sbixHeaderLen

	sbixTable := make([]byte, sbixHeaderLen+strikeLen)
	binary.BigEndian.PutUint16(sbixTable[0:], 1) // version
	binary.BigEndian.PutUint16(sbixTable[2:], 0) // flags
	binary.BigEndian.PutUint32(sbixTable[4:], 1) // numStrikes
	binary.BigEndian.PutUint32(sbixTable[8:], uint32(strikeOff))

	s := strikeOff
	binary.BigEndian.PutUint16(sbixTable[s:], 1000) // ppem = FUnitsPerEm
	binary.BigEndian.PutUint16(sbixTable[s+2:], 72) // ppi
	for i, o := range offsets {
		binary.BigEndian.PutUint32(sbixTable[s+4+4*i:], o)
	}
	copy(sbixTable[s+strikeHeaderLen:], glyph1)

	// Build minimal SFNT: reuse helpers from kind_test.go / face_color_test.go.
	tables := []struct {
		tag  string
		data []byte
	}{
		{"cmap", buildCmapFormat4For('A', 1)},
		{"head", buildMinimalHead()},
		{"hhea", buildMinimalHhea(2)},
		{"hmtx", buildMinimalHmtx(2, 500)},
		{"loca", []byte{0, 0, 0, 0, 0, 0}}, // 3 zero offsets = 2 empty glyphs
		{"maxp", buildMinimalMaxp(2)},
		{"glyf", []byte{}},
		{"sbix", sbixTable},
	}
	return buildMinimalSFNT(0x00010000, tables)
}

func buildMinimalHead() []byte {
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:], 1000)
	var xMin, yMin int16 = -100, -100
	binary.BigEndian.PutUint16(head[36:], uint16(xMin))
	binary.BigEndian.PutUint16(head[38:], uint16(yMin))
	binary.BigEndian.PutUint16(head[40:], 1000)
	binary.BigEndian.PutUint16(head[42:], 1000)
	return head
}

func buildMinimalHhea(numMetrics int) []byte {
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[4:], 800)
	binary.BigEndian.PutUint16(hhea[6:], 0xff38)
	binary.BigEndian.PutUint16(hhea[18:], 1)
	binary.BigEndian.PutUint16(hhea[34:], uint16(numMetrics))
	return hhea
}

func buildMinimalHmtx(numMetrics int, advance uint16) []byte {
	b := make([]byte, 4*numMetrics)
	for i := 0; i < numMetrics; i++ {
		binary.BigEndian.PutUint16(b[4*i:], advance)
		binary.BigEndian.PutUint16(b[4*i+2:], 0)
	}
	return b
}

func buildMinimalMaxp(numGlyphs int) []byte {
	b := make([]byte, 32)
	binary.BigEndian.PutUint32(b[0:], 0x00010000)
	binary.BigEndian.PutUint16(b[4:], uint16(numGlyphs))
	return b
}
