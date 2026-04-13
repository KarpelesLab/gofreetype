// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package varfont

import "fmt"

// ItemVariationStore is the shared "store" referenced by HVAR, VVAR, MVAR,
// GDEF and a few other variable-font tables. It provides a uniform way to
// fetch a delta given (outerIndex, innerIndex) and a normalized axis
// coordinate vector.
type ItemVariationStore struct {
	AxisCount int
	regions   []variationRegion // len = regionCount
	data      []itemVariationData
}

type variationRegion struct {
	// Per axis: start / peak / end in normalized coord space.
	start []float64
	peak  []float64
	end   []float64
}

type itemVariationData struct {
	regionIndexes []uint16
	// One int32 delta per (item, region). Stored flat: item * len(regionIndexes) + region.
	deltas []int32
}

// ParseItemVariationStore decodes an ItemVariationStore at data[off:].
func ParseItemVariationStore(data []byte, off int) (*ItemVariationStore, error) {
	if off+8 > len(data) {
		return nil, FormatError("IVS header truncated")
	}
	format := u16(data, off)
	if format != 1 {
		return nil, UnsupportedError(fmt.Sprintf("IVS format %d", format))
	}
	regionListOff := off + int(u32(data, off+2))
	ivdCount := int(u16(data, off+6))
	if off+8+4*ivdCount > len(data) {
		return nil, FormatError("IVS data offsets truncated")
	}

	store := &ItemVariationStore{}

	// VariationRegionList.
	if regionListOff+4 > len(data) {
		return nil, FormatError("VariationRegionList header truncated")
	}
	axisCount := int(u16(data, regionListOff))
	regionCount := int(u16(data, regionListOff+2))
	store.AxisCount = axisCount

	regionSize := 6 * axisCount
	if regionListOff+4+regionCount*regionSize > len(data) {
		return nil, FormatError("VariationRegion records truncated")
	}
	store.regions = make([]variationRegion, regionCount)
	for i := 0; i < regionCount; i++ {
		recOff := regionListOff + 4 + i*regionSize
		r := variationRegion{
			start: make([]float64, axisCount),
			peak:  make([]float64, axisCount),
			end:   make([]float64, axisCount),
		}
		for a := 0; a < axisCount; a++ {
			r.start[a] = f2dot14ToFloat(int16(u16(data, recOff+6*a)))
			r.peak[a] = f2dot14ToFloat(int16(u16(data, recOff+6*a+2)))
			r.end[a] = f2dot14ToFloat(int16(u16(data, recOff+6*a+4)))
		}
		store.regions[i] = r
	}

	// ItemVariationData subtables.
	store.data = make([]itemVariationData, ivdCount)
	for i := 0; i < ivdCount; i++ {
		ivdOff := off + int(u32(data, off+8+4*i))
		ivd, err := parseItemVariationData(data, ivdOff)
		if err != nil {
			return nil, fmt.Errorf("ItemVariationData[%d]: %w", i, err)
		}
		store.data[i] = *ivd
	}
	return store, nil
}

func parseItemVariationData(data []byte, off int) (*itemVariationData, error) {
	if off+6 > len(data) {
		return nil, FormatError("ItemVariationData header truncated")
	}
	itemCount := int(u16(data, off))
	shortDeltaCount := int(u16(data, off+2))
	regionIndexCount := int(u16(data, off+4))
	if off+6+2*regionIndexCount > len(data) {
		return nil, FormatError("regionIndexes truncated")
	}
	ivd := &itemVariationData{
		regionIndexes: make([]uint16, regionIndexCount),
		deltas:        make([]int32, itemCount*regionIndexCount),
	}
	for i := 0; i < regionIndexCount; i++ {
		ivd.regionIndexes[i] = u16(data, off+6+2*i)
	}
	p := off + 6 + 2*regionIndexCount
	// DeltaSets: itemCount rows. Each row: shortDeltaCount int16s then
	// (regionIndexCount - shortDeltaCount) int8s.
	rowBytes := 2*shortDeltaCount + (regionIndexCount - shortDeltaCount)
	if p+itemCount*rowBytes > len(data) {
		return nil, FormatError("DeltaSet rows truncated")
	}
	for i := 0; i < itemCount; i++ {
		rp := p + i*rowBytes
		for j := 0; j < shortDeltaCount; j++ {
			ivd.deltas[i*regionIndexCount+j] = int32(int16(u16(data, rp+2*j)))
		}
		for j := shortDeltaCount; j < regionIndexCount; j++ {
			ivd.deltas[i*regionIndexCount+j] = int32(int8(data[rp+2*shortDeltaCount+(j-shortDeltaCount)]))
		}
	}
	return ivd, nil
}

// Delta returns the cumulative delta for (outerIndex, innerIndex) at the
// given normalized axis coordinate vector. Values outside the store's
// index range return 0.
func (s *ItemVariationStore) Delta(outer, inner uint16, coords []float64) float64 {
	if s == nil || int(outer) >= len(s.data) {
		return 0
	}
	ivd := &s.data[outer]
	regionCount := len(ivd.regionIndexes)
	if regionCount == 0 || int(inner)*regionCount+regionCount > len(ivd.deltas) {
		return 0
	}
	total := 0.0
	for j, regIdx := range ivd.regionIndexes {
		if int(regIdx) >= len(s.regions) {
			continue
		}
		scalar := regionScalar(&s.regions[regIdx], coords)
		if scalar == 0 {
			continue
		}
		total += float64(ivd.deltas[int(inner)*regionCount+j]) * scalar
	}
	return total
}

// regionScalar returns the weight of a VariationRegion at the given
// normalized axis coords. Uses the same piecewise-linear semantics as
// gvar intermediate tuples.
func regionScalar(r *variationRegion, coords []float64) float64 {
	scalar := 1.0
	for a := range r.peak {
		if a >= len(coords) {
			return 0
		}
		start, peak, end := r.start[a], r.peak[a], r.end[a]
		v := coords[a]
		if start > peak || peak > end {
			continue
		}
		if start < 0 && end > 0 && peak != 0 {
			continue
		}
		if peak == 0 {
			continue
		}
		if v < start || v > end {
			return 0
		}
		if v == peak {
			continue
		}
		if v < peak {
			scalar *= (v - start) / (peak - start)
		} else {
			scalar *= (end - v) / (end - peak)
		}
	}
	return scalar
}
