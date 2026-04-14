// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import (
	"image"
	"image/draw"
	"testing"

	"github.com/KarpelesLab/gofreetype/cff"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// buildCFFWithCharStrings assembles a minimal CFF v1 table containing the
// given charstrings, each as the bytecode body of one glyph. The font has
// no subroutines, no encodings, and a trivial Private DICT.
func buildCFFWithCharStrings(charStringObjs [][]byte) []byte {
	encIdx := func(objs [][]byte) []byte {
		count := len(objs)
		if count == 0 {
			return []byte{0, 0}
		}
		offsets := []int{1}
		for _, o := range objs {
			offsets = append(offsets, offsets[len(offsets)-1]+len(o))
		}
		offSize := 1
		switch {
		case offsets[count] > 1<<16:
			offSize = 3
		case offsets[count] > 255:
			offSize = 2
		}
		buf := []byte{byte(count >> 8), byte(count), byte(offSize)}
		for _, o := range offsets {
			for s := offSize - 1; s >= 0; s-- {
				buf = append(buf, byte(o>>(8*s)))
			}
		}
		for _, o := range objs {
			buf = append(buf, o...)
		}
		return buf
	}
	encInt := func(v int) []byte {
		if v >= -107 && v <= 107 {
			return []byte{byte(v + 139)}
		}
		return []byte{28, byte(v >> 8), byte(v)}
	}

	empty := encIdx(nil)
	nameIdx := encIdx([][]byte{[]byte("T")})
	csIdx := encIdx(charStringObjs)

	priv := func(subrsOff int) []byte {
		var b []byte
		b = append(b, encInt(subrsOff)...)
		b = append(b, 19)
		return b
	}
	privSize := len(priv(0))
	pv := priv(privSize)

	encTop := func(csOff, privOff int) []byte {
		var b []byte
		b = append(b, encInt(csOff)...)
		b = append(b, 17)
		b = append(b, encInt(privSize)...)
		b = append(b, encInt(privOff)...)
		b = append(b, 18)
		return b
	}
	csOff, privOff := 0, 0
	var topDict []byte
	for iter := 0; iter < 8; iter++ {
		topDict = encTop(csOff, privOff)
		topIdx := encIdx([][]byte{topDict})
		hdr := 4
		nameOff := hdr + 0
		topOff := nameOff + len(nameIdx)
		stringsOff := topOff + len(topIdx)
		globalSubrsOff := stringsOff + len(empty)
		csNew := globalSubrsOff + len(empty)
		privNew := csNew + len(csIdx)
		if csNew == csOff && privNew == privOff {
			break
		}
		csOff, privOff = csNew, privNew
	}
	topIdx := encIdx([][]byte{topDict})

	var out []byte
	out = append(out, 1, 0, 4, 2)
	out = append(out, nameIdx...)
	out = append(out, topIdx...)
	out = append(out, empty...)
	out = append(out, empty...)
	out = append(out, csIdx...)
	out = append(out, pv...)
	out = append(out, empty...)
	return out
}

// buildOTF with the given CFF blob and a minimal set of companion tables
// tuned for CFF glyph loading: head.unitsPerEm=1000, hmtx with one hMetric
// (advance=500, lsb=0) per glyph, hhea with that count, plus a 1-glyph maxp
// v0.5 — except we can override the glyph count by supplying a specific
// nGlyphs.
func buildOTFForCFF(nGlyphs int, cffTable []byte) []byte {
	tables := minimalCFFTables()
	// Update maxp (position 5) to reflect the actual glyph count.
	for i := range tables {
		switch tables[i].tag {
		case "maxp":
			b := make([]byte, 6)
			b[0], b[1], b[2], b[3] = 0, 0, 0x50, 0x00
			b[4] = byte(nGlyphs >> 8)
			b[5] = byte(nGlyphs)
			tables[i].data = b
		case "hhea":
			b := make([]byte, 36)
			// ascent / descent / linegap
			b[4], b[5] = 0x03, 0x20   // 800
			b[6], b[7] = 0xff, 0x38   // -200
			b[8], b[9] = 0x00, 0x64   // 100
			b[18], b[19] = 0x00, 0x01 // caretSlopeRise
			b[34] = byte(nGlyphs >> 8)
			b[35] = byte(nGlyphs)
			tables[i].data = b
		case "hmtx":
			// 4 bytes per glyph: advanceWidth (u16) + lsb (s16).
			b := make([]byte, 4*nGlyphs)
			for g := 0; g < nGlyphs; g++ {
				b[4*g] = 0x02 // advance = 512
				b[4*g+1] = 0
				b[4*g+2] = 0
				b[4*g+3] = 0
			}
			tables[i].data = b
		case "CFF ":
			tables[i].data = cffTable
		}
	}
	return buildMinimalSFNT(0x4F54544F, tables)
}

