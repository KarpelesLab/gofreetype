// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package varfont

import (
	"math"
	"testing"
)

func TestTupleScalarNonIntermediate(t *testing.T) {
	// Peak at +1 on one axis.
	peak := []float64{1, 0}
	for _, tc := range []struct {
		coord      []float64
		wantScalar float64
	}{
		{[]float64{0, 0}, 0},     // Not on the peak axis: scalar=0 (0 axis value with peak!=0 -> 0)
		{[]float64{1, 0}, 1},     // Exactly at peak.
		{[]float64{0.5, 0}, 0.5}, // Halfway.
		{[]float64{-0.5, 0}, 0},  // Opposite sign.
		{[]float64{1, 0.8}, 1},   // Other axis with peak=0: no effect.
	} {
		got := tupleScalar(tc.coord, peak, nil, nil)
		if math.Abs(got-tc.wantScalar) > 1e-9 {
			t.Errorf("tupleScalar(%v): got %v, want %v", tc.coord, got, tc.wantScalar)
		}
	}
}

func TestTupleScalarIntermediate(t *testing.T) {
	// Intermediate region: start=0.2, peak=0.6, end=1.0 on one axis.
	peak := []float64{0.6}
	start := []float64{0.2}
	end := []float64{1.0}
	for _, tc := range []struct {
		coord float64
		want  float64
	}{
		{0.2, 0},   // At start.
		{0.6, 1},   // At peak.
		{1.0, 0},   // At end.
		{0.4, 0.5}, // (0.4-0.2)/(0.6-0.2) = 0.5
		{0.8, 0.5}, // (1.0-0.8)/(1.0-0.6) = 0.5
		{0.1, 0},   // Below start.
		{1.1, 0},   // Above end.
	} {
		got := tupleScalar([]float64{tc.coord}, peak, start, end)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("intermediate(%v): got %v, want %v", tc.coord, got, tc.want)
		}
	}
}

func TestReadPackedDeltasAllU8(t *testing.T) {
	// control = 0x02 (3 deltas, 8-bit, non-zero) + 3 bytes.
	data := []byte{0x02, 0xff, 0x05, 0x00}
	out, n, err := readPackedDeltas(data, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("bytes consumed: got %d, want 4", n)
	}
	want := []int16{-1, 5, 0}
	for i, v := range want {
		if out[i] != v {
			t.Errorf("out[%d]: got %d, want %d", i, out[i], v)
		}
	}
}

func TestReadPackedDeltasZeroRun(t *testing.T) {
	// control = 0x82 (3 zero deltas).
	data := []byte{0x82}
	out, n, err := readPackedDeltas(data, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("bytes consumed: got %d, want 1", n)
	}
	for i, v := range out {
		if v != 0 {
			t.Errorf("out[%d]: got %d, want 0", i, v)
		}
	}
}

func TestReadPackedDeltasU16(t *testing.T) {
	// control = 0x41 (2 deltas, 16-bit) + 4 bytes.
	data := []byte{0x41, 0x01, 0x00, 0xff, 0xff}
	out, n, err := readPackedDeltas(data, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("bytes consumed: got %d, want 5", n)
	}
	if out[0] != 256 || out[1] != -1 {
		t.Errorf("got %v, want [256, -1]", out)
	}
}

func TestReadPackedPointNumbersAll(t *testing.T) {
	// First byte 0 = "all points" shorthand.
	data := []byte{0x00}
	pts, n, err := readPackedPointNumbers(data, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if pts != nil {
		t.Errorf("all-points: got %v, want nil", pts)
	}
	if n != 1 {
		t.Errorf("bytes consumed: got %d, want 1", n)
	}
}

func TestReadPackedPointNumbersDelta(t *testing.T) {
	// count=3, then one u8 run of 3 deltas: 0, 2, 3.
	// Control byte 0x02 (low 7 = 2 = runLen-1, so runLen=3; high bit clear = u8).
	data := []byte{0x03, 0x02, 0x00, 0x02, 0x03}
	pts, n, err := readPackedPointNumbers(data, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("bytes consumed: got %d, want 5", n)
	}
	// Cumulative: 0, 0+2=2, 2+3=5.
	want := []int{0, 2, 5}
	for i, v := range want {
		if pts[i] != v {
			t.Errorf("pts[%d]: got %d, want %d", i, pts[i], v)
		}
	}
}
