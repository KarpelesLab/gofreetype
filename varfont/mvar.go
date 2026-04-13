// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package varfont

import "fmt"

// MVAR is the parsed MVAR (Metric Variations) table. It keeps per-tag
// deltas for font-wide metrics like ascender, descender, x-height, etc.
// Applications apply these deltas to the base hhea/OS2/post metric values
// when the font is evaluated at a non-default axis coordinate.
type MVAR struct {
	store   *ItemVariationStore
	records map[Tag]mvarRecord
}

type mvarRecord struct {
	outer, inner uint16
}

// Well-known MVAR tags. A font is not required to list every tag; callers
// should treat a missing tag as "zero delta".
//
// Full list at:
// https://learn.microsoft.com/en-us/typography/opentype/spec/mvar
const (
	TagHorizAscender       = "hasc"
	TagHorizDescender      = "hdsc"
	TagHorizLineGap        = "hlgp"
	TagHorizCaretRise      = "hcrs"
	TagHorizCaretRun       = "hcrn"
	TagHorizCaretOffset    = "hcof"
	TagVerticalAscender    = "vasc"
	TagVerticalDescender   = "vdsc"
	TagVerticalLineGap     = "vlgp"
	TagVerticalCaretRise   = "vcrs"
	TagVerticalCaretRun    = "vcrn"
	TagVerticalCaretOffset = "vcof"
	TagXHeight             = "xhgt"
	TagCapHeight           = "cpht"
	TagStrikeoutSize       = "strs"
	TagStrikeoutOffset     = "stro"
	TagSubscriptXSize      = "sbxs"
	TagSubscriptYSize      = "sbys"
	TagSubscriptXOffset    = "sbxo"
	TagSubscriptYOffset    = "sbyo"
	TagSuperscriptXSize    = "spxs"
	TagSuperscriptYSize    = "spys"
	TagSuperscriptXOffset  = "spxo"
	TagSuperscriptYOffset  = "spyo"
	TagUnderlineSize       = "undo"
	TagUnderlineOffset     = "unds"
)

// ParseMVAR decodes an MVAR table.
//
// Header:
//
//	uint16 majorVersion (1)
//	uint16 minorVersion (0)
//	uint16 reserved
//	uint16 valueRecordSize   (8)
//	uint16 valueRecordCount
//	Offset16 itemVariationStoreOffset
//	ValueRecord valueRecords[valueRecordCount]
//
// ValueRecord (8 bytes):
//
//	Tag valueTag
//	uint16 deltaSetOuterIndex
//	uint16 deltaSetInnerIndex
func ParseMVAR(data []byte) (*MVAR, error) {
	if len(data) < 12 {
		return nil, FormatError("MVAR header too short")
	}
	recSize := int(u16(data, 6))
	recCount := int(u16(data, 8))
	ivsOff := int(u16(data, 10))

	if recSize != 8 {
		return nil, UnsupportedError(fmt.Sprintf("MVAR record size %d (expected 8)", recSize))
	}

	store, err := ParseItemVariationStore(data, ivsOff)
	if err != nil {
		return nil, err
	}

	m := &MVAR{store: store, records: make(map[Tag]mvarRecord, recCount)}
	recStart := 12
	if recStart+recCount*recSize > len(data) {
		return nil, FormatError("MVAR value records truncated")
	}
	for i := 0; i < recCount; i++ {
		off := recStart + i*recSize
		tag := Tag(u32(data, off))
		outer := u16(data, off+4)
		inner := u16(data, off+6)
		m.records[tag] = mvarRecord{outer: outer, inner: inner}
	}
	return m, nil
}

// MetricDelta returns the delta (in font units) to apply to the named
// metric at the given normalized axis coordinates. Unknown tags return 0.
func (m *MVAR) MetricDelta(tag string, coords []float64) float64 {
	if m == nil {
		return 0
	}
	rec, ok := m.records[MakeTag(tag)]
	if !ok {
		return 0
	}
	return m.store.Delta(rec.outer, rec.inner, coords)
}
