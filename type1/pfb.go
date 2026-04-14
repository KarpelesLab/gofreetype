// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

// Package type1 parses Adobe Type 1 PostScript fonts. Both the PFA
// (7-bit ASCII) and PFB (segmented binary) container formats are
// supported; inside both is a PostScript dictionary header, a body of
// eexec-encrypted charstrings, and an ASCII trailer. Type 1 fonts are
// legacy but are still commonly embedded in older PDFs and some system
// fonts.
package type1

import "fmt"

// FormatError reports a malformed Type 1 font.
type FormatError string

func (e FormatError) Error() string { return "type1: invalid: " + string(e) }

// UnsupportedError reports an unimplemented feature.
type UnsupportedError string

func (e UnsupportedError) Error() string { return "type1: unsupported: " + string(e) }

// PFB segment types. See "Adobe Type 1 Font Format", chapter 2.
const (
	pfbSegmentASCII  = 1 // Plain PostScript text (dictionary header or trailer)
	pfbSegmentBinary = 2 // Eexec-encrypted binary data
	pfbSegmentEOF    = 3 // End of file marker
)

// sections holds the three logical parts of a Type 1 font after PFB
// de-segmentation: the ASCII header, the binary (still eexec-encrypted)
// body, and the ASCII trailer.
type sections struct {
	header  []byte // ASCII PostScript before eexec body
	body    []byte // eexec-encrypted bytes
	trailer []byte // ASCII PostScript after eexec body
}

// splitPFB walks a Portable Font Binary file and returns the ASCII
// header, the concatenation of every binary segment, and the ASCII
// trailer. The input may contain interleaved ASCII + binary segments;
// the accumulated binary payload is the caller's eexec-encrypted stream.
func splitPFB(data []byte) (*sections, error) {
	s := &sections{}
	i := 0
	seenBinary := false
	for i < len(data) {
		if data[i] != 0x80 {
			return nil, FormatError(fmt.Sprintf("bad PFB marker 0x%02x at offset %d", data[i], i))
		}
		if i+1 >= len(data) {
			return nil, FormatError("truncated PFB segment header")
		}
		typ := data[i+1]
		if typ == pfbSegmentEOF {
			break
		}
		if i+6 > len(data) {
			return nil, FormatError("truncated PFB segment length")
		}
		length := int(uint32(data[i+2]) |
			uint32(data[i+3])<<8 |
			uint32(data[i+4])<<16 |
			uint32(data[i+5])<<24)
		start := i + 6
		end := start + length
		if end > len(data) {
			return nil, FormatError("PFB segment runs past EOF")
		}
		payload := data[start:end]
		switch typ {
		case pfbSegmentASCII:
			if !seenBinary {
				s.header = append(s.header, payload...)
			} else {
				s.trailer = append(s.trailer, payload...)
			}
		case pfbSegmentBinary:
			seenBinary = true
			s.body = append(s.body, payload...)
		default:
			return nil, FormatError(fmt.Sprintf("unknown PFB segment type %d", typ))
		}
		i = end
	}
	return s, nil
}

// IsPFB reports whether data starts with a valid PFB segment marker.
func IsPFB(data []byte) bool {
	return len(data) >= 6 && data[0] == 0x80 && (data[1] == pfbSegmentASCII || data[1] == pfbSegmentBinary)
}
