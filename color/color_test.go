// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package color

import (
	"encoding/binary"
	"image/color"
	"testing"
)

// buildCPAL builds a synthetic CPAL v0 table with the given palettes.
// Each palette is a slice of BGRA colors (the on-disk byte order).
func buildCPAL(palettes [][]color.NRGBA) []byte {
	if len(palettes) == 0 {
		return nil
	}
	numEntries := len(palettes[0])
	numPalettes := len(palettes)
	totalColors := numPalettes * numEntries

	headerLen := 12 + 2*numPalettes
	colorArrayOff := headerLen
	totalLen := headerLen + 4*totalColors

	b := make([]byte, totalLen)
	binary.BigEndian.PutUint16(b[0:], 0) // version
	binary.BigEndian.PutUint16(b[2:], uint16(numEntries))
	binary.BigEndian.PutUint16(b[4:], uint16(numPalettes))
	binary.BigEndian.PutUint16(b[6:], uint16(totalColors))
	binary.BigEndian.PutUint32(b[8:], uint32(colorArrayOff))

	for i := 0; i < numPalettes; i++ {
		binary.BigEndian.PutUint16(b[12+2*i:], uint16(i*numEntries))
	}
	for i, pal := range palettes {
		for j, c := range pal {
			off := colorArrayOff + 4*(i*numEntries+j)
			b[off] = c.B
			b[off+1] = c.G
			b[off+2] = c.R
			b[off+3] = c.A
		}
	}
	return b
}

func TestParseCPAL(t *testing.T) {
	pal0 := []color.NRGBA{
		{R: 255, G: 0, B: 0, A: 255},
		{R: 0, G: 255, B: 0, A: 128},
	}
	pal1 := []color.NRGBA{
		{R: 0, G: 0, B: 255, A: 255},
		{R: 128, G: 128, B: 128, A: 64},
	}
	data := buildCPAL([][]color.NRGBA{pal0, pal1})
	cpal, err := ParseCPAL(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(cpal.Palettes) != 2 {
		t.Fatalf("Palettes: got %d, want 2", len(cpal.Palettes))
	}
	for i, wantPal := range [][]color.NRGBA{pal0, pal1} {
		for j, want := range wantPal {
			got := cpal.Palettes[i].Colors[j]
			if got != want {
				t.Errorf("palette[%d][%d]: got %v, want %v", i, j, got, want)
			}
		}
	}
}

// buildCOLR builds a synthetic COLR v0 table.
func buildCOLR(bases []struct {
	glyphID    uint16
	layers     []Layer
}) []byte {
	// Flatten layers.
	var allLayers []Layer
	type baseRec struct {
		gid        uint16
		firstLayer uint16
		numLayers  uint16
	}
	baseRecs := make([]baseRec, len(bases))
	for i, b := range bases {
		baseRecs[i] = baseRec{
			gid:        b.glyphID,
			firstLayer: uint16(len(allLayers)),
			numLayers:  uint16(len(b.layers)),
		}
		allLayers = append(allLayers, b.layers...)
	}

	headerLen := 14
	baseOff := headerLen
	layerOff := baseOff + 6*len(baseRecs)
	totalLen := layerOff + 4*len(allLayers)

	out := make([]byte, totalLen)
	binary.BigEndian.PutUint16(out[0:], 0) // version
	binary.BigEndian.PutUint16(out[2:], uint16(len(baseRecs)))
	binary.BigEndian.PutUint32(out[4:], uint32(baseOff))
	binary.BigEndian.PutUint32(out[8:], uint32(layerOff))
	binary.BigEndian.PutUint16(out[12:], uint16(len(allLayers)))

	for i, br := range baseRecs {
		off := baseOff + 6*i
		binary.BigEndian.PutUint16(out[off:], br.gid)
		binary.BigEndian.PutUint16(out[off+2:], br.firstLayer)
		binary.BigEndian.PutUint16(out[off+4:], br.numLayers)
	}
	for i, l := range allLayers {
		off := layerOff + 4*i
		binary.BigEndian.PutUint16(out[off:], l.GlyphID)
		binary.BigEndian.PutUint16(out[off+2:], l.PaletteIndex)
	}
	return out
}

func TestParseCOLR(t *testing.T) {
	data := buildCOLR([]struct {
		glyphID uint16
		layers  []Layer
	}{
		{
			glyphID: 10,
			layers: []Layer{
				{GlyphID: 100, PaletteIndex: 0},
				{GlyphID: 101, PaletteIndex: 1},
			},
		},
		{
			glyphID: 20,
			layers: []Layer{
				{GlyphID: 200, PaletteIndex: 0xFFFF},
			},
		},
	})
	colr, err := ParseCOLR(data)
	if err != nil {
		t.Fatal(err)
	}
	// Glyph 10 should have 2 layers.
	layers := colr.ColorLayers(10)
	if len(layers) != 2 {
		t.Fatalf("ColorLayers(10): got %d layers, want 2", len(layers))
	}
	if layers[0].GlyphID != 100 || layers[0].PaletteIndex != 0 {
		t.Errorf("layer 0: got %+v, want {100, 0}", layers[0])
	}
	if layers[1].GlyphID != 101 || layers[1].PaletteIndex != 1 {
		t.Errorf("layer 1: got %+v, want {101, 1}", layers[1])
	}
	// Glyph 20 should have 1 foreground-colored layer.
	layers = colr.ColorLayers(20)
	if len(layers) != 1 || layers[0].PaletteIndex != 0xFFFF {
		t.Errorf("ColorLayers(20): got %+v, want [{200, 0xFFFF}]", layers)
	}
	// Non-color glyph.
	if colr.ColorLayers(99) != nil {
		t.Error("ColorLayers(99) should be nil")
	}
	if colr.IsColorGlyph(10) != true {
		t.Error("IsColorGlyph(10) should be true")
	}
	if colr.IsColorGlyph(99) != false {
		t.Error("IsColorGlyph(99) should be false")
	}
}
