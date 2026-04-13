// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package woff

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"testing"
)

func TestIsWOFF(t *testing.T) {
	if !IsWOFF([]byte("wOFF")) {
		t.Error("IsWOFF(wOFF) = false")
	}
	if !IsWOFF([]byte("wOF2")) {
		t.Error("IsWOFF(wOF2) = false")
	}
	// Plain SFNT signature should not match.
	if IsWOFF([]byte{0, 1, 0, 0}) {
		t.Error("IsWOFF(TrueType magic) = true")
	}
}

func TestDecodePassThroughForSFNT(t *testing.T) {
	// A non-WOFF blob should pass through unchanged.
	input := []byte{0x00, 0x01, 0x00, 0x00, 'x', 'y', 'z'}
	out, err := Decode(input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, input) {
		t.Errorf("non-WOFF pass-through: got %v, want %v", out, input)
	}
}

// encodeTable zlib-compresses the body if shouldCompress is true.
func encodeTable(body []byte, shouldCompress bool) []byte {
	if !shouldCompress {
		return body
	}
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	w.Write(body)
	w.Close()
	return buf.Bytes()
}

// buildWOFF1 constructs a synthetic WOFF 1.0 file with the given table
// tags and bodies. Each table is compressed iff the zlib form is smaller.
func buildWOFF1(flavor uint32, tables []struct {
	tag  string
	body []byte
}) []byte {
	numTables := len(tables)
	dirStart := 44
	dataStart := dirStart + 20*numTables

	// Encode all bodies and lay them out.
	encoded := make([][]byte, numTables)
	origLens := make([]uint32, numTables)
	cursor := dataStart
	offsets := make([]uint32, numTables)
	for i, tb := range tables {
		origLens[i] = uint32(len(tb.body))
		compressed := encodeTable(tb.body, true)
		// Only use compressed form if smaller.
		if len(compressed) >= len(tb.body) {
			compressed = tb.body
		}
		encoded[i] = compressed
		offsets[i] = uint32(cursor)
		cursor += len(compressed)
		// 4-byte align between tables.
		for cursor%4 != 0 {
			cursor++
		}
	}

	out := make([]byte, cursor)
	// WOFF header.
	binary.BigEndian.PutUint32(out[0:4], signatureWOFF1)
	binary.BigEndian.PutUint32(out[4:8], flavor)
	binary.BigEndian.PutUint32(out[8:12], uint32(cursor))
	binary.BigEndian.PutUint16(out[12:14], uint16(numTables))
	// Remaining header fields can stay zero.

	for i, tb := range tables {
		entry := dirStart + 20*i
		// Tag (4 ASCII chars, padded).
		var tagBytes [4]byte
		for j := 0; j < 4; j++ {
			if j < len(tb.tag) {
				tagBytes[j] = tb.tag[j]
			} else {
				tagBytes[j] = ' '
			}
		}
		copy(out[entry:entry+4], tagBytes[:])
		binary.BigEndian.PutUint32(out[entry+4:entry+8], offsets[i])
		binary.BigEndian.PutUint32(out[entry+8:entry+12], uint32(len(encoded[i])))
		binary.BigEndian.PutUint32(out[entry+12:entry+16], origLens[i])
		// checksum left as zero.

		copy(out[offsets[i]:offsets[i]+uint32(len(encoded[i]))], encoded[i])
	}
	return out
}

func TestDecodeWOFF1RoundTrip(t *testing.T) {
	// Build tables large enough that zlib compression actually helps.
	bigBody := bytes.Repeat([]byte{'A'}, 200)
	smallBody := []byte{1, 2, 3, 4}
	tables := []struct {
		tag  string
		body []byte
	}{
		{"head", bytes.Repeat([]byte{'H'}, 54)},
		{"name", bigBody},
		{"post", smallBody},
	}
	woff := buildWOFF1(0x00010000, tables)
	if !IsWOFF(woff) {
		t.Fatal("buildWOFF1 produced a non-WOFF blob")
	}
	sfnt, err := Decode(woff)
	if err != nil {
		t.Fatal(err)
	}
	// Verify the SFNT header flavor and numTables.
	if got, want := binary.BigEndian.Uint32(sfnt[0:4]), uint32(0x00010000); got != want {
		t.Errorf("flavor: got %08x, want %08x", got, want)
	}
	if got := binary.BigEndian.Uint16(sfnt[4:6]); int(got) != len(tables) {
		t.Errorf("numTables: got %d, want %d", got, len(tables))
	}
	// Check each table round-trips.
	for i, tb := range tables {
		entry := 12 + 16*i
		// Tag.
		gotTag := string(sfnt[entry : entry+4])
		wantTag := tb.tag
		for len(wantTag) < 4 {
			wantTag += " "
		}
		if gotTag != wantTag {
			t.Errorf("table[%d] tag: got %q, want %q", i, gotTag, wantTag)
			continue
		}
		offset := binary.BigEndian.Uint32(sfnt[entry+8 : entry+12])
		length := binary.BigEndian.Uint32(sfnt[entry+12 : entry+16])
		body := sfnt[offset : offset+length]
		if !bytes.Equal(body, tb.body) {
			t.Errorf("table[%d] body mismatch: got %d bytes, want %d", i, len(body), len(tb.body))
		}
	}
}

func TestDecodeWOFF2Unsupported(t *testing.T) {
	// Minimal WOFF2 signature — should return UnsupportedError.
	data := []byte("wOF2xxxxxxxxxxxxxxxxxxxxxx")
	_, err := Decode(data)
	if err == nil {
		t.Fatal("expected UnsupportedError for WOFF2")
	}
	if _, ok := err.(UnsupportedError); !ok {
		t.Errorf("expected UnsupportedError, got %T: %v", err, err)
	}
}
