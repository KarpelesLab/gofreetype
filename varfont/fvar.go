// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

// Package varfont parses the OpenType variable font tables: fvar (axis
// definitions and named instances), avar (user-to-normalized axis
// coordinate remapping), gvar (per-glyph point variation deltas), HVAR
// (horizontal metric variations), and MVAR (font-wide metric variations).
// The package exposes primitives for evaluating deltas at a given axis
// coordinate vector; applying them to glyph outlines is done by the
// truetype package.
package varfont

import (
	"fmt"
	"math"
)

// Tag is a 4-byte ASCII OpenType tag. Duplicated here to avoid a layout
// import just for one type.
type Tag uint32

// MakeTag builds a Tag from a 4-character string.
func MakeTag(s string) Tag {
	var b [4]byte
	for i := 0; i < 4; i++ {
		if i < len(s) {
			b[i] = s[i]
		} else {
			b[i] = ' '
		}
	}
	return Tag(uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]))
}

// String renders a Tag as its 4-character ASCII representation, trimmed.
func (t Tag) String() string {
	b := [4]byte{byte(t >> 24), byte(t >> 16), byte(t >> 8), byte(t)}
	n := 4
	for n > 0 && b[n-1] == ' ' {
		n--
	}
	return string(b[:n])
}

// FormatError reports a malformed variable-font structure.
type FormatError string

func (e FormatError) Error() string { return "varfont: invalid: " + string(e) }

// UnsupportedError reports an unimplemented feature.
type UnsupportedError string

func (e UnsupportedError) Error() string { return "varfont: unsupported: " + string(e) }

// VariationAxis is one axis declared in fvar. Min / Default / Max are in
// "user" coordinates (the values the application supplies; e.g. 400 for
// Regular weight).
type VariationAxis struct {
	Tag     Tag
	Min     float64
	Default float64
	Max     float64
	Flags   uint16 // 0x0001 = HIDDEN
	Name    uint16 // NameID for the axis label
}

// NamedInstance is one pre-declared instance in fvar, giving a name and
// a fixed coordinate vector.
type NamedInstance struct {
	Name         uint16 // NameID for the instance name
	Flags        uint16
	Coordinates  []float64 // length == len(Axes); one user-space coord per axis
	PostScriptID uint16    // NameID for PostScript name (0 if absent)
}

// FVar is the parsed fvar table.
type FVar struct {
	Axes      []VariationAxis
	Instances []NamedInstance
}

// ParseFVar decodes an fvar table.
//
// Header (16 bytes):
//
//	uint16 majorVersion (1)
//	uint16 minorVersion (0)
//	Offset16 axesArrayOffset
//	uint16 reserved (2)
//	uint16 axisCount
//	uint16 axisSize (20)
//	uint16 instanceCount
//	uint16 instanceSize (axisCount*4 + 4 or + 6)
//
// VariationAxisRecord (20 bytes):
//
//	Tag axisTag
//	Fixed minValue
//	Fixed defaultValue
//	Fixed maxValue
//	uint16 flags
//	uint16 axisNameID
//
// InstanceRecord (instanceSize bytes):
//
//	uint16 subfamilyNameID
//	uint16 flags
//	Fixed coordinates[axisCount]
//	[uint16 postScriptNameID] — present iff instanceSize == axisCount*4 + 6
func ParseFVar(data []byte) (*FVar, error) {
	if len(data) < 16 {
		return nil, FormatError("fvar header too short")
	}
	major := u16(data, 0)
	if major != 1 {
		return nil, UnsupportedError(fmt.Sprintf("fvar major version %d", major))
	}
	axesOff := int(u16(data, 4))
	axisCount := int(u16(data, 8))
	axisSize := int(u16(data, 10))
	instanceCount := int(u16(data, 12))
	instanceSize := int(u16(data, 14))
	if axisSize != 20 {
		return nil, UnsupportedError(fmt.Sprintf("fvar axisSize %d", axisSize))
	}
	if axesOff+axisSize*axisCount > len(data) {
		return nil, FormatError("fvar axis records out of bounds")
	}

	f := &FVar{
		Axes:      make([]VariationAxis, axisCount),
		Instances: make([]NamedInstance, instanceCount),
	}
	for i := 0; i < axisCount; i++ {
		off := axesOff + axisSize*i
		f.Axes[i] = VariationAxis{
			Tag:     Tag(u32(data, off)),
			Min:     fixed2_30ToFloat(int32(u32(data, off+4))),
			Default: fixed2_30ToFloat(int32(u32(data, off+8))),
			Max:     fixed2_30ToFloat(int32(u32(data, off+12))),
			Flags:   u16(data, off+16),
			Name:    u16(data, off+18),
		}
	}

	instOff := axesOff + axisSize*axisCount
	if instOff+instanceSize*instanceCount > len(data) {
		return nil, FormatError("fvar instance records out of bounds")
	}
	for i := 0; i < instanceCount; i++ {
		off := instOff + instanceSize*i
		if off+4+4*axisCount > len(data) {
			return nil, FormatError("fvar instance record truncated")
		}
		inst := NamedInstance{
			Name:        u16(data, off),
			Flags:       u16(data, off+2),
			Coordinates: make([]float64, axisCount),
		}
		for a := 0; a < axisCount; a++ {
			inst.Coordinates[a] = fixed2_30ToFloat(int32(u32(data, off+4+4*a)))
		}
		if instanceSize >= 4+4*axisCount+2 && off+4+4*axisCount+2 <= len(data) {
			inst.PostScriptID = u16(data, off+4+4*axisCount)
		}
		f.Instances[i] = inst
	}
	return f, nil
}

// fixed2_30ToFloat decodes an OpenType 16.16 signed fixed-point value.
// (fvar uses 16.16, not 2.30 — named for historical reasons).
func fixed2_30ToFloat(v int32) float64 {
	return float64(v) / 65536.0
}

// NormalizeAxisValue maps a user-space axis coordinate to the normalized
// [-1, 1] range used by variation tables. default maps to 0, min to -1,
// max to +1, with linear interpolation in between.
func (v *VariationAxis) NormalizeAxisValue(value float64) float64 {
	if value < v.Min {
		value = v.Min
	}
	if value > v.Max {
		value = v.Max
	}
	if value == v.Default {
		return 0
	}
	if value < v.Default {
		if v.Default == v.Min {
			return 0
		}
		return -(v.Default - value) / (v.Default - v.Min)
	}
	if v.Max == v.Default {
		return 0
	}
	return (value - v.Default) / (v.Max - v.Default)
}

// Clamp returns v clamped to [Min, Max].
func (v *VariationAxis) Clamp(value float64) float64 {
	if math.IsNaN(value) {
		return v.Default
	}
	if value < v.Min {
		return v.Min
	}
	if value > v.Max {
		return v.Max
	}
	return value
}

func u16(b []byte, i int) uint16 {
	return uint16(b[i])<<8 | uint16(b[i+1])
}

func u32(b []byte, i int) uint32 {
	return uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3])
}
