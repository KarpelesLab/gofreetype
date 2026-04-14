// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package type1

import (
	"testing"

	"github.com/KarpelesLab/gofreetype/cff"
)

// encNum encodes a Type 1 numeric operand using the shortest form.
func encNum(v int) []byte {
	switch {
	case v >= -107 && v <= 107:
		return []byte{byte(v + 139)}
	case v >= 108 && v <= 1131:
		w := v - 108
		return []byte{byte(w>>8) + 247, byte(w)}
	case v >= -1131 && v <= -108:
		w := -v - 108
		return []byte{byte(w>>8) + 251, byte(w)}
	}
	return []byte{255,
		byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v),
	}
}

func TestDecodeRectangleType1(t *testing.T) {
	// A Type 1 glyph drawing a 100x50 rectangle.
	var cs []byte
	// hsbw 0 500
	cs = append(cs, encNum(0)...)
	cs = append(cs, encNum(500)...)
	cs = append(cs, 13)
	// rmoveto 10 10
	cs = append(cs, encNum(10)...)
	cs = append(cs, encNum(10)...)
	cs = append(cs, 21)
	// rlineto 100 0
	cs = append(cs, encNum(100)...)
	cs = append(cs, encNum(0)...)
	cs = append(cs, 5)
	// rlineto 0 50
	cs = append(cs, encNum(0)...)
	cs = append(cs, encNum(50)...)
	cs = append(cs, 5)
	// rlineto -100 0
	cs = append(cs, encNum(-100)...)
	cs = append(cs, encNum(0)...)
	cs = append(cs, 5)
	// closepath
	cs = append(cs, 9)
	// endchar
	cs = append(cs, 14)

	g, err := Decode(cs, nil)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if g.Width != 500 {
		t.Errorf("Width: got %v, want 500", g.Width)
	}
	if g.SideBearing != 0 {
		t.Errorf("SideBearing: got %v, want 0", g.SideBearing)
	}
	wantOps := []cff.SegmentOp{cff.SegMoveTo, cff.SegLineTo, cff.SegLineTo, cff.SegLineTo}
	if len(g.Segments) != len(wantOps) {
		t.Fatalf("segments: got %d, want %d", len(g.Segments), len(wantOps))
	}
	for i, op := range wantOps {
		if g.Segments[i].Op != op {
			t.Errorf("segment[%d] op: got %d, want %d", i, g.Segments[i].Op, op)
		}
	}
	// Moveto went to (sbx+10, 0+10) = (10, 10); lineto (110, 10); (110, 60); (10, 60).
	if g.Segments[0].X != 10 || g.Segments[0].Y != 10 {
		t.Errorf("moveto: got (%v, %v), want (10, 10)", g.Segments[0].X, g.Segments[0].Y)
	}
	if g.Segments[2].X != 110 || g.Segments[2].Y != 60 {
		t.Errorf("lineto 2: got (%v, %v), want (110, 60)", g.Segments[2].X, g.Segments[2].Y)
	}
}

func TestDecodeCallsSubr(t *testing.T) {
	// Subr 0: rlineto 10 20, return.
	subr := []byte{}
	subr = append(subr, encNum(10)...)
	subr = append(subr, encNum(20)...)
	subr = append(subr, 5)
	subr = append(subr, 11)

	// Main: hsbw 0 500, rmoveto 0 0, push 0, callsubr, endchar.
	var cs []byte
	cs = append(cs, encNum(0)...)
	cs = append(cs, encNum(500)...)
	cs = append(cs, 13)
	cs = append(cs, encNum(0)...)
	cs = append(cs, encNum(0)...)
	cs = append(cs, 21)
	cs = append(cs, encNum(0)...) // subr index
	cs = append(cs, 10)
	cs = append(cs, 14)

	g, err := Decode(cs, [][]byte{subr})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Segments) < 2 {
		t.Fatalf("segments: got %d, want >= 2", len(g.Segments))
	}
	if g.Segments[1].X != 10 || g.Segments[1].Y != 20 {
		t.Errorf("subr-produced lineto: got (%v, %v), want (10, 20)",
			g.Segments[1].X, g.Segments[1].Y)
	}
}

func TestDecodeSeacUnsupported(t *testing.T) {
	// 5 operands + escape 6 (seac).
	var cs []byte
	for i := 0; i < 5; i++ {
		cs = append(cs, encNum(0)...)
	}
	cs = append(cs, 12, 6)
	cs = append(cs, 14)
	_, err := Decode(cs, nil)
	if err == nil {
		t.Fatal("expected UnsupportedError for seac")
	}
	if _, ok := err.(UnsupportedError); !ok {
		t.Errorf("expected UnsupportedError, got %T: %v", err, err)
	}
}

func TestDecodeOpBudget(t *testing.T) {
	// Push a lot of zeroes via subr recursion until budget runs out.
	// Subr 0 = push 0, callsubr to self.
	subr := []byte{}
	subr = append(subr, encNum(0)...)
	subr = append(subr, encNum(0)...)
	subr = append(subr, 10)
	// Main: push 0, callsubr.
	cs := []byte{}
	cs = append(cs, encNum(0)...)
	cs = append(cs, 10)
	_, err := Decode(cs, [][]byte{subr})
	if err == nil {
		t.Fatal("expected budget error")
	}
}
