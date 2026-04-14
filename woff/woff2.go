// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package woff

import (
	"encoding/binary"
	"fmt"
)

// BrotliDecoder is any callable that takes a brotli-compressed byte
// slice and returns the uncompressed bytes. Go's standard library does
// not ship a brotli decompressor, so WOFF2 callers supply their own via
// RegisterBrotli — typical implementations wrap
// github.com/andybalholm/brotli.
type BrotliDecoder func(src []byte) ([]byte, error)

// registeredBrotli is a process-wide brotli decoder registered by the
// user. It's accessed only inside Decode, which runs on the caller's
// goroutine; no lock needed for typical register-once-then-decode-many
// usage.
var registeredBrotli BrotliDecoder

// RegisterBrotli installs the given brotli decompression function so
// woff.Decode (and therefore truetype.Parse) can unpack WOFF2 files.
// Pass nil to clear the registration.
//
// Typical setup in main:
//
//	import "github.com/andybalholm/brotli"
//	woff.RegisterBrotli(func(src []byte) ([]byte, error) {
//	    r := brotli.NewReader(bytes.NewReader(src))
//	    return io.ReadAll(r)
//	})
func RegisterBrotli(dec BrotliDecoder) {
	registeredBrotli = dec
}

// WOFF2 known-table tags. The WOFF2 spec numbers the 63 most common
// table tags so they can be referenced in the table directory via a
// 6-bit index instead of a 4-byte tag; a flag of 0x3F means "next 4
// bytes are the tag".
var woff2KnownTableTags = [...]string{
	"cmap", "head", "hhea", "hmtx", "maxp", "name", "OS/2", "post",
	"cvt ", "fpgm", "glyf", "loca", "prep", "CFF ", "VORG", "EBDT",
	"EBLC", "gasp", "hdmx", "kern", "LTSH", "PCLT", "VDMX", "vhea",
	"vmtx", "BASE", "GDEF", "GPOS", "GSUB", "EBSC", "JSTF", "MATH",
	"CBDT", "CBLC", "COLR", "CPAL", "SVG ", "sbix", "acnt", "avar",
	"bdat", "bloc", "bsln", "cvar", "fdsc", "feat", "fmtx", "fvar",
	"gvar", "hsty", "just", "lcar", "mort", "morx", "opbd", "prop",
	"trak", "Zapf", "Silf", "Glat", "Gloc", "Feat", "Sill",
}

// decodeWOFF2 unpacks a WOFF 2.0 file. Requires a brotli decoder to be
// registered via RegisterBrotli; returns UnsupportedError otherwise.
//
// WOFF 2.0 header (48 bytes):
//
//	uint32 signature       ("wOF2")
//	uint32 flavor          (SFNT magic)
//	uint32 length          (total WOFF2 file size)
//	uint16 numTables
//	uint16 reserved
//	uint32 totalSfntSize
//	uint32 totalCompressedSize
//	uint16 majorVersion
//	uint16 minorVersion
//	uint32 metaOffset
//	uint32 metaLength
//	uint32 metaOrigLength
//	uint32 privOffset
//	uint32 privLength
//
// After the header: a variable-length table directory (see
// parseWOFF2Directory), then the brotli-compressed table data blob.
func decodeWOFF2(data []byte) ([]byte, error) {
	if registeredBrotli == nil {
		return nil, UnsupportedError("WOFF2 requires a brotli decoder: call woff.RegisterBrotli first")
	}
	if len(data) < 48 {
		return nil, FormatError("WOFF2 header too short")
	}
	flavor := binary.BigEndian.Uint32(data[4:8])
	numTables := int(binary.BigEndian.Uint16(data[12:14]))
	totalSfntSize := int(binary.BigEndian.Uint32(data[16:20]))
	_ = totalSfntSize

	dir, dirEnd, err := parseWOFF2Directory(data[48:], numTables)
	if err != nil {
		return nil, err
	}
	// Brotli-compressed table body starts right after the directory.
	bodyStart := 48 + dirEnd
	totalCompSize := int(binary.BigEndian.Uint32(data[20:24]))
	if bodyStart+totalCompSize > len(data) {
		return nil, FormatError("WOFF2 compressed body runs past EOF")
	}
	compressed := data[bodyStart : bodyStart+totalCompSize]
	raw, err := registeredBrotli(compressed)
	if err != nil {
		return nil, fmt.Errorf("woff2: brotli decode: %w", err)
	}

	// Slice table bodies out of the decompressed blob.
	cursor := 0
	for i := range dir {
		end := cursor + int(dir[i].transformLength)
		if end > len(raw) {
			return nil, FormatError("WOFF2 decompressed body truncated")
		}
		dir[i].data = raw[cursor:end]
		cursor = end
	}

	return reconstructSFNT(flavor, dir)
}

// woff2TableEntry is one decoded table directory entry.
type woff2TableEntry struct {
	tag             [4]byte
	flags           byte
	transformVer    byte
	origLength      uint32
	transformLength uint32 // on-disk (post-transform) length in the brotli stream
	data            []byte // populated after brotli decompression
}

