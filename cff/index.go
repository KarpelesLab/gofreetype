// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package cff

// parseIndex decodes a CFF INDEX starting at offset `off` in `data`.
// It returns the list of object byte-slices (sharing backing storage with
// `data`), the offset just past the INDEX, and any error.
//
// An INDEX with count 0 has the shape:
//
//	count: uint16 == 0
//
// (no offSize, no offsets, no data). This is legal and represents an empty
// INDEX.
//
// A non-empty INDEX has:
//
//	count:     uint16 (>= 1)
//	offSize:   uint8 (1..4)
//	offset:    [count+1] of offSize bytes, 1-based into data
//	data:      object data
func parseIndex(data []byte, off int) ([][]byte, int, error) {
	if off+2 > len(data) {
		return nil, 0, FormatError("INDEX count out of bounds")
	}
	count := int(u16(data, off))
	if count == 0 {
		return nil, off + 2, nil
	}
	off += 2

	if off+1 > len(data) {
		return nil, 0, FormatError("INDEX offSize out of bounds")
	}
	offSize := int(data[off])
	off++
	if offSize < 1 || offSize > 4 {
		return nil, 0, FormatError("INDEX offSize must be 1..4")
	}

	offsetsLen := (count + 1) * offSize
	if off+offsetsLen > len(data) {
		return nil, 0, FormatError("INDEX offsets out of bounds")
	}
	offsets := make([]int, count+1)
	for i := 0; i <= count; i++ {
		offsets[i] = readOffSize(data[off+i*offSize:], offSize)
		if offsets[i] < 1 {
			return nil, 0, FormatError("INDEX offset below 1")
		}
	}
	off += offsetsLen

	// The offsets are 1-based from the byte just before `off` (i.e., off-1 is
	// the "zeroth" data byte). Per the spec, offset[0] == 1.
	dataStart := off - 1
	objects := make([][]byte, count)
	for i := 0; i < count; i++ {
		a := dataStart + offsets[i]
		b := dataStart + offsets[i+1]
		if a < dataStart || b < a || b > len(data) {
			return nil, 0, FormatError("INDEX object offset out of bounds")
		}
		objects[i] = data[a:b]
	}
	return objects, dataStart + offsets[count], nil
}

// readOffSize reads a big-endian integer of width `size` (1..4) from b.
func readOffSize(b []byte, size int) int {
	switch size {
	case 1:
		return int(b[0])
	case 2:
		return int(b[0])<<8 | int(b[1])
	case 3:
		return int(b[0])<<16 | int(b[1])<<8 | int(b[2])
	case 4:
		return int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	}
	return 0
}

// u16 returns the big-endian uint16 at b[i:].
func u16(b []byte, i int) uint16 {
	return uint16(b[i])<<8 | uint16(b[i+1])
}

