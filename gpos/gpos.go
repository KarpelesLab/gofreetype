// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

// Package gpos implements glyph positioning lookups from an OpenType GPOS
// table. The positioning logic is split by Lookup Type across multiple
// source files (pair.go, single.go, etc.). This file contains the shared
// ValueRecord decoder and per-lookup-kind dispatch.
package gpos

import (
	"fmt"

	"github.com/KarpelesLab/gofreetype/layout"
)

// Common errors.

type FormatError string

func (e FormatError) Error() string { return "gpos: invalid: " + string(e) }

type UnsupportedError string

func (e UnsupportedError) Error() string { return "gpos: unsupported: " + string(e) }

// Table is a parsed GPOS table, wrapping a layout.Table with GPOS-specific
// subtable dispatch done lazily on demand.
type Table struct {
	*layout.Table
}

// Parse decodes a GPOS table.
func Parse(data []byte) (*Table, error) {
	lt, err := layout.Parse(data)
	if err != nil {
		return nil, err
	}
	return &Table{Table: lt}, nil
}

// ValueRecord carries the subset of a GPOS ValueRecord we use for rendering.
// Device-table offsets are parsed but not yet applied; hinting support is a
// later phase.
type ValueRecord struct {
	XPlacement int16
	YPlacement int16
	XAdvance   int16
	YAdvance   int16
}

// IsZero reports whether this ValueRecord has no effect.
func (v ValueRecord) IsZero() bool {
	return v.XPlacement == 0 && v.YPlacement == 0 && v.XAdvance == 0 && v.YAdvance == 0
}

// valueRecordSize returns the byte size of a ValueRecord given a ValueFormat
// bitfield. Each bit names a uint16 field.
func valueRecordSize(valueFormat uint16) int {
	n := 0
	for b := valueFormat; b != 0; b >>= 1 {
		if b&1 != 0 {
			n += 2
		}
	}
	return n
}

// decodeValueRecord reads a ValueRecord described by valueFormat from
// data[off:]. It returns the decoded record and the number of bytes
// consumed. Device table offsets (value-format bits 0x20..0x80) are
// skipped over.
func decodeValueRecord(data []byte, off int, valueFormat uint16) (ValueRecord, int, error) {
	size := valueRecordSize(valueFormat)
	if off+size > len(data) {
		return ValueRecord{}, 0, FormatError("ValueRecord truncated")
	}
	var v ValueRecord
	p := off
	if valueFormat&0x0001 != 0 {
		v.XPlacement = int16(u16(data, p))
		p += 2
	}
	if valueFormat&0x0002 != 0 {
		v.YPlacement = int16(u16(data, p))
		p += 2
	}
	if valueFormat&0x0004 != 0 {
		v.XAdvance = int16(u16(data, p))
		p += 2
	}
	if valueFormat&0x0008 != 0 {
		v.YAdvance = int16(u16(data, p))
		p += 2
	}
	// 0x10 XPlaDevice, 0x20 YPlaDevice, 0x40 XAdvDevice, 0x80 YAdvDevice.
	// We skip these — they're device-resolution hints.
	if valueFormat&0x0010 != 0 {
		p += 2
	}
	if valueFormat&0x0020 != 0 {
		p += 2
	}
	if valueFormat&0x0040 != 0 {
		p += 2
	}
	if valueFormat&0x0080 != 0 {
		p += 2
	}
	return v, p - off, nil
}

// Pair looks up a GPOS pair-positioning adjustment for the pair
// (firstGlyph, secondGlyph). It tries each Lookup in order; the first
// matching Type-2 subtable whose Coverage includes firstGlyph determines
// the adjustment. If no pair match is found, both ValueRecords are zero
// and ok is false.
//
// Only the "kern" feature's lookups are conventionally used for kerning,
// but Pair is feature-agnostic: callers select which lookups to try by
// iterating LangSys.FeatureIndexes -> Features[...].LookupIndices.
func (t *Table) Pair(lookupIndex uint16, firstGlyph, secondGlyph uint16) (v1, v2 ValueRecord, ok bool) {
	if int(lookupIndex) >= len(t.Lookups) {
		return ValueRecord{}, ValueRecord{}, false
	}
	lk := t.Lookups[lookupIndex]
	actualType, subtables := resolveExtension(lk.Type, lk.SubtableData)
	if actualType != 2 {
		return ValueRecord{}, ValueRecord{}, false
	}
	for _, sub := range subtables {
		got1, got2, found, err := lookupPair(sub, firstGlyph, secondGlyph)
		if err != nil {
			continue
		}
		if found {
			return got1, got2, true
		}
	}
	return ValueRecord{}, ValueRecord{}, false
}

// PairKernAdvance returns the horizontal advance adjustment to apply between
// firstGlyph and secondGlyph for the given kerning-style lookup index. It is
// a convenience on top of Pair for the common case where a caller only
// cares about XAdvance of v1 (the first glyph).
func (t *Table) PairKernAdvance(lookupIndex uint16, firstGlyph, secondGlyph uint16) int16 {
	v1, _, _ := t.Pair(lookupIndex, firstGlyph, secondGlyph)
	return v1.XAdvance
}

// KernFeatureIndex returns the index of the "kern" feature in t.Features for
// the given (script, language), or -1 if absent.
func (t *Table) KernFeatureIndex(script, lang layout.Tag) int {
	ls := t.FindLanguage(script, lang)
	if ls == nil {
		return -1
	}
	kern := layout.MakeTag("kern")
	for _, idx := range ls.FeatureIndexes {
		if f := t.FindFeature(idx); f != nil && f.Tag == kern {
			return int(idx)
		}
	}
	return -1
}

// u16 / u32 are the same helpers used in layout, duplicated so that gpos
// does not have to re-export them.

func u16(b []byte, i int) uint16 {
	if i+1 >= len(b) {
		return 0
	}
	return uint16(b[i])<<8 | uint16(b[i+1])
}

func u32(b []byte, i int) uint32 {
	if i+3 >= len(b) {
		return 0
	}
	return uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3])
}

// intCount is a tiny helper for cases where we want to show an integer in
// an error message without pulling in strconv.
func intCount(n int) string {
	return fmt.Sprintf("%d", n)
}
