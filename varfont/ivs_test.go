// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package varfont

import (
	"encoding/binary"
	"math"
	"testing"
)

// buildItemVariationStore assembles a minimal IVS with the given regions
// (each in normalized coords) and per-item deltas. shortDeltaCount
// controls how many regions use 16-bit deltas versus 8-bit.
//
// regions: [][3]coords per axis (start, peak, end), ordered [region][axis].
// items:   [][int16 delta per region].
func buildItemVariationStore(axisCount int, regions [][][3]float64, items [][]int16) []byte {
	// Layout: IVS header (8) + IVD offset slot(s) (4 each) + region list + IVD bodies.
	// For simplicity, one IVD subtable containing all items.
	regionCount := len(regions)
	regionSize := 6 * axisCount
	regionListLen := 4 + regionCount*regionSize

	// IVD: header (6) + regionIndexes (2*regionCount) + itemCount*rowBytes.
	shortDeltaCount := regionCount // all deltas stored as int16 for simplicity
	rowBytes := 2*shortDeltaCount + (regionCount - shortDeltaCount)
	ivdLen := 6 + 2*regionCount + len(items)*rowBytes

	headerLen := 8 + 4 // 1 IVD offset
	regionListOff := headerLen
	ivdOff := regionListOff + regionListLen

	total := ivdOff + ivdLen
	out := make([]byte, total)
	binary.BigEndian.PutUint16(out[0:], 1) // format
	binary.BigEndian.PutUint32(out[2:], uint32(regionListOff))
	binary.BigEndian.PutUint16(out[6:], 1) // itemVariationDataCount
	binary.BigEndian.PutUint32(out[8:], uint32(ivdOff))

	// Region list.
	binary.BigEndian.PutUint16(out[regionListOff:], uint16(axisCount))
	binary.BigEndian.PutUint16(out[regionListOff+2:], uint16(regionCount))
	for i, r := range regions {
		recOff := regionListOff + 4 + i*regionSize
		for a := 0; a < axisCount; a++ {
			binary.BigEndian.PutUint16(out[recOff+6*a:], uint16(int16(r[a][0]*16384)))
			binary.BigEndian.PutUint16(out[recOff+6*a+2:], uint16(int16(r[a][1]*16384)))
			binary.BigEndian.PutUint16(out[recOff+6*a+4:], uint16(int16(r[a][2]*16384)))
		}
	}

	// IVD.
	binary.BigEndian.PutUint16(out[ivdOff:], uint16(len(items)))
	binary.BigEndian.PutUint16(out[ivdOff+2:], uint16(shortDeltaCount))
	binary.BigEndian.PutUint16(out[ivdOff+4:], uint16(regionCount))
	for j := 0; j < regionCount; j++ {
		binary.BigEndian.PutUint16(out[ivdOff+6+2*j:], uint16(j))
	}
	body := ivdOff + 6 + 2*regionCount
	for i, item := range items {
		for j, d := range item {
			binary.BigEndian.PutUint16(out[body+i*rowBytes+2*j:], uint16(d))
		}
	}
	return out
}

