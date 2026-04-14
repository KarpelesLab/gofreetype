// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package raster

import (
	"testing"

	"golang.org/x/image/math/fixed"
)

func TestMaxAbs(t *testing.T) {
	if got := maxAbs(fixed.I(-5), fixed.I(3)); got != fixed.I(5) {
		t.Errorf("maxAbs(-5, 3): got %v, want 5", got)
	}
	if got := maxAbs(fixed.I(2), fixed.I(-7)); got != fixed.I(7) {
		t.Errorf("maxAbs(2, -7): got %v, want 7", got)
	}
}

func TestPNeg(t *testing.T) {
	p := fixed.Point26_6{X: fixed.I(3), Y: fixed.I(-4)}
	got := pNeg(p)
	if got.X != fixed.I(-3) || got.Y != fixed.I(4) {
		t.Errorf("pNeg: got %+v, want (-3, 4)", got)
	}
}

func TestPDot(t *testing.T) {
	a := fixed.Point26_6{X: fixed.I(2), Y: fixed.I(3)}
	b := fixed.Point26_6{X: fixed.I(4), Y: fixed.I(5)}
	// 2*4 + 3*5 = 23 (in fixed.Int26_6^2 = fixed.Int52_12).
	if got := pDot(a, b); got != fixed.Int52_12(int64(2<<6)*int64(4<<6)+int64(3<<6)*int64(5<<6)) {
		t.Errorf("pDot: got %v, want ~23", got)
	}
}

func TestPLenAndPNorm(t *testing.T) {
	v := fixed.Point26_6{X: fixed.I(3), Y: fixed.I(4)} // length 5
	length := pLen(v)
	if length < fixed.I(4) || length > fixed.I(6) {
		t.Errorf("pLen(3, 4): got %v, want ~5", length)
	}
	// Normalize to length 10 — should scale up by 2.
	normalized := pNorm(v, fixed.I(10))
	// Should have length approximately 10.
	gotLen := pLen(normalized)
	if gotLen < fixed.I(9) || gotLen > fixed.I(11) {
		t.Errorf("pNorm: length %v, want ~10", gotLen)
	}
	// pNorm on a zero vector returns zero.
	zero := pNorm(fixed.Point26_6{}, fixed.I(10))
	if zero.X != 0 || zero.Y != 0 {
		t.Errorf("pNorm(zero): got %+v, want (0, 0)", zero)
	}
}

func TestPRotations(t *testing.T) {
	right := fixed.Point26_6{X: fixed.I(1), Y: 0}

	cw90 := pRot90CW(right)
	if cw90.X != 0 || cw90.Y != fixed.I(1) {
		t.Errorf("pRot90CW({1,0}): got %+v, want (0, 1)", cw90)
	}
	ccw90 := pRot90CCW(right)
	if ccw90.X != 0 || ccw90.Y != fixed.I(-1) {
		t.Errorf("pRot90CCW({1,0}): got %+v, want (0, -1)", ccw90)
	}
	// 45-degree rotations: components should equal 181/256 of the unit.
	// We just check that neither component is exactly 0 (they're both
	// non-zero after rotation from an axis-aligned vector).
	cw45 := pRot45CW(right)
	if cw45.X == 0 || cw45.Y == 0 {
		t.Errorf("pRot45CW({1,0}): got %+v, want two non-zero components", cw45)
	}
	ccw45 := pRot45CCW(right)
	if ccw45.X == 0 || ccw45.Y == 0 {
		t.Errorf("pRot45CCW({1,0}): got %+v, want two non-zero components", ccw45)
	}
}
