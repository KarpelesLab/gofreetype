// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package varfont

import "fmt"

// AxisSegmentMap is one axis's remapping of normalized user coordinates.
// Given a normalized input x in [-1, 1], the output is obtained by linear
// interpolation between the listed (fromCoord, toCoord) points; both are
// in normalized space.
type AxisSegmentMap struct {
	FromCoord []float64
	ToCoord   []float64
}

// AVar is the parsed avar table: one AxisSegmentMap per axis.
type AVar struct {
	Axes []AxisSegmentMap
}

// ParseAVar decodes an avar table.
//
// Header (8 bytes):
//
//	uint16 majorVersion (1)
//	uint16 minorVersion (0)
//	uint16 reserved (0)
//	uint16 axisCount
//
// For each axis, a SegmentMaps record:
//
//	uint16 positionMapCount
//	AxisValueMap axisValueMaps[positionMapCount]
//
// AxisValueMap (4 bytes):
//
//	F2DOT14 fromCoordinate
//	F2DOT14 toCoordinate
//
// axisCount MUST match fvar's axisCount but we don't enforce that here —
// the caller is responsible for consistency.
func ParseAVar(data []byte) (*AVar, error) {
	if len(data) < 8 {
		return nil, FormatError("avar header too short")
	}
	major := u16(data, 0)
	if major != 1 {
		return nil, UnsupportedError(fmt.Sprintf("avar major version %d", major))
	}
	axisCount := int(u16(data, 6))
	av := &AVar{Axes: make([]AxisSegmentMap, axisCount)}
	off := 8
	for i := 0; i < axisCount; i++ {
		if off+2 > len(data) {
			return nil, FormatError("avar segment map header truncated")
		}
		n := int(u16(data, off))
		off += 2
		if off+4*n > len(data) {
			return nil, FormatError("avar segment map entries truncated")
		}
		from := make([]float64, n)
		to := make([]float64, n)
		for j := 0; j < n; j++ {
			from[j] = f2dot14ToFloat(int16(u16(data, off+4*j)))
			to[j] = f2dot14ToFloat(int16(u16(data, off+4*j+2)))
		}
		av.Axes[i] = AxisSegmentMap{FromCoord: from, ToCoord: to}
		off += 4 * n
	}
	return av, nil
}

// Remap applies axis index i's segment map to the normalized coordinate x.
// Returns x unchanged if the map is empty or the axis is out of range.
func (av *AVar) Remap(i int, x float64) float64 {
	if av == nil || i < 0 || i >= len(av.Axes) {
		return x
	}
	m := &av.Axes[i]
	n := len(m.FromCoord)
	if n == 0 {
		return x
	}
	// Outside the table range, extrapolate linearly using the last segment.
	if x <= m.FromCoord[0] {
		return m.ToCoord[0]
	}
	if x >= m.FromCoord[n-1] {
		return m.ToCoord[n-1]
	}
	// Find the bracketing segment and lerp.
	for j := 0; j+1 < n; j++ {
		if x >= m.FromCoord[j] && x <= m.FromCoord[j+1] {
			span := m.FromCoord[j+1] - m.FromCoord[j]
			if span == 0 {
				return m.ToCoord[j]
			}
			t := (x - m.FromCoord[j]) / span
			return m.ToCoord[j] + t*(m.ToCoord[j+1]-m.ToCoord[j])
		}
	}
	return x
}

// f2dot14ToFloat decodes the F2DOT14 fixed-point format.
func f2dot14ToFloat(v int16) float64 {
	return float64(v) / 16384.0
}
