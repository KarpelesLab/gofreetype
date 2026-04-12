// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package cff

// parseFDSelect decodes the FDSelect table (CID-keyed fonts only), returning
// an array of length numGlyphs where element [gid] is the FDArray index for
// that glyph.
func parseFDSelect(data []byte, offset, numGlyphs int) ([]uint8, error) {
	if offset+1 > len(data) {
		return nil, FormatError("FDSelect truncated")
	}
	format := data[offset]
	offset++
	switch format {
	case 0:
		// Each glyph has a single byte naming its FD index.
		if offset+numGlyphs > len(data) {
			return nil, FormatError("FDSelect format 0 truncated")
		}
		out := make([]uint8, numGlyphs)
		copy(out, data[offset:offset+numGlyphs])
		return out, nil
	case 3:
		// Ranges: nRanges (u16), then nRanges entries of (first gid: u16, fd: u8),
		// then a sentinel gid: u16 which is the end-of-last-range.
		if offset+2 > len(data) {
			return nil, FormatError("FDSelect format 3 truncated")
		}
		n := int(u16(data, offset))
		offset += 2
		if offset+n*3+2 > len(data) {
			return nil, FormatError("FDSelect format 3 body truncated")
		}
		out := make([]uint8, numGlyphs)
		prevStart := int(u16(data, offset))
		prevFD := data[offset+2]
		offset += 3
		for i := 1; i < n; i++ {
			start := int(u16(data, offset))
			fd := data[offset+2]
			fillFDSelect(out, prevStart, start, prevFD)
			prevStart, prevFD = start, fd
			offset += 3
		}
		sentinel := int(u16(data, offset))
		fillFDSelect(out, prevStart, sentinel, prevFD)
		return out, nil
	}
	return nil, UnsupportedError("FDSelect format")
}

func fillFDSelect(out []uint8, from, to int, fd uint8) {
	if from < 0 {
		from = 0
	}
	if to > len(out) {
		to = len(out)
	}
	for i := from; i < to; i++ {
		out[i] = fd
	}
}
