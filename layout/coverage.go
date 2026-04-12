// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package layout

import "fmt"

// A Coverage table tells a lookup which glyphs it applies to, and gives
// each matched glyph a zero-based coverage index that indexes into the
// lookup's per-glyph data.
type Coverage struct {
	// Format 1 layout.
	glyphs []uint16
	// Format 2 layout: sorted [start, end] ranges with a starting coverage
	// index.
	rangeStart []uint16
	rangeEnd   []uint16
	rangeIdx   []uint16
}

// ParseCoverage decodes a Coverage table starting at data[off:]. It does
// not consume any trailing bytes beyond the table itself.
func ParseCoverage(data []byte, off int) (*Coverage, error) {
	if off+4 > len(data) {
		return nil, FormatError("Coverage header truncated")
	}
	format := u16(data, off)
	switch format {
	case 1:
		n := int(u16(data, off+2))
		if off+4+2*n > len(data) {
			return nil, FormatError("Coverage format 1 body truncated")
		}
		c := &Coverage{glyphs: make([]uint16, n)}
		for i := 0; i < n; i++ {
			c.glyphs[i] = u16(data, off+4+2*i)
		}
		return c, nil
	case 2:
		n := int(u16(data, off+2))
		if off+4+6*n > len(data) {
			return nil, FormatError("Coverage format 2 body truncated")
		}
		c := &Coverage{
			rangeStart: make([]uint16, n),
			rangeEnd:   make([]uint16, n),
			rangeIdx:   make([]uint16, n),
		}
		for i := 0; i < n; i++ {
			c.rangeStart[i] = u16(data, off+4+6*i)
			c.rangeEnd[i] = u16(data, off+4+6*i+2)
			c.rangeIdx[i] = u16(data, off+4+6*i+4)
		}
		return c, nil
	}
	return nil, UnsupportedError(fmt.Sprintf("Coverage format %d", format))
}

// Index returns the zero-based coverage index of g in the Coverage table,
// or -1 if g is not covered.
func (c *Coverage) Index(g uint16) int {
	if c == nil {
		return -1
	}
	if c.glyphs != nil {
		lo, hi := 0, len(c.glyphs)
		for lo < hi {
			m := lo + (hi-lo)/2
			if c.glyphs[m] < g {
				lo = m + 1
			} else if c.glyphs[m] > g {
				hi = m
			} else {
				return m
			}
		}
		return -1
	}
	lo, hi := 0, len(c.rangeStart)
	for lo < hi {
		m := lo + (hi-lo)/2
		if c.rangeEnd[m] < g {
			lo = m + 1
		} else if c.rangeStart[m] > g {
			hi = m
		} else {
			return int(c.rangeIdx[m]) + int(g-c.rangeStart[m])
		}
	}
	return -1
}

// Len returns the total number of covered glyphs.
func (c *Coverage) Len() int {
	if c == nil {
		return 0
	}
	if c.glyphs != nil {
		return len(c.glyphs)
	}
	total := 0
	for i := range c.rangeStart {
		total += int(c.rangeEnd[i]) - int(c.rangeStart[i]) + 1
	}
	return total
}