// parseWOFF2Directory reads numTables directory entries from data
// starting at offset 0, returning the slice and the byte count consumed.
func parseWOFF2Directory(data []byte, numTables int) ([]woff2TableEntry, int, error) {
	dir := make([]woff2TableEntry, numTables)
	i := 0
	for k := 0; k < numTables; k++ {
		if i >= len(data) {
			return nil, 0, FormatError("WOFF2 directory truncated (flags)")
		}
		flagByte := data[i]
		i++
		tagIdx := int(flagByte & 0x3F)
		transformVer := (flagByte >> 6) & 0x03

		if tagIdx == 0x3F {
			// Arbitrary 4-byte tag follows.
			if i+4 > len(data) {
				return nil, 0, FormatError("WOFF2 directory truncated (tag)")
			}
			copy(dir[k].tag[:], data[i:i+4])
			i += 4
		} else {
			if tagIdx >= len(woff2KnownTableTags) {
				return nil, 0, FormatError(fmt.Sprintf("unknown WOFF2 table index %d", tagIdx))
			}
			copy(dir[k].tag[:], woff2KnownTableTags[tagIdx])
		}
		dir[k].flags = flagByte
		dir[k].transformVer = transformVer

		origLen, n, err := readBase128(data[i:])
		if err != nil {
			return nil, 0, err
		}
		dir[k].origLength = origLen
		i += n

		// transformLength is present for glyf and loca with the default
		// transform (version 0). For every other case transformLength is
		// absent and transformLength == origLength.
		tag := string(dir[k].tag[:])
		hasTransform := (tag == "glyf" || tag == "loca") && transformVer == 0
		if hasTransform {
			tl, n, err := readBase128(data[i:])
			if err != nil {
				return nil, 0, err
			}
			dir[k].transformLength = tl
			i += n
		} else {
			dir[k].transformLength = origLen
		}
	}
	return dir, i, nil
}

// readBase128 reads a UIntBase128-encoded unsigned integer from the head
// of data. Each byte: high bit = continuation, low 7 bits = next chunk
// (MSB first). Values are at most 5 bytes; any encoding with a leading
// zero byte is invalid.
func readBase128(data []byte) (uint32, int, error) {
	var v uint32
	for i := 0; i < 5; i++ {
		if i >= len(data) {
			return 0, 0, FormatError("truncated base128 integer")
		}
		b := data[i]
		// No leading zero byte except for the value 0 itself encoded in
		// a single byte.
		if i == 0 && b == 0x80 {
			return 0, 0, FormatError("base128 integer has leading zero byte")
		}
		v = v<<7 | uint32(b&0x7F)
		if b&0x80 == 0 {
			return v, i + 1, nil
		}
	}
	return 0, 0, FormatError("base128 integer exceeds 5 bytes")
}

// reconstructSFNT builds a classic SFNT file from the WOFF2 decoded
// directory + table bodies. Tables whose transformVer == 0 for "glyf"
// (the default glyf transform) are not yet re-expanded; fonts that use
// the glyf transform currently surface as UnsupportedError. Tables with
// identity transform (transformVer == 3 on glyf/loca, or any other
// table) pass through.
func reconstructSFNT(flavor uint32, dir []woff2TableEntry) ([]byte, error) {
	n := len(dir)
	headerLen := 12 + 16*n
	cursor := headerLen
	// First pass: validate that we can lay everything out, and compute
	// 4-byte aligned offsets.
	offsets := make([]int, n)
	for i := range dir {
		if dir[i].transformVer == 0 && (string(dir[i].tag[:]) == "glyf" || string(dir[i].tag[:]) == "loca") {
			return nil, UnsupportedError("WOFF2 glyf/loca transform is not yet implemented")
		}
		// Pass-through table: origLength bytes.
		offsets[i] = cursor
		cursor += int(dir[i].origLength)
		for cursor%4 != 0 {
			cursor++
		}
	}
	out := make([]byte, cursor)
	binary.BigEndian.PutUint32(out[0:4], flavor)
	binary.BigEndian.PutUint16(out[4:6], uint16(n))
	searchRange, entrySelector, rangeShift := sfntSearchHints(n)
	binary.BigEndian.PutUint16(out[6:8], searchRange)
	binary.BigEndian.PutUint16(out[8:10], entrySelector)
	binary.BigEndian.PutUint16(out[10:12], rangeShift)
	for i := range dir {
		rec := 12 + 16*i
		copy(out[rec:rec+4], dir[i].tag[:])
		// checksum: left zero
		binary.BigEndian.PutUint32(out[rec+8:rec+12], uint32(offsets[i]))
		binary.BigEndian.PutUint32(out[rec+12:rec+16], dir[i].origLength)
		if len(dir[i].data) > 0 {
			copy(out[offsets[i]:offsets[i]+len(dir[i].data)], dir[i].data)
		}
	}
	return out, nil
}