func TestItemVariationStoreDelta(t *testing.T) {
	// One axis, one region peaking at +1.
	regions := [][][3]float64{
		{{0, 1, 1}}, // region 0: start=0, peak=1, end=1
	}
	// Two items: delta 100 at peak, delta -50 at peak.
	items := [][]int16{
		{100},
		{-50},
	}
	data := buildItemVariationStore(1, regions, items)
	store, err := ParseItemVariationStore(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	// At axis = 0: scalar = 0 (below start of region 0 (which is 0) — hmm
	// actually v==0==start so (v-start)/(peak-start) = 0; weight = 0).
	if got := store.Delta(0, 0, []float64{0}); math.Abs(got) > 1e-9 {
		t.Errorf("Delta at 0: got %v, want 0", got)
	}
	// At axis = 1: scalar = 1, delta = 100.
	if got := store.Delta(0, 0, []float64{1}); math.Abs(got-100) > 1e-9 {
		t.Errorf("Delta at 1: got %v, want 100", got)
	}
	// At axis = 0.5: scalar = 0.5, delta = 50.
	if got := store.Delta(0, 0, []float64{0.5}); math.Abs(got-50) > 1e-9 {
		t.Errorf("Delta at 0.5: got %v, want 50", got)
	}
	// Different item.
	if got := store.Delta(0, 1, []float64{1}); math.Abs(got-(-50)) > 1e-9 {
		t.Errorf("Delta item 1 at 1: got %v, want -50", got)
	}
}

// buildHVAR wraps an IVS in a minimal HVAR header (no advanceMapping, so
// gid == inner index).
func buildHVAR(ivsData []byte) []byte {
	headerLen := 20
	out := make([]byte, headerLen+len(ivsData))
	binary.BigEndian.PutUint16(out[0:], 1)                 // major
	binary.BigEndian.PutUint16(out[2:], 0)                 // minor
	binary.BigEndian.PutUint32(out[4:], uint32(headerLen)) // ivsOff
	// advOff at +8 = 0 (direct mapping).
	copy(out[headerLen:], ivsData)
	return out
}

func TestHVARAdvanceWidthDelta(t *testing.T) {
	// One axis, one region (0, 1, 1), one IVD with 3 items.
	ivs := buildItemVariationStore(1,
		[][][3]float64{{{0, 1, 1}}},
		[][]int16{{10}, {20}, {30}},
	)
	data := buildHVAR(ivs)
	h, err := ParseHVAR(data)
	if err != nil {
		t.Fatal(err)
	}
	for gid, wantDelta := range map[uint16]float64{0: 10, 1: 20, 2: 30} {
		if got := h.AdvanceWidthDelta(gid, []float64{1}); math.Abs(got-wantDelta) > 1e-9 {
			t.Errorf("gid %d at axis=1: got %v, want %v", gid, got, wantDelta)
		}
	}
	// At axis=0: all deltas zero.
	if got := h.AdvanceWidthDelta(0, []float64{0}); math.Abs(got) > 1e-9 {
		t.Errorf("gid 0 at axis=0: got %v, want 0", got)
	}
}

func buildMVAR(ivsData []byte, records map[string][2]uint16) []byte {
	recSize := 8
	recCount := len(records)
	headerLen := 12
	// ivs comes last so we can compute the offset.
	recEnd := headerLen + recCount*recSize
	out := make([]byte, recEnd+len(ivsData))
	binary.BigEndian.PutUint16(out[0:], 1) // major
	binary.BigEndian.PutUint16(out[2:], 0) // minor
	binary.BigEndian.PutUint16(out[4:], 0) // reserved
	binary.BigEndian.PutUint16(out[6:], uint16(recSize))
	binary.BigEndian.PutUint16(out[8:], uint16(recCount))
	binary.BigEndian.PutUint16(out[10:], uint16(recEnd))

	// Stable iteration order by inserting in a sorted order for test
	// determinism (map iteration order is not stable). For the test we
	// don't rely on record order.
	i := 0
	for tag, oi := range records {
		off := headerLen + i*recSize
		binary.BigEndian.PutUint32(out[off:], uint32(MakeTag(tag)))
		binary.BigEndian.PutUint16(out[off+4:], oi[0])
		binary.BigEndian.PutUint16(out[off+6:], oi[1])
		i++
	}
	copy(out[recEnd:], ivsData)
	return out
}

func TestMVARMetricDelta(t *testing.T) {
	ivs := buildItemVariationStore(1,
		[][][3]float64{{{0, 1, 1}}},
		[][]int16{{50}, {-30}},
	)
	data := buildMVAR(ivs, map[string][2]uint16{
		"xhgt": {0, 0}, // x-height -> item 0 (delta 50)
		"cpht": {0, 1}, // cap-height -> item 1 (delta -30)
	})
	m, err := ParseMVAR(data)
	if err != nil {
		t.Fatal(err)
	}
	coords := []float64{1}
	if got := m.MetricDelta("xhgt", coords); math.Abs(got-50) > 1e-9 {
		t.Errorf("xhgt: got %v, want 50", got)
	}
	if got := m.MetricDelta("cpht", coords); math.Abs(got-(-30)) > 1e-9 {
		t.Errorf("cpht: got %v, want -30", got)
	}
	// Unknown tag.
	if got := m.MetricDelta("zzzz", coords); got != 0 {
		t.Errorf("unknown tag: got %v, want 0", got)
	}
}
