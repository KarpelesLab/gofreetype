// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package cff

import "testing"

// encodeCSOperand encodes a Type 2 charstring numeric operand using the
// shortest valid representation for the given integer value.
func encodeCSOperand(v int) []byte {
	switch {
	case v >= -107 && v <= 107:
		return []byte{byte(v + 139)}
	case v >= 108 && v <= 1131:
		w := v - 108
		return []byte{byte(w>>8) + 247, byte(w)}
	case v >= -1131 && v <= -108:
		w := -v - 108
		return []byte{byte(w>>8) + 251, byte(w)}
	case v >= -32768 && v <= 32767:
		return []byte{28, byte(v >> 8), byte(v)}
	}
	panic("encodeCSOperand: out of range")
}

// runCharstring executes a single charstring in isolation and returns the
// resulting glyph. Subroutines and widths are empty/defaulted.
func runCharstring(t *testing.T, cs []byte) *Glyph {
	t.Helper()
	psInterp := &interp{}
	if err := psInterp.run(cs); err != nil {
		t.Fatalf("run: %v", err)
	}
	return &Glyph{Segments: psInterp.segments, Width: psInterp.width, HasWidth: psInterp.hasWidth}
}

func TestCharstringRectangle(t *testing.T) {
	// rmoveto(100,200), rlineto(50,0), rlineto(0,30), rlineto(-50,0), endchar.
	var cs []byte
	cs = append(cs, encodeCSOperand(100)...)
	cs = append(cs, encodeCSOperand(200)...)
	cs = append(cs, 21) // rmoveto
	cs = append(cs, encodeCSOperand(50)...)
	cs = append(cs, encodeCSOperand(0)...)
	cs = append(cs, encodeCSOperand(0)...)
	cs = append(cs, encodeCSOperand(30)...)
	cs = append(cs, encodeCSOperand(-50)...)
	cs = append(cs, encodeCSOperand(0)...)
	cs = append(cs, 5)  // rlineto
	cs = append(cs, 14) // endchar

	g := runCharstring(t, cs)
	wantOps := []SegmentOp{SegMoveTo, SegLineTo, SegLineTo, SegLineTo}
	if len(g.Segments) != len(wantOps) {
		t.Fatalf("segments: got %d, want %d", len(g.Segments), len(wantOps))
	}
	for i, op := range wantOps {
		if g.Segments[i].Op != op {
			t.Errorf("segment %d op: got %d, want %d", i, g.Segments[i].Op, op)
		}
	}
	// Final pen is at (100, 230) after the three lineto operations.
	last := g.Segments[3]
	if last.X != 100 || last.Y != 230 {
		t.Errorf("final lineto: got (%v, %v), want (100, 230)", last.X, last.Y)
	}
}

func TestCharstringHVLineAlternating(t *testing.T) {
	// rmoveto(0,0), hlineto(10, 20, 30) — x += 10, y += 20, x += 30
	var cs []byte
	cs = append(cs, encodeCSOperand(0)...)
	cs = append(cs, encodeCSOperand(0)...)
	cs = append(cs, 21) // rmoveto
	cs = append(cs, encodeCSOperand(10)...)
	cs = append(cs, encodeCSOperand(20)...)
	cs = append(cs, encodeCSOperand(30)...)
	cs = append(cs, 6)  // hlineto (starts horizontal)
	cs = append(cs, 14) // endchar

	g := runCharstring(t, cs)
	if len(g.Segments) != 4 {
		t.Fatalf("segments: got %d, want 4", len(g.Segments))
	}
	if got := g.Segments[1]; got.X != 10 || got.Y != 0 {
		t.Errorf("segment 1: got (%v,%v), want (10,0)", got.X, got.Y)
	}
	if got := g.Segments[2]; got.X != 10 || got.Y != 20 {
		t.Errorf("segment 2: got (%v,%v), want (10,20)", got.X, got.Y)
	}
	if got := g.Segments[3]; got.X != 40 || got.Y != 20 {
		t.Errorf("segment 3: got (%v,%v), want (40,20)", got.X, got.Y)
	}
}

func TestCharstringRRCurveTo(t *testing.T) {
	// rmoveto(0,0), rrcurveto(10,0, 10,10, 0,10) — a quarter-circle-ish curve.
	var cs []byte
	cs = append(cs, encodeCSOperand(0)...)
	cs = append(cs, encodeCSOperand(0)...)
	cs = append(cs, 21)
	cs = append(cs, encodeCSOperand(10)...)
	cs = append(cs, encodeCSOperand(0)...)
	cs = append(cs, encodeCSOperand(10)...)
	cs = append(cs, encodeCSOperand(10)...)
	cs = append(cs, encodeCSOperand(0)...)
	cs = append(cs, encodeCSOperand(10)...)
	cs = append(cs, 8)
	cs = append(cs, 14)

	g := runCharstring(t, cs)
	if len(g.Segments) != 2 {
		t.Fatalf("segments: got %d, want 2", len(g.Segments))
	}
	if g.Segments[1].Op != SegCubicTo {
		t.Errorf("segment 1: got op %d, want SegCubicTo", g.Segments[1].Op)
	}
	seg := g.Segments[1]
	// Controls: (10,0), (20,10). End: (20,20).
	if seg.CX1 != 10 || seg.CY1 != 0 {
		t.Errorf("c1: got (%v,%v), want (10,0)", seg.CX1, seg.CY1)
	}
	if seg.CX2 != 20 || seg.CY2 != 10 {
		t.Errorf("c2: got (%v,%v), want (20,10)", seg.CX2, seg.CY2)
	}
	if seg.X != 20 || seg.Y != 20 {
		t.Errorf("end: got (%v,%v), want (20,20)", seg.X, seg.Y)
	}
}

