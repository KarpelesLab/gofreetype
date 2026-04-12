// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package layout

import "fmt"

// ClassDef maps glyph IDs to class numbers. Glyphs not explicitly listed
// belong to class 0.
type ClassDef struct {
	// Format 1: a single contiguous range [startGID, startGID+len(classes)).
	startGID uint16
	classes  []uint16
	// Format 2: arbitrary ranges.
	rangeStart []uint16
	rangeEnd   []uint16
	rangeClass []uint16
}

// ParseClassDef decodes a ClassDef table at data[off:].
func ParseClassDef(data []byte, off int) (*ClassDef, error) {
	if off+2 > len(data) {
		return nil, FormatError("ClassDef header truncated")
	}
	format := u16(data, off)
	switch format {
	case 1:
		if off+6 > len(data) {
			return nil, FormatError("ClassDef format 1 truncated")
		}
		start := u16(data, off+2)
		n := int(u16(data, off+4))
		if off+6+2*n > len(data) {
			return nil, FormatError("ClassDef format 1 body truncated")
		}
		c := &ClassDef{startGID: start, classes: make([]uint16, n)}
		for i := 0; i < n; i++ {
			c.classes[i] = u16(data, off+6+2*i)
		}
		return c, nil
	case 2:
		if off+4 > len(data) {
			return nil, FormatError("ClassDef format 2 truncated")
		}
		n := int(u16(data, off+2))
		if off+4+6*n > len(data) {
			return nil, FormatError("ClassDef format 2 body truncated")
		}
		c := &ClassDef{
			rangeStart: make([]uint16, n),
			rangeEnd:   make([]uint16, n),
			rangeClass: make([]uint16, n),
		}
		for i := 0; i < n; i++ {
			c.rangeStart[i] = u16(data, off+4+6*i)
			c.rangeEnd[i] = u16(data, off+4+6*i+2)
			c.rangeClass[i] = u16(data, off+4+6*i+4)
		}
		return c, nil
	}
	return nil, UnsupportedError(fmt.Sprintf("ClassDef format %d", format))
}

// Class returns the class of g, or 0 if g is not in the table.
func (c *ClassDef) Class(g uint16) uint16 {
	if c == nil {
		return 0
	}
	if c.classes != nil {
		if g >= c.startGID && int(g-c.startGID) < len(c.classes) {
			return c.classes[g-c.startGID]
		}
		return 0
	}
	lo, hi := 0, len(c.rangeStart)
	for lo < hi {
		m := lo + (hi-lo)/2
		if c.rangeEnd[m] < g {
			lo = m + 1
		} else if c.rangeStart[m] > g {
			hi = m
		} else {
			return c.rangeClass[m]
		}
	}
	return 0
}
