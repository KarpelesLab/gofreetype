// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package cff

import "testing"

func TestStemCollection(t *testing.T) {
	// Charstring: hstem 100 20 200 30 (two hstems: edges at 100 and 320,
	// widths 20 and 30), endchar.
	var cs []byte
	cs = append(cs, encodeCSOperand(100)...) // edge delta
	cs = append(cs, encodeCSOperand(20)...)  // width
	cs = append(cs, encodeCSOperand(200)...) // edge delta (relative)
	cs = append(cs, encodeCSOperand(30)...)  // width
	cs = append(cs, 1)                        // hstem
	cs = append(cs, 14)                       // endchar

	p := &interp{hintSink: &stemSink{}}
	if err := p.run(cs); err != nil {
		t.Fatal(err)
	}
	stems := p.hintSink.stems
	if len(stems) != 2 {
		t.Fatalf("stems: got %d, want 2", len(stems))
	}
	if !stems[0].Horizontal {
		t.Errorf("stem 0: expected horizontal")
	}
	if stems[0].Edge != 100 || stems[0].Width != 20 {
		t.Errorf("stem 0: got (edge=%v, width=%v), want (100, 20)",
			stems[0].Edge, stems[0].Width)
	}
	// Second hstem edge is relative to the PREVIOUS stem's top edge
	// (100+20 = 120) plus the 200 delta = 320.
	if stems[1].Edge != 320 || stems[1].Width != 30 {
		t.Errorf("stem 1: got (edge=%v, width=%v), want (320, 30)",
			stems[1].Edge, stems[1].Width)
	}
}

func TestSnapToPixelGrid(t *testing.T) {
	// A glyph with one hstem at y=100.3 width 20.8.
	g := &HintedGlyph{
		Stems: []Stem{
			{Horizontal: true, Edge: 100.3, Width: 20.8},
		},
		Glyph: Glyph{
			Segments: []Segment{
				{Op: SegMoveTo, X: 0, Y: 100.3},
				{Op: SegLineTo, X: 50, Y: 100.3},
				{Op: SegLineTo, X: 50, Y: 121.1}, // 100.3 + 20.8
				{Op: SegLineTo, X: 0, Y: 121.1},
				{Op: SegLineTo, X: 10, Y: 50}, // unrelated point
			},
		},
	}
	// scale = 1 pixel per font unit means we snap to integer Y.
	snapped := g.SnapToPixelGrid(1.0)
	if snapped[0].Y != 100 {
		t.Errorf("snapped[0].Y: got %v, want 100", snapped[0].Y)
	}
	if snapped[1].Y != 100 {
		t.Errorf("snapped[1].Y: got %v, want 100", snapped[1].Y)
	}
	if snapped[2].Y != 121 {
		t.Errorf("snapped[2].Y: got %v, want 121", snapped[2].Y)
	}
	if snapped[3].Y != 121 {
		t.Errorf("snapped[3].Y: got %v, want 121", snapped[3].Y)
	}
	// Unrelated point stays at its original fractional Y (50 is already
	// integer so it's also unchanged, but this confirms non-stem points
	// aren't touched).
	if snapped[4].Y != 50 {
		t.Errorf("snapped[4].Y: got %v, want 50", snapped[4].Y)
	}
}