// rlineto encoded as 2 operands + op 5. Helper to build concise charstrings.
func cspush(cs []byte, v int) []byte {
	if v >= -107 && v <= 107 {
		return append(cs, byte(v+139))
	}
	// 28 short-int form.
	return append(cs, 28, byte(v>>8), byte(v))
}

func TestCFFLoadRectangle(t *testing.T) {
	// Build a charstring that draws a 100x50 rectangle starting at (0, 0).
	// rmoveto: 0, 0
	// rlineto: 100, 0
	// rlineto: 0, 50
	// rlineto: -100, 0
	// endchar
	var cs []byte
	cs = cspush(cs, 0)
	cs = cspush(cs, 0)
	cs = append(cs, 21) // rmoveto
	cs = cspush(cs, 100)
	cs = cspush(cs, 0)
	cs = cspush(cs, 0)
	cs = cspush(cs, 50)
	cs = cspush(cs, -100)
	cs = cspush(cs, 0)
	cs = append(cs, 5)  // rlineto (3 segments)
	cs = append(cs, 14) // endchar

	cffBlob := buildCFFWithCharStrings([][]byte{cs})
	otf := buildOTFForCFF(1, cffBlob)
	f, err := Parse(otf)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Kind() != FontKindCFF {
		t.Fatalf("Kind: got %d, want %d", f.Kind(), FontKindCFF)
	}

	// Load glyph 0 at scale = fUnitsPerEm (so CFF units == pixel units).
	scale := fixed.Int26_6(f.FUnitsPerEm()) << 6
	g := &GlyphBuf{}
	if err := g.Load(f, scale, 0, font.HintingNone); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(g.Segments) != 4 {
		t.Fatalf("Segments: got %d, want 4", len(g.Segments))
	}
	want := []cff.SegmentOp{cff.SegMoveTo, cff.SegLineTo, cff.SegLineTo, cff.SegLineTo}
	for i, op := range want {
		if g.Segments[i].Op != op {
			t.Errorf("segment[%d] op: got %d, want %d", i, g.Segments[i].Op, op)
		}
	}
	// Final lineto should bring the pen back to (0, 50) in font units.
	last := g.Segments[3]
	wantX := fixed.Int26_6(0)
	wantY := fixed.Int26_6(50 << 6) // 50 font units * 64 (26.6 unit)
	if last.X != wantX || last.Y != wantY {
		t.Errorf("final lineto: got (%v, %v), want (%v, %v)", last.X, last.Y, wantX, wantY)
	}

	// Advance width comes from hmtx: 512 FUnits. At scale == fUnitsPerEm pixels
	// per em, 1 FUnit == 1 pixel, so advance == 512 * 64 = 32768 (26.6 units).
	if got, want := g.AdvanceWidth, fixed.Int26_6(512<<6); got != want {
		t.Errorf("AdvanceWidth: got %v, want %v", got, want)
	}
}

func TestCFFRenderRectangle(t *testing.T) {
	// Draw a 200x100 rectangle glyph, render it through face.Glyph, and
	// verify the resulting mask has non-zero coverage somewhere inside the
	// expected region.
	var cs []byte
	cs = cspush(cs, 100)
	cs = cspush(cs, 100)
	cs = append(cs, 21)
	cs = cspush(cs, 200)
	cs = cspush(cs, 0)
	cs = cspush(cs, 0)
	cs = cspush(cs, 100)
	cs = cspush(cs, -200)
	cs = cspush(cs, 0)
	cs = append(cs, 5)
	cs = append(cs, 14)

	cffBlob := buildCFFWithCharStrings([][]byte{cs})
	otf := buildOTFForCFF(1, cffBlob)
	f, err := Parse(otf)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	face := NewFace(f, &Options{Size: float64(f.FUnitsPerEm()), DPI: 72})
	defer face.Close()

	// Place the baseline well inside a 1500x1500 canvas so the glyph lands
	// entirely within the image. The dot is in 26.6 fixed-point pixels.
	dot := fixed.Point26_6{X: fixed.I(200), Y: fixed.I(1000)}
	dr, mask, maskp, _, ok := face.Glyph(dot, 'A')
	if !ok {
		t.Fatal("face.Glyph returned !ok")
	}
	if dr.Empty() {
		t.Fatal("face.Glyph produced an empty destination rect")
	}

	dst := image.NewRGBA(image.Rect(0, 0, 1500, 1500))
	draw.Draw(dst, dst.Bounds(), image.Transparent, image.Point{}, draw.Src)
	// Honor the dr rectangle and maskp offset as per the font.Face contract.
	draw.DrawMask(dst, dr, image.Black, image.Point{}, mask, maskp, draw.Over)

	nonZero := 0
	for y := 0; y < dst.Rect.Dy(); y++ {
		for x := 0; x < dst.Rect.Dx(); x++ {
			if _, _, _, a := dst.At(x, y).RGBA(); a > 0 {
				nonZero++
			}
		}
	}
	if nonZero == 0 {
		t.Errorf("rasterization produced no non-zero pixels for a 200x100 rectangle; dr=%v maskp=%v", dr, maskp)
	}
}
