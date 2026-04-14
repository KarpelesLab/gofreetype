// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package varfont

import "fmt"

// GVar is the parsed gvar table. Per-glyph variation data is kept in
// slice form; to apply deltas for a specific glyph + axis coordinate
// vector, call ApplyDeltas.
type GVar struct {
	AxisCount    int
	sharedTuples [][]float64 // each entry: axisCount normalized coords
	longOffsets  bool
	dataArrayOff int
	offsets      []uint32 // glyphCount+1 offsets into dataArrayOff
	data         []byte
}

// PointDelta is the (x, y) offset applied to one glyph point in font units.
type PointDelta struct {
	X, Y float64
}

// ParseGVar decodes a gvar table.
func ParseGVar(data []byte) (*GVar, error) {
	if len(data) < 20 {
		return nil, FormatError("gvar header too short")
	}
	major := u16(data, 0)
	if major != 1 {
		return nil, UnsupportedError(fmt.Sprintf("gvar major version %d", major))
	}
	axisCount := int(u16(data, 4))
	nShared := int(u16(data, 6))
	sharedOff := int(u32(data, 8))
	glyphCount := int(u16(data, 12))
	flags := u16(data, 14)
	dataArrayOff := int(u32(data, 16))
	longOffsets := flags&0x0001 != 0

	g := &GVar{
		AxisCount:    axisCount,
		longOffsets:  longOffsets,
		dataArrayOff: dataArrayOff,
		data:         data,
	}

	// Shared tuples.
	if sharedOff+2*axisCount*nShared > len(data) {
		return nil, FormatError("gvar shared tuples out of bounds")
	}
	g.sharedTuples = make([][]float64, nShared)
	for i := 0; i < nShared; i++ {
		coords := make([]float64, axisCount)
		for a := 0; a < axisCount; a++ {
			coords[a] = f2dot14ToFloat(int16(u16(data, sharedOff+2*(i*axisCount+a))))
		}
		g.sharedTuples[i] = coords
	}

	// Offsets.
	g.offsets = make([]uint32, glyphCount+1)
	off := 20
	if longOffsets {
		if off+4*(glyphCount+1) > len(data) {
			return nil, FormatError("gvar long offsets truncated")
		}
		for i := 0; i <= glyphCount; i++ {
			g.offsets[i] = u32(data, off+4*i)
		}
	} else {
		if off+2*(glyphCount+1) > len(data) {
			return nil, FormatError("gvar short offsets truncated")
		}
		for i := 0; i <= glyphCount; i++ {
			g.offsets[i] = uint32(u16(data, off+2*i)) * 2 // short offsets are multiplied by 2
		}
	}

	return g, nil
}

// Tuple variation header flags.
const (
	flagEmbeddedPeakTuple   = 0x8000
	flagIntermediateRegion  = 0x4000
	flagPrivatePointNumbers = 0x2000
	flagTupleIndexMask      = 0x0FFF
)

// tupleHeader captures the decoded fields of one tuple variation header.
type tupleHeader struct {
	variationDataSize int
	peakTuple         []float64 // len == AxisCount
	intermediateStart []float64 // nil unless intermediate region
	intermediateEnd   []float64
	privatePointNums  bool
}