func TestCharstringSubrBias(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want int
	}{
		{0, 107},
		{1239, 107},
		{1240, 1131},
		{33899, 1131},
		{33900, 32768},
	} {
		if got := subrBias(tc.n); got != tc.want {
			t.Errorf("subrBias(%d): got %d, want %d", tc.n, got, tc.want)
		}
	}
}

func TestCharstringCallSubr(t *testing.T) {
	// Local subr: rlineto(5, 5), return.
	subr := []byte{}
	subr = append(subr, encodeCSOperand(5)...)
	subr = append(subr, encodeCSOperand(5)...)
	subr = append(subr, 5)  // rlineto
	subr = append(subr, 11) // return

	// Charstring: rmoveto(0,0), push idx -107 (so +107 bias = subr 0), callsubr, endchar.
	var cs []byte
	cs = append(cs, encodeCSOperand(0)...)
	cs = append(cs, encodeCSOperand(0)...)
	cs = append(cs, 21)
	cs = append(cs, encodeCSOperand(-107)...)
	cs = append(cs, 10) // callsubr
	cs = append(cs, 14) // endchar

	p := &interp{locals: [][]byte{subr}}
	if err := p.run(cs); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(p.segments) != 2 {
		t.Fatalf("segments: got %d, want 2", len(p.segments))
	}
	if got := p.segments[1]; got.Op != SegLineTo || got.X != 5 || got.Y != 5 {
		t.Errorf("segment[1]: got %+v, want line to (5,5)", got)
	}
}

func TestCharstringCallSubrOutOfRange(t *testing.T) {
	// callsubr with index that produces an out-of-range value after bias.
	var cs []byte
	cs = append(cs, encodeCSOperand(0)...)
	cs = append(cs, encodeCSOperand(0)...)
	cs = append(cs, 21)
	cs = append(cs, encodeCSOperand(500)...) // +107 bias = 607, but only 1 subr
	cs = append(cs, 10)                      // callsubr
	cs = append(cs, 14)

	subr := []byte{encodeCSOperand(1)[0], 11}
	p := &interp{locals: [][]byte{subr}}
	err := p.run(cs)
	if err == nil {
		t.Fatal("expected out-of-range subr error, got nil")
	}
}

func TestCharstringWidth(t *testing.T) {
	// rmoveto with stack = [width, dx, dy] → width gets popped.
	var cs []byte
	cs = append(cs, encodeCSOperand(50)...) // width delta
	cs = append(cs, encodeCSOperand(10)...)
	cs = append(cs, encodeCSOperand(20)...)
	cs = append(cs, 21) // rmoveto
	cs = append(cs, 14) // endchar

	p := &interp{nominalWidth: 500, defaultWidth: 450}
	if err := p.run(cs); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !p.hasWidth {
		t.Error("hasWidth should be true")
	}
	if p.width != 550 {
		t.Errorf("width: got %v, want 550 (nominalWidth 500 + delta 50)", p.width)
	}
	if len(p.segments) != 1 {
		t.Fatalf("segments: got %d, want 1", len(p.segments))
	}
	seg := p.segments[0]
	if seg.X != 10 || seg.Y != 20 {
		t.Errorf("moveto target: got (%v,%v), want (10,20)", seg.X, seg.Y)
	}
}

func TestCharstringNoWidth(t *testing.T) {
	// rmoveto with exactly 2 operands → no width, default is used.
	var cs []byte
	cs = append(cs, encodeCSOperand(10)...)
	cs = append(cs, encodeCSOperand(20)...)
	cs = append(cs, 21)
	cs = append(cs, 14)

	p := &interp{nominalWidth: 500, defaultWidth: 450}
	if err := p.run(cs); err != nil {
		t.Fatalf("run: %v", err)
	}
	if p.hasWidth {
		t.Error("hasWidth should be false")
	}
	if p.width != 450 {
		t.Errorf("width: got %v, want 450 (defaultWidth)", p.width)
	}
}

func TestCharstringOpBudget(t *testing.T) {
	// Build a charstring that pushes operand 0 a million times.
	cs := make([]byte, 0, 150000)
	// Loop by building a local subr that pushes 1 operand and returns,
	// then call it repeatedly from the main charstring until budget hits.
	subr := []byte{encodeCSOperand(0)[0], 11}
	for i := 0; i < maxCharstringOps+100; i++ {
		cs = append(cs, encodeCSOperand(-107)...) // subr 0
		cs = append(cs, 10)
	}
	cs = append(cs, 14) // endchar

	p := &interp{locals: [][]byte{subr}}
	err := p.run(cs)
	if err == nil {
		t.Fatal("expected operator-budget error, got nil")
	}
}
