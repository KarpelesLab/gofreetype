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

// buildFVar builds a synthetic fvar with the given axes and instances.
// Each axis: (tag, min, default, max, flags, nameID).
// Each instance: (nameID, flags, coordinates, psID). If psID is 0, the
// PostScriptID slot is omitted.
func buildFVar(axes []VariationAxis, instances []NamedInstance, includePS bool) []byte {
	axisCount := len(axes)
	instanceSize := 4 + 4*axisCount
	if includePS {
		instanceSize += 2
	}
	headerLen := 16
	axesOff := headerLen
	instOff := axesOff + 20*axisCount
	totalLen := instOff + instanceSize*len(instances)

	b := make([]byte, totalLen)
	binary.BigEndian.PutUint16(b[0:], 1)                 // major
	binary.BigEndian.PutUint16(b[2:], 0)                 // minor
	binary.BigEndian.PutUint16(b[4:], uint16(axesOff))
	binary.BigEndian.PutUint16(b[6:], 2)                 // reserved
	binary.BigEndian.PutUint16(b[8:], uint16(axisCount))
	binary.BigEndian.PutUint16(b[10:], 20)                // axisSize
	binary.BigEndian.PutUint16(b[12:], uint16(len(instances)))
	binary.BigEndian.PutUint16(b[14:], uint16(instanceSize))

	for i, a := range axes {
		off := axesOff + 20*i
		binary.BigEndian.PutUint32(b[off:], uint32(a.Tag))
		binary.BigEndian.PutUint32(b[off+4:], uint32(int32(a.Min*65536)))
		binary.BigEndian.PutUint32(b[off+8:], uint32(int32(a.Default*65536)))
		binary.BigEndian.PutUint32(b[off+12:], uint32(int32(a.Max*65536)))
		binary.BigEndian.PutUint16(b[off+16:], a.Flags)
		binary.BigEndian.PutUint16(b[off+18:], a.Name)
	}
	for i, inst := range instances {
		off := instOff + instanceSize*i
		binary.BigEndian.PutUint16(b[off:], inst.Name)
		binary.BigEndian.PutUint16(b[off+2:], inst.Flags)
		for a, c := range inst.Coordinates {
			binary.BigEndian.PutUint32(b[off+4+4*a:], uint32(int32(c*65536)))
		}
		if includePS {
			binary.BigEndian.PutUint16(b[off+4+4*axisCount:], inst.PostScriptID)
		}
	}
	return b
}

func TestFVarBasic(t *testing.T) {
	axes := []VariationAxis{
		{Tag: MakeTag("wght"), Min: 100, Default: 400, Max: 900, Name: 256},
		{Tag: MakeTag("wdth"), Min: 50, Default: 100, Max: 200, Name: 257},
	}
	instances := []NamedInstance{
		{Name: 300, Coordinates: []float64{400, 100}},
		{Name: 301, Coordinates: []float64{700, 100}},
	}
	data := buildFVar(axes, instances, false)
	f, err := ParseFVar(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Axes) != 2 {
		t.Fatalf("axes: got %d, want 2", len(f.Axes))
	}
	if f.Axes[0].Tag.String() != "wght" {
		t.Errorf("axis[0].Tag: got %s, want wght", f.Axes[0].Tag)
	}
	if f.Axes[0].Default != 400 || f.Axes[0].Min != 100 || f.Axes[0].Max != 900 {
		t.Errorf("axis[0]: got %+v", f.Axes[0])
	}
	if len(f.Instances) != 2 {
		t.Fatalf("instances: got %d, want 2", len(f.Instances))
	}
	if f.Instances[1].Coordinates[0] != 700 {
		t.Errorf("instance[1] wght: got %v, want 700", f.Instances[1].Coordinates[0])
	}
}

func TestNormalizeAxisValue(t *testing.T) {
	axis := VariationAxis{Min: 100, Default: 400, Max: 900}
	cases := []struct {
		in   float64
		want float64
	}{
		{100, -1},
		{400, 0},
		{900, 1},
		{250, -0.5},
		{650, 0.5},
		{50, -1},   // clamp below
		{1000, 1}, // clamp above
	}
	for _, tc := range cases {
		got := axis.NormalizeAxisValue(tc.in)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("NormalizeAxisValue(%v): got %v, want %v", tc.in, got, tc.want)
		}
	}
}

// buildAVar builds a synthetic avar table with the given per-axis segment
// maps.
func buildAVar(maps []AxisSegmentMap) []byte {
	var b []byte
	b = append(b, 0, 1, 0, 0, 0, 0) // major/minor/reserved
	b = binary.BigEndian.AppendUint16(b, uint16(len(maps)))
	for _, m := range maps {
		b = binary.BigEndian.AppendUint16(b, uint16(len(m.FromCoord)))
		for i := range m.FromCoord {
			b = binary.BigEndian.AppendUint16(b, uint16(int16(m.FromCoord[i]*16384)))
			b = binary.BigEndian.AppendUint16(b, uint16(int16(m.ToCoord[i]*16384)))
		}
	}
	return b
}

func TestAVarRemap(t *testing.T) {
	// Non-linear remap: (-1, -1), (0, 0.25), (1, 1). So midway between 0
	// and 1 should be 0.25 + (1-0.25)/2 = 0.625.
	data := buildAVar([]AxisSegmentMap{
		{FromCoord: []float64{-1, 0, 1}, ToCoord: []float64{-1, 0.25, 1}},
	})
	av, err := ParseAVar(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := av.Remap(0, 0); math.Abs(got-0.25) > 1e-9 {
		t.Errorf("Remap(0): got %v, want 0.25", got)
	}
	if got := av.Remap(0, 0.5); math.Abs(got-0.625) > 1e-9 {
		t.Errorf("Remap(0.5): got %v, want 0.625", got)
	}
	if got := av.Remap(0, -1); math.Abs(got-(-1)) > 1e-9 {
		t.Errorf("Remap(-1): got %v, want -1", got)
	}
	// Axis index out of range returns x unchanged.
	if got := av.Remap(5, 0.3); got != 0.3 {
		t.Errorf("Remap(5, 0.3) out of range: got %v, want 0.3", got)
	}
}