// ApplyDeltas computes the point deltas for glyph gid at the given
// normalized axis coordinate vector (len == AxisCount, values in [-1, 1]).
//
// numPoints is the number of outline points in the glyph, NOT counting
// the synthetic phantom points that OpenType appends after loading. If
// the glyph has no gvar entry, returns nil deltas (no variation).
func (g *GVar) ApplyDeltas(gid int, normCoords []float64, numPoints int) []PointDelta {
	if g == nil || gid < 0 || gid+1 >= len(g.offsets) {
		return nil
	}
	if len(normCoords) != g.AxisCount {
		return nil
	}
	start := g.dataArrayOff + int(g.offsets[gid])
	end := g.dataArrayOff + int(g.offsets[gid+1])
	if start >= end || end > len(g.data) {
		return nil
	}
	gvd := g.data[start:end]
	if len(gvd) < 4 {
		return nil
	}
	// gvd[0:2] = tupleVariationCount (high 4 bits flags, low 12 bits count)
	// gvd[2:4] = offset to serialized data (from start of gvd)
	countField := u16(gvd, 0)
	serializedOff := int(u16(gvd, 2))
	nTuples := int(countField & 0x0FFF)
	sharedPointsFlag := countField&0x8000 != 0

	// Total points includes phantom points (4 extra points appended after
	// the outline points) per the OpenType spec for gvar. Callers pass just
	// the outline-point count; we add 4 here for phantom points. This
	// matches fontTools' allPoints behavior.
	totalPoints := numPoints + 4

	cursor := 4 // position in gvd, just past header + serialized offset

	// Optional shared point numbers at serializedOff.
	var sharedPoints []int
	pos := serializedOff
	if sharedPointsFlag {
		pts, n, err := readPackedPointNumbers(gvd, pos, totalPoints)
		if err != nil {
			return nil
		}
		sharedPoints = pts
		pos += n
	}

	// Accumulator.
	deltas := make([]PointDelta, totalPoints)

	// Per-tuple processing.
	for t := 0; t < nTuples; t++ {
		if cursor+4 > len(gvd) {
			return nil
		}
		dataSize := int(u16(gvd, cursor))
		tupleIndex := u16(gvd, cursor+2)
		cursor += 4

		h := tupleHeader{variationDataSize: dataSize}

		// Peak tuple: inline or shared.
		if tupleIndex&flagEmbeddedPeakTuple != 0 {
			if cursor+2*g.AxisCount > len(gvd) {
				return nil
			}
			h.peakTuple = make([]float64, g.AxisCount)
			for a := 0; a < g.AxisCount; a++ {
				h.peakTuple[a] = f2dot14ToFloat(int16(u16(gvd, cursor+2*a)))
			}
			cursor += 2 * g.AxisCount
		} else {
			idx := int(tupleIndex & flagTupleIndexMask)
			if idx >= len(g.sharedTuples) {
				return nil
			}
			h.peakTuple = g.sharedTuples[idx]
		}

		// Intermediate region coordinates.
		if tupleIndex&flagIntermediateRegion != 0 {
			if cursor+4*g.AxisCount > len(gvd) {
				return nil
			}
			h.intermediateStart = make([]float64, g.AxisCount)
			for a := 0; a < g.AxisCount; a++ {
				h.intermediateStart[a] = f2dot14ToFloat(int16(u16(gvd, cursor+2*a)))
			}
			cursor += 2 * g.AxisCount
			h.intermediateEnd = make([]float64, g.AxisCount)
			for a := 0; a < g.AxisCount; a++ {
				h.intermediateEnd[a] = f2dot14ToFloat(int16(u16(gvd, cursor+2*a)))
			}
			cursor += 2 * g.AxisCount
		}

		h.privatePointNums = tupleIndex&flagPrivatePointNumbers != 0

		// Compute tuple scalar weight for the given axis coord vector.
		scalar := tupleScalar(normCoords, h.peakTuple, h.intermediateStart, h.intermediateEnd)

		// Read serialized data.
		if pos+dataSize > len(gvd) {
			return nil
		}
		dataEnd := pos + dataSize
		tupleData := gvd[pos:dataEnd]
		pos = dataEnd

		if scalar == 0 {
			continue
		}

		// Point numbers — inline or shared.
		var points []int
		tp := 0
		if h.privatePointNums {
			pts, n, err := readPackedPointNumbers(tupleData, 0, totalPoints)
			if err != nil {
				continue
			}
			points = pts
			tp = n
		} else {
			points = sharedPoints
		}
		// Apply deltas.
		xDeltas, n, err := readPackedDeltas(tupleData, tp, countPointsInSet(points, totalPoints))
		if err != nil {
			continue
		}
		tp += n
		yDeltas, _, err := readPackedDeltas(tupleData, tp, countPointsInSet(points, totalPoints))
		if err != nil {
			continue
		}

		applyTuple(deltas, points, xDeltas, yDeltas, scalar, totalPoints)
	}

	return deltas
}

// countPointsInSet returns len(points) if non-nil (private set), else the
// total point count (all-points shorthand).
func countPointsInSet(points []int, total int) int {
	if points == nil {
		return total
	}
	return len(points)
}

// applyTuple adds scaled deltas to the accumulator.
func applyTuple(acc []PointDelta, points []int, xd, yd []int16, scalar float64, total int) {
	if points == nil {
		// "All points" shorthand — index-aligned.
		for i := 0; i < len(xd) && i < total; i++ {
			acc[i].X += float64(xd[i]) * scalar
			acc[i].Y += float64(yd[i]) * scalar
		}
		return
	}
	for i, p := range points {
		if p < 0 || p >= total || i >= len(xd) || i >= len(yd) {
			continue
		}
		acc[p].X += float64(xd[i]) * scalar
		acc[p].Y += float64(yd[i]) * scalar
	}
}

