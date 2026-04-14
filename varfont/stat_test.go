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

func buildSTATFormat1Value(axisIndex uint16, value float64, nameID uint16) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint16(b[0:], uint16(STATFormat1))
	binary.BigEndian.PutUint16(b[2:], axisIndex)
	binary.BigEndian.PutUint16(b[4:], 0) // flags
	binary.BigEndian.PutUint16(b[6:], nameID)
	binary.BigEndian.PutUint32(b[8:], uint32(int32(value*65536)))
	return b
}

// TestParseSTAT builds a STAT with one axis ("wght") and two Format 1
// axis values ("Regular" at 400 and "Bold" at 700).
func TestParseSTAT(t *testing.T) {
	axisCount := 1
	axisSize := 8
	headerLen := 20
	axesOff := headerLen
	// Two Format 1 axis value records, 12 bytes each.
	val1 := buildSTATFormat1Value(0, 400, 300)
	val2 := buildSTATFormat1Value(0, 700, 301)
	// AxisValueOffsets array (uint16 per value) follows the axes block.
	axisValueOffsetsOff := axesOff + axisSize*axisCount
	valuesStart := axisValueOffsetsOff + 2*2

	total := valuesStart + len(val1) + len(val2)
	data := make([]byte, total)
	// Header
	binary.BigEndian.PutUint16(data[0:], 1)               // major
	binary.BigEndian.PutUint16(data[2:], 1)               // minor
	binary.BigEndian.PutUint16(data[4:], uint16(axisSize))
	binary.BigEndian.PutUint16(data[6:], uint16(axisCount))
	binary.BigEndian.PutUint32(data[8:], uint32(axesOff))
	binary.BigEndian.PutUint16(data[12:], 2) // axisValueCount
	binary.BigEndian.PutUint32(data[14:], uint32(axisValueOffsetsOff))
	binary.BigEndian.PutUint16(data[18:], 2) // elidedFallbackNameID

	// Design axis: "wght".
	copy(data[axesOff:axesOff+4], "wght")
	binary.BigEndian.PutUint16(data[axesOff+4:], 256) // nameID
	binary.BigEndian.PutUint16(data[axesOff+6:], 0)   // ordering

	// Axis value offsets (relative to the offsets array start).
	val1Off := uint16(valuesStart - axisValueOffsetsOff)
	val2Off := val1Off + uint16(len(val1))
	binary.BigEndian.PutUint16(data[axisValueOffsetsOff:], val1Off)
	binary.BigEndian.PutUint16(data[axisValueOffsetsOff+2:], val2Off)

	copy(data[valuesStart:], val1)
	copy(data[valuesStart+len(val1):], val2)

	st, err := ParseSTAT(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.DesignAxes) != 1 {
		t.Fatalf("DesignAxes: got %d, want 1", len(st.DesignAxes))
	}
	if st.DesignAxes[0].Tag.String() != "wght" {
		t.Errorf("axis tag: got %s, want wght", st.DesignAxes[0].Tag)
	}
	if st.ElidedFallbackNameID != 2 {
		t.Errorf("ElidedFallbackNameID: got %d, want 2", st.ElidedFallbackNameID)
	}
	if len(st.AxisValues) != 2 {
		t.Fatalf("AxisValues: got %d, want 2", len(st.AxisValues))
	}
	if st.AxisValues[0].Format != STATFormat1 {
		t.Errorf("AxisValue[0] format: got %d, want %d", st.AxisValues[0].Format, STATFormat1)
	}
	if math.Abs(st.AxisValues[0].Value-400) > 1e-9 {
		t.Errorf("AxisValue[0] value: got %v, want 400", st.AxisValues[0].Value)
	}
	if st.AxisValues[0].ValueNameID != 300 {
		t.Errorf("AxisValue[0] nameID: got %d, want 300", st.AxisValues[0].ValueNameID)
	}
	if st.AxisValues[1].ValueNameID != 301 || math.Abs(st.AxisValues[1].Value-700) > 1e-9 {
		t.Errorf("AxisValue[1]: got %+v", st.AxisValues[1])
	}
}
