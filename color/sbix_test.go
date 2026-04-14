// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package color

import (
	"encoding/binary"
	"testing"
)

// buildSbix constructs a synthetic sbix table with one strike containing
// the given per-glyph image entries. numGlyphs is the font's glyph count.
func buildSbix(ppem, ppi uint16, numGlyphs int, glyphs map[int]SbixGlyph) []byte {
	// Layout: sbix header (8) + 1 strike offset (4) + strike.
	headerLen := 8 + 4 // 1 strike

	// Strike: ppem(2) + ppi(2) + offsets(4*(numGlyphs+1)) + glyph data.
	strikeHeaderLen := 4 + 4*(numGlyphs+1)
	var glyphData []byte
	offsets := make([]uint32, numGlyphs+1)
	for gid := 0; gid < numGlyphs; gid++ {
		offsets[gid] = uint32(strikeHeaderLen + len(glyphData))
		g, ok := glyphs[gid]
		if !ok {
			continue
		}
		// GlyphData: originX(2) + originY(2) + type(4) + data.
		var rec []byte
		rec = binary.BigEndian.AppendUint16(rec, uint16(g.OriginOffsetX))
		rec = binary.BigEndian.AppendUint16(rec, uint16(g.OriginOffsetY))
		rec = append(rec, g.GraphicType[:]...)
		rec = append(rec, g.Data...)
		glyphData = append(glyphData, rec...)
	}
	offsets[numGlyphs] = uint32(strikeHeaderLen + len(glyphData))

	strikeLen := strikeHeaderLen + len(glyphData)
	strikeOff := headerLen

	total := headerLen + strikeLen
	out := make([]byte, total)
	binary.BigEndian.PutUint16(out[0:], 1) // version
	binary.BigEndian.PutUint16(out[2:], 0) // flags
	binary.BigEndian.PutUint32(out[4:], 1) // numStrikes
	binary.BigEndian.PutUint32(out[8:], uint32(strikeOff))

	sOff := strikeOff
	binary.BigEndian.PutUint16(out[sOff:], ppem)
	binary.BigEndian.PutUint16(out[sOff+2:], ppi)
	for i, o := range offsets {
		binary.BigEndian.PutUint32(out[sOff+4+4*i:], o)
	}
	copy(out[sOff+strikeHeaderLen:], glyphData)
	return out
}

func TestSbixBasic(t *testing.T) {
	fakeImage := []byte("PNG_DATA_HERE")
	sb, err := ParseSbix(buildSbix(96, 72, 3, map[int]SbixGlyph{
		1: {
			OriginOffsetX: 5,
			OriginOffsetY: -10,
			GraphicType:   [4]byte{'p', 'n', 'g', ' '},
			Data:          fakeImage,
		},
	}), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(sb.Strikes) != 1 || sb.Strikes[0].PPEM != 96 {
		t.Fatalf("Strikes: got %+v", sb.Strikes)
	}
	// FindStrike for ppem=100 should return the only strike (ppem=96).
	strike := sb.FindStrike(100)
	if strike == nil || strike.PPEM != 96 {
		t.Fatal("FindStrike(100) didn't return the 96ppem strike")
	}

	g := strike.Glyph(1)
	if g == nil {
		t.Fatal("Glyph(1) is nil")
	}
	if string(g.Data) != string(fakeImage) {
		t.Errorf("Glyph(1) data: got %q, want %q", g.Data, fakeImage)
	}
	if g.OriginOffsetX != 5 || g.OriginOffsetY != -10 {
		t.Errorf("origin: got (%d, %d), want (5, -10)", g.OriginOffsetX, g.OriginOffsetY)
	}
	if g.GraphicType != [4]byte{'p', 'n', 'g', ' '} {
		t.Errorf("type: got %q, want \"png \"", g.GraphicType)
	}

	// Missing glyph.
	if strike.Glyph(0) != nil {
		t.Error("Glyph(0) should be nil for missing image")
	}
	if strike.Glyph(2) != nil {
		t.Error("Glyph(2) should be nil for missing image")
	}
}

func TestSbixDupe(t *testing.T) {
	fakeImage := []byte("JPEG")
	dupeData := []byte{0, 1} // points at glyph 1
	sb, err := ParseSbix(buildSbix(48, 72, 3, map[int]SbixGlyph{
		1: {
			GraphicType: [4]byte{'j', 'p', 'g', ' '},
			Data:        fakeImage,
		},
		2: {
			GraphicType: [4]byte{'d', 'u', 'p', 'e'},
			Data:        dupeData,
		},
	}), 3)
	if err != nil {
		t.Fatal(err)
	}
	strike := &sb.Strikes[0]
	g := strike.Glyph(2)
	if g == nil {
		t.Fatal("dupe resolution failed: Glyph(2) is nil")
	}
	if string(g.Data) != "JPEG" {
		t.Errorf("dupe resolution: got data %q, want JPEG", g.Data)
	}
}