// tupleScalar computes the weight of a tuple given the current axis coords
// and the tuple's peak (and optional start/end) coords.
func tupleScalar(coords, peak, start, end []float64) float64 {
	scalar := 1.0
	for a, p := range peak {
		v := coords[a]
		if p == 0 {
			continue
		}
		if v == 0 {
			return 0
		}
		if start == nil {
			// Non-intermediate: scalar *= clamp(v/p, 0, 1) if same sign.
			if (p > 0 && v < 0) || (p < 0 && v > 0) {
				return 0
			}
			ratio := v / p
			if ratio > 1 {
				ratio = 1
			}
			if ratio < 0 {
				return 0
			}
			scalar *= ratio
		} else {
			s := start[a]
			e := end[a]
			if v <= s || v >= e {
				return 0
			}
			if v < p {
				scalar *= (v - s) / (p - s)
			} else if v > p {
				scalar *= (e - v) / (e - p)
			}
			// v == p: scalar *= 1
		}
	}
	return scalar
}

// readPackedPointNumbers decodes the packed point-number format.
//
// If the first byte is 0, the array is "all points". Otherwise:
//
//	controlByte:
//	  high bit    = point numbers are uint16 (not uint8)
//	  low 7 bits  = run length - 1
//	followed by run-length values (delta-encoded).
//
// Total count is in the first 1 or 2 bytes: high bit of byte 0 controls
// whether it's a 1-byte or 2-byte count.
func readPackedPointNumbers(data []byte, off, totalPoints int) ([]int, int, error) {
	if off >= len(data) {
		return nil, 0, FormatError("packed point numbers out of bounds")
	}
	start := off
	first := data[off]
	off++
	var count int
	if first&0x80 == 0 {
		count = int(first)
	} else {
		if off >= len(data) {
			return nil, 0, FormatError("packed point numbers (2-byte count) out of bounds")
		}
		count = int(first&0x7F)<<8 | int(data[off])
		off++
	}
	if count == 0 {
		// "All points" shorthand.
		return nil, off - start, nil
	}
	points := make([]int, 0, count)
	current := 0
	for len(points) < count {
		if off >= len(data) {
			return nil, 0, FormatError("packed point numbers run control truncated")
		}
		ctrl := data[off]
		off++
		runLen := int(ctrl&0x7F) + 1
		isU16 := ctrl&0x80 != 0
		for i := 0; i < runLen && len(points) < count; i++ {
			var v int
			if isU16 {
				if off+2 > len(data) {
					return nil, 0, FormatError("packed point numbers u16 run truncated")
				}
				v = int(u16(data, off))
				off += 2
			} else {
				if off >= len(data) {
					return nil, 0, FormatError("packed point numbers u8 run truncated")
				}
				v = int(data[off])
				off++
			}
			current += v
			points = append(points, current)
		}
	}
	return points, off - start, nil
}

// readPackedDeltas decodes the packed delta format:
//
//	controlByte:
//	  bit 7: deltas are zero (no data follows)
//	  bit 6: deltas are uint16 (otherwise uint8)
//	  low 6 bits: run length - 1
func readPackedDeltas(data []byte, off, count int) ([]int16, int, error) {
	start := off
	out := make([]int16, 0, count)
	for len(out) < count {
		if off >= len(data) {
			return nil, 0, FormatError("packed deltas control truncated")
		}
		ctrl := data[off]
		off++
		runLen := int(ctrl&0x3F) + 1
		if ctrl&0x80 != 0 {
			// Zero run.
			for i := 0; i < runLen && len(out) < count; i++ {
				out = append(out, 0)
			}
		} else if ctrl&0x40 != 0 {
			// 16-bit run.
			for i := 0; i < runLen && len(out) < count; i++ {
				if off+2 > len(data) {
					return nil, 0, FormatError("packed deltas u16 run truncated")
				}
				out = append(out, int16(u16(data, off)))
				off += 2
			}
		} else {
			// 8-bit run.
			for i := 0; i < runLen && len(out) < count; i++ {
				if off >= len(data) {
					return nil, 0, FormatError("packed deltas u8 run truncated")
				}
				out = append(out, int16(int8(data[off])))
				off++
			}
		}
	}
	return out, off - start, nil
}
