// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package varfont

import "fmt"

// STAT is the parsed Style Attributes table. Variable-font pickers use
// STAT to render axis names ("Weight", "Width"), value names ("Thin",
// "Regular", "Black"), and the "elidable fallback" name that's implied
// when no axis values differ from the default.
type STAT struct {
	// DesignAxes describes each variation axis referenced by STAT. Note
	// that this list is independent from fvar's axis list; a font can
	// STAT-describe axes that aren't variable (they might be style
	// dimensions described only via naming).
	DesignAxes []STATAxis

	// AxisValues lists every AxisValue record. Each record links a
	// numeric position (or range) to a name ID so UIs can label it.
	AxisValues []STATAxisValue

	// ElidedFallbackNameID is the name ID of the fallback style that
	// applies when all axis values equal their defaults. 0xFFFE means
	// absent.
	ElidedFallbackNameID uint16
}

// STATAxis is one Design Axis record from STAT.
type STATAxis struct {
	Tag      Tag
	NameID   uint16
	Ordering uint16
}

// STATAxisValueFormat identifies the layout of a STATAxisValue entry.
type STATAxisValueFormat uint8

const (
	STATFormat1 STATAxisValueFormat = 1 // one axis, one value, one name
	STATFormat2 STATAxisValueFormat = 2 // one axis, value + range, one name
	STATFormat3 STATAxisValueFormat = 3 // one axis, value + linked value, one name
	STATFormat4 STATAxisValueFormat = 4 // multiple axes (each with its own value), one name
)

// STATAxisValue is one Axis Value Table. Format determines which fields
// are meaningful.
type STATAxisValue struct {
	Format       STATAxisValueFormat
	AxisIndex    uint16  // formats 1..3
	Flags        uint16
	ValueNameID  uint16
	Value        float64 // formats 1..3
	RangeMinValue float64 // format 2 only
	RangeMaxValue float64 // format 2 only
	LinkedValue  float64 // format 3 only
	AxisValues   []STATMultiAxisValue // format 4 only
}

// STATMultiAxisValue is one (axis index, value) pair in a format-4
// Axis Value Table.
type STATMultiAxisValue struct {
	AxisIndex uint16
	Value     float64
}

// ParseSTAT decodes a STAT table.
//
// Header (20 bytes minimum):
//
//	uint16 majorVersion (1)
//	uint16 minorVersion (1 or 2)
//	uint16 designAxisSize (8)
//	uint16 designAxisCount
//	Offset32 designAxesOffset
//	uint16 axisValueCount
//	Offset32 offsetToAxisValueOffsets
//	uint16 elidedFallbackNameID   (v1.1+)
func ParseSTAT(data []byte) (*STAT, error) {
	if len(data) < 20 {
		return nil, FormatError("STAT header too short")
	}
	major := u16(data, 0)
	minor := u16(data, 2)
	if major != 1 {
		return nil, UnsupportedError(fmt.Sprintf("STAT major version %d", major))
	}
	axisSize := int(u16(data, 4))
	axisCount := int(u16(data, 6))
	axesOff := int(u32(data, 8))
	valueCount := int(u16(data, 12))
	valueOffsetsOff := int(u32(data, 14))
	elidedFallback := uint16(0xFFFE)
	if minor >= 1 {
		if len(data) < 20 {
			return nil, FormatError("STAT v1.1 header truncated")
		}
		elidedFallback = u16(data, 18)
	}

	if axisSize < 8 {
		return nil, FormatError("STAT designAxisSize < 8")
	}
	if axesOff+axisSize*axisCount > len(data) {
		return nil, FormatError("STAT design axes out of bounds")
	}

	st := &STAT{
		ElidedFallbackNameID: elidedFallback,
		DesignAxes:           make([]STATAxis, axisCount),
	}
	for i := 0; i < axisCount; i++ {
		off := axesOff + axisSize*i
		st.DesignAxes[i] = STATAxis{
			Tag:      Tag(u32(data, off)),
			NameID:   u16(data, off+4),
			Ordering: u16(data, off+6),
		}
	}

	if valueCount > 0 && valueOffsetsOff != 0 {
		if valueOffsetsOff+2*valueCount > len(data) {
			return nil, FormatError("STAT axis value offsets out of bounds")
		}
		st.AxisValues = make([]STATAxisValue, 0, valueCount)
		for i := 0; i < valueCount; i++ {
			off := valueOffsetsOff + int(u16(data, valueOffsetsOff+2*i))
			av, err := parseSTATAxisValue(data, off)
			if err != nil {
				// Skip malformed entries rather than fail the whole parse.
				continue
			}
			st.AxisValues = append(st.AxisValues, av)
		}
	}
	return st, nil
}

func parseSTATAxisValue(data []byte, off int) (STATAxisValue, error) {
	if off+4 > len(data) {
		return STATAxisValue{}, FormatError("STAT axis value header truncated")
	}
	format := STATAxisValueFormat(u16(data, off))
	av := STATAxisValue{Format: format}
	switch format {
	case STATFormat1:
		if off+12 > len(data) {
			return STATAxisValue{}, FormatError("STAT format 1 truncated")
		}
		av.AxisIndex = u16(data, off+2)
		av.Flags = u16(data, off+4)
		av.ValueNameID = u16(data, off+6)
		av.Value = float64(int32(u32(data, off+8))) / 65536.0
	case STATFormat2:
		if off+20 > len(data) {
			return STATAxisValue{}, FormatError("STAT format 2 truncated")
		}
		av.AxisIndex = u16(data, off+2)
		av.Flags = u16(data, off+4)
		av.ValueNameID = u16(data, off+6)
		av.Value = float64(int32(u32(data, off+8))) / 65536.0
		av.RangeMinValue = float64(int32(u32(data, off+12))) / 65536.0
		av.RangeMaxValue = float64(int32(u32(data, off+16))) / 65536.0
	case STATFormat3:
		if off+16 > len(data) {
			return STATAxisValue{}, FormatError("STAT format 3 truncated")
		}
		av.AxisIndex = u16(data, off+2)
		av.Flags = u16(data, off+4)
		av.ValueNameID = u16(data, off+6)
		av.Value = float64(int32(u32(data, off+8))) / 65536.0
		av.LinkedValue = float64(int32(u32(data, off+12))) / 65536.0
	case STATFormat4:
		if off+8 > len(data) {
			return STATAxisValue{}, FormatError("STAT format 4 truncated")
		}
		count := int(u16(data, off+2))
		av.Flags = u16(data, off+4)
		av.ValueNameID = u16(data, off+6)
		body := off + 8
		if body+8*count > len(data) {
			return STATAxisValue{}, FormatError("STAT format 4 body truncated")
		}
		av.AxisValues = make([]STATMultiAxisValue, count)
		for i := 0; i < count; i++ {
			av.AxisValues[i] = STATMultiAxisValue{
				AxisIndex: u16(data, body+8*i),
				Value:     float64(int32(u32(data, body+8*i+2))) / 65536.0,
			}
		}
	default:
		return STATAxisValue{}, UnsupportedError(fmt.Sprintf("STAT axis value format %d", format))
	}
	return av, nil
}
