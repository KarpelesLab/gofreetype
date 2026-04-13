// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package varfont

// HVAR is the parsed HVAR (Horizontal Metrics Variations) table. It carries
// per-glyph deltas for horizontal advance width (and optionally LSB/RSB)
// in font units, applied at a given normalized axis coordinate vector.
type HVAR struct {
	store          *ItemVariationStore
	advanceMapping *deltaSetIndexMap // nil = direct (outer=0, inner=gid)
}

// ParseHVAR decodes an HVAR table.
//
// Header:
//
//	uint16 majorVersion (1)
//	uint16 minorVersion (0)
//	Offset32 itemVariationStoreOffset
//	Offset32 advanceWidthMappingOffset  (may be 0)
//	Offset32 LSBMappingOffset           (optional)
//	Offset32 RSBMappingOffset           (optional)
func ParseHVAR(data []byte) (*HVAR, error) {
	if len(data) < 20 {
		return nil, FormatError("HVAR header too short")
	}
	ivsOff := int(u32(data, 4))
	advOff := int(u32(data, 8))

	store, err := ParseItemVariationStore(data, ivsOff)
	if err != nil {
		return nil, err
	}
	h := &HVAR{store: store}
	if advOff != 0 {
		m, err := parseDeltaSetIndexMap(data, advOff)
		if err != nil {
			return nil, err
		}
		h.advanceMapping = m
	}
	return h, nil
}

// AdvanceWidthDelta returns the variation delta (in font units) to add to
// the advance width of glyph gid at the given normalized coords. Returns
// 0 if the glyph has no variation.
func (h *HVAR) AdvanceWidthDelta(gid uint16, coords []float64) float64 {
	if h == nil {
		return 0
	}
	outer, inner := uint16(0), gid
	if h.advanceMapping != nil {
		outer, inner = h.advanceMapping.Lookup(gid)
	}
	return h.store.Delta(outer, inner, coords)
}

// deltaSetIndexMap maps a glyph id (or generic entry) to an
// (outerIndex, innerIndex) pair.
type deltaSetIndexMap struct {
	entryFormat uint8
	// Derived from entryFormat:
	innerBits      uint8  // low nibble + 1
	entrySize      int    // (high nibble >> 4) + 1
	entries        []byte // raw entry bytes, `entrySize` each
	mapCount       int
}

// Format layout:
//
//	v0: uint16 format, uint16 mapCount, then entries
//	v1: uint16 format, uint32 mapCount, then entries
func parseDeltaSetIndexMap(data []byte, off int) (*deltaSetIndexMap, error) {
	if off+4 > len(data) {
		return nil, FormatError("DeltaSetIndexMap header truncated")
	}
	format := u16(data, off)
	entryFormat := data[off+3]
	_ = data[off+2] // reserved / format upper

	m := &deltaSetIndexMap{entryFormat: entryFormat}
	m.innerBits = (entryFormat & 0x0F) + 1
	m.entrySize = int(((entryFormat>>4)&0x03)+1)

	var bodyStart int
	if format == 0 {
		if off+4 > len(data) {
			return nil, FormatError("DeltaSetIndexMap v0 header truncated")
		}
		m.mapCount = int(u16(data, off+2))
		bodyStart = off + 4
	} else {
		if off+8 > len(data) {
			return nil, FormatError("DeltaSetIndexMap v1 header truncated")
		}
		m.mapCount = int(u32(data, off+4))
		bodyStart = off + 8
	}
	if bodyStart+m.mapCount*m.entrySize > len(data) {
		return nil, FormatError("DeltaSetIndexMap entries truncated")
	}
	m.entries = data[bodyStart : bodyStart+m.mapCount*m.entrySize]
	return m, nil
}

// Lookup returns the (outerIndex, innerIndex) pair for the given glyph id.
// If gid is out of range, the last entry is returned (per the spec).
func (m *deltaSetIndexMap) Lookup(gid uint16) (uint16, uint16) {
	if m.mapCount == 0 {
		return 0, 0
	}
	idx := int(gid)
	if idx >= m.mapCount {
		idx = m.mapCount - 1
	}
	off := idx * m.entrySize
	var raw uint32
	for i := 0; i < m.entrySize; i++ {
		raw = (raw << 8) | uint32(m.entries[off+i])
	}
	innerMask := uint32(1)<<m.innerBits - 1
	inner := uint16(raw & innerMask)
	outer := uint16(raw >> m.innerBits)
	return outer, inner
}
