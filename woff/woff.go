// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

// Package woff decodes the Web Open Font Format containers (WOFF 1.0
// and WOFF 2.0) into plain SFNT bytes that the truetype package can
// parse. The goal is transparent web-font loading: callers do not need
// to know whether a .woff / .woff2 file landed on disk versus a raw
// .ttf / .otf — they call Decode and hand the result to truetype.Parse.
//
// This file implements WOFF 1.0 (zlib-compressed table data). WOFF 2.0
// (brotli-compressed + glyph table transformations) lives in woff2.go.
package woff

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
)

// FormatError reports a malformed WOFF container.
type FormatError string

func (e FormatError) Error() string { return "woff: invalid: " + string(e) }

// UnsupportedError reports an unimplemented feature.
type UnsupportedError string

func (e UnsupportedError) Error() string { return "woff: unsupported: " + string(e) }

// The four WOFF signature values.
const (
	signatureWOFF1 = 0x774F4646 // "wOFF"
	signatureWOFF2 = 0x774F4632 // "wOF2"
)

// IsWOFF reports whether data has a WOFF 1.0 or 2.0 signature. Call
// Decode to actually unpack it.
func IsWOFF(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	sig := binary.BigEndian.Uint32(data[0:4])
	return sig == signatureWOFF1 || sig == signatureWOFF2
}

// Decode unpacks a WOFF container and returns the plain SFNT bytes.
// Accepts either WOFF 1.0 or WOFF 2.0 (if the woff2 support is available);
// non-WOFF input is returned unchanged as a convenience for callers that
// accept mixed web-font / raw-SFNT input.
func Decode(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, FormatError("WOFF data too short")
	}
	switch binary.BigEndian.Uint32(data[0:4]) {
	case signatureWOFF1:
		return decodeWOFF1(data)
	case signatureWOFF2:
		return decodeWOFF2(data)
	}
	// Not WOFF — assume plain SFNT and pass through.
	return data, nil
}

// decodeWOFF1 unpacks a WOFF 1.0 file.
//
// WOFF 1.0 header (44 bytes):
//
//	uint32 signature       ("wOFF")
//	uint32 flavor          (SFNT magic, typically 0x00010000 or "OTTO")
//	uint32 length          (total WOFF file size)
//	uint16 numTables
//	uint16 reserved
//	uint32 totalSfntSize
//	uint16 majorVersion
//	uint16 minorVersion
//	uint32 metaOffset
//	uint32 metaLength
//	uint32 metaOrigLength
//	uint32 privOffset
//	uint32 privLength
//
// Each table directory entry (20 bytes):
//
//	uint32 tag
//	uint32 offset
//	uint32 compLength
//	uint32 origLength
//	uint32 origChecksum
//
// When compLength < origLength, the table is zlib-compressed; otherwise
// the bytes at offset..offset+origLength are copied verbatim.
func decodeWOFF1(data []byte) ([]byte, error) {
	if len(data) < 44 {
		return nil, FormatError("WOFF 1.0 header too short")
	}
	flavor := binary.BigEndian.Uint32(data[4:8])
	numTables := int(binary.BigEndian.Uint16(data[12:14]))
	totalSfntSize := int(binary.BigEndian.Uint32(data[16:20]))
	dirStart := 44
	if dirStart+20*numTables > len(data) {
		return nil, FormatError("WOFF 1.0 directory truncated")
	}

	// Compute the SFNT header shape: 12-byte header + 16-byte per-table
	// directory entry. All tables follow, 4-byte aligned.
	out := make([]byte, 12+16*numTables)
	binary.BigEndian.PutUint32(out[0:4], flavor)
	binary.BigEndian.PutUint16(out[4:6], uint16(numTables))

	// searchRange / entrySelector / rangeShift — standard SFNT search hints.
	searchRange, entrySelector, rangeShift := sfntSearchHints(numTables)
	binary.BigEndian.PutUint16(out[6:8], searchRange)
	binary.BigEndian.PutUint16(out[8:10], entrySelector)
	binary.BigEndian.PutUint16(out[10:12], rangeShift)

	// Write table data sequentially. The order in the output SFNT matches
	// the WOFF directory order.
	cursor := 12 + 16*numTables
	for i := 0; i < numTables; i++ {
		entry := dirStart + 20*i
		tag := binary.BigEndian.Uint32(data[entry : entry+4])
		offset := binary.BigEndian.Uint32(data[entry+4 : entry+8])
		compLen := binary.BigEndian.Uint32(data[entry+8 : entry+12])
		origLen := binary.BigEndian.Uint32(data[entry+12 : entry+16])
		checksum := binary.BigEndian.Uint32(data[entry+16 : entry+20])

		if int(offset)+int(compLen) > len(data) {
			return nil, FormatError("WOFF 1.0 table body out of bounds")
		}
		var body []byte
		if compLen < origLen {
			r, err := zlib.NewReader(bytes.NewReader(data[offset : offset+compLen]))
			if err != nil {
				return nil, fmt.Errorf("woff: zlib: %w", err)
			}
			body, err = io.ReadAll(r)
			r.Close()
			if err != nil {
				return nil, fmt.Errorf("woff: zlib read: %w", err)
			}
			if uint32(len(body)) != origLen {
				return nil, FormatError("WOFF 1.0 decompressed length mismatch")
			}
		} else {
			body = make([]byte, origLen)
			copy(body, data[offset:offset+origLen])
		}

		// Write this table's directory entry.
		dirOut := 12 + 16*i
		binary.BigEndian.PutUint32(out[dirOut:dirOut+4], tag)
		binary.BigEndian.PutUint32(out[dirOut+4:dirOut+8], checksum)
		binary.BigEndian.PutUint32(out[dirOut+8:dirOut+12], uint32(cursor))
		binary.BigEndian.PutUint32(out[dirOut+12:dirOut+16], origLen)

		// Append table body, 4-byte aligned.
		need := cursor + int(origLen)
		if cap(out) < need {
			expanded := make([]byte, need)
			copy(expanded, out)
			out = expanded
		}
		if len(out) < need {
			out = out[:need]
		}
		copy(out[cursor:cursor+int(origLen)], body)
		cursor += int(origLen)
		// Align to 4 bytes.
		for cursor%4 != 0 {
			if len(out) <= cursor {
				out = append(out, 0)
			}
			cursor++
		}
	}

	// If totalSfntSize is specified and the output exceeds it, something is
	// off; otherwise leave the trailing padding alone.
	if totalSfntSize != 0 && len(out) > totalSfntSize {
		out = out[:totalSfntSize]
	}
	return out, nil
}

// decodeWOFF2 is a stub that returns an UnsupportedError. Full WOFF2
// requires a brotli decompressor and the glyf-table transform which is
// substantial enough to live in its own file.
func decodeWOFF2(data []byte) ([]byte, error) {
	return nil, UnsupportedError("WOFF 2.0 not yet implemented")
}

// sfntSearchHints computes the searchRange / entrySelector / rangeShift
// triple written into every SFNT header for binary-search-friendly
// directory layout.
func sfntSearchHints(numTables int) (searchRange, entrySelector, rangeShift uint16) {
	maxPow2 := 1
	selector := uint16(0)
	for maxPow2*2 <= numTables {
		maxPow2 *= 2
		selector++
	}
	searchRange = uint16(maxPow2 * 16)
	entrySelector = selector
	rangeShift = uint16(numTables*16) - searchRange
	return
}
