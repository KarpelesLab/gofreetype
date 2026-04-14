// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package woff

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// fakeBrotli is a stand-in decoder that the test suite uses in place of
// a real brotli implementation. We plug it in via RegisterBrotli and
// have it pass the bytes through unchanged — the synthetic WOFF2 files
// the test builds leave the "compressed" body uncompressed.
func fakeBrotli(src []byte) ([]byte, error) {
	out := make([]byte, len(src))
	copy(out, src)
	return out, nil
}

func TestReadBase128(t *testing.T) {
	cases := []struct {
		in   []byte
		want uint32
		n    int
	}{
		{[]byte{0x00}, 0, 1},
		{[]byte{0x7F}, 127, 1},
		{[]byte{0x81, 0x00}, 128, 2},
		{[]byte{0xFF, 0x7F}, 16383, 2},
		{[]byte{0x82, 0x80, 0x00}, 32768, 3},
	}
	for _, c := range cases {
		v, n, err := readBase128(c.in)
		if err != nil {
			t.Errorf("readBase128(%v): err %v", c.in, err)
			continue
		}
		if v != c.want || n != c.n {
			t.Errorf("readBase128(%v): got (%d, %d), want (%d, %d)", c.in, v, n, c.want, c.n)
		}
	}
}

func TestReadBase128LeadingZero(t *testing.T) {
	if _, _, err := readBase128([]byte{0x80, 0x01}); err == nil {
		t.Error("leading-zero encoding should error")
	}
}

// encodeBase128 produces the minimum-length base128 encoding of v.
func encodeBase128(v uint32) []byte {
	if v == 0 {
		return []byte{0}
	}
	// Count significant 7-bit groups, MSB first.
	var tmp [5]byte
	n := 0
	for tmp7 := v; tmp7 > 0; tmp7 >>= 7 {
		tmp[n] = byte(tmp7 & 0x7F)
		n++
	}
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		b := tmp[n-1-i]
		if i < n-1 {
			b |= 0x80
		}
		out[i] = b
	}
	return out
}

func TestEncodeDecodeBase128RoundTrip(t *testing.T) {
	for _, v := range []uint32{0, 1, 127, 128, 255, 1<<14 - 1, 1 << 14, 1<<21 - 1, 1 << 21, 1<<28 - 1} {
		enc := encodeBase128(v)
		got, n, err := readBase128(enc)
		if err != nil {
			t.Errorf("v=%d enc=%v: %v", v, enc, err)
			continue
		}
		if got != v || n != len(enc) {
			t.Errorf("v=%d: decoded (%d, %d bytes), want (%d, %d)", v, got, n, v, len(enc))
		}
	}
}

func TestDecodeWOFF2RequiresBrotli(t *testing.T) {
	RegisterBrotli(nil)
	defer RegisterBrotli(nil)
	data := []byte("wOF2" + string(make([]byte, 100)))
	_, err := Decode(data)
	if err == nil {
		t.Fatal("expected UnsupportedError without registered brotli")
	}
	if _, ok := err.(UnsupportedError); !ok {
		t.Errorf("expected UnsupportedError, got %T: %v", err, err)
	}
}

// buildMinimalWOFF2 constructs a fake WOFF2 with the given tables
// (each carrying identity transforms so no brotli-actual work is
// needed when fakeBrotli just passes the body through).
func buildMinimalWOFF2(flavor uint32, tables []struct {
	tag  string
	body []byte
}) []byte {
	// Build the directory + concatenated "compressed" body.
	var dir []byte
	var body []byte
	for _, tb := range tables {
		// Flag byte: table index in woff2KnownTableTags, or 0x3F + tag.
		flag := byte(0x3F)
		for i, t := range woff2KnownTableTags {
			if t == tb.tag {
				flag = byte(i)
				break
			}
		}
		// For CBDT, CBLC, cvt, kern we can't easily leave transformVer as 0
		// because that pair is reserved for glyf/loca default transforms.
		// For other tables any transformVer is fine; 3 is "null transform".
		if tb.tag == "glyf" || tb.tag == "loca" {
			flag |= 3 << 6 // transformVer = 3 (null transform)
		}
		dir = append(dir, flag)
		if flag&0x3F == 0x3F {
			// Explicit 4-byte tag.
			var tagBytes [4]byte
			for i := 0; i < 4; i++ {
				if i < len(tb.tag) {
					tagBytes[i] = tb.tag[i]
				} else {
					tagBytes[i] = ' '
				}
			}
			dir = append(dir, tagBytes[:]...)
		}
		dir = append(dir, encodeBase128(uint32(len(tb.body)))...)
		// No transformLength for non-default transforms.
		body = append(body, tb.body...)
	}

	// Header.
	headerLen := 48
	totalLen := headerLen + len(dir) + len(body)
	out := make([]byte, totalLen)
	copy(out[0:4], "wOF2")
	binary.BigEndian.PutUint32(out[4:8], flavor)
	binary.BigEndian.PutUint32(out[8:12], uint32(totalLen))
	binary.BigEndian.PutUint16(out[12:14], uint16(len(tables)))
	// reserved = 0
	binary.BigEndian.PutUint32(out[16:20], uint32(totalLen)) // totalSfntSize — not checked strictly
	binary.BigEndian.PutUint32(out[20:24], uint32(len(body)))
	// majorVersion / minorVersion / meta* / priv* left zero.

	copy(out[headerLen:], dir)
	copy(out[headerLen+len(dir):], body)
	return out
}

func TestDecodeWOFF2RoundTrip(t *testing.T) {
	RegisterBrotli(fakeBrotli)
	defer RegisterBrotli(nil)

	bigBody := bytes.Repeat([]byte{'A'}, 200)
	tables := []struct {
		tag  string
		body []byte
	}{
		{"head", bytes.Repeat([]byte{'H'}, 54)},
		{"name", bigBody},
		{"post", []byte{1, 2, 3, 4}},
	}
	w2 := buildMinimalWOFF2(0x00010000, tables)
	if !IsWOFF(w2) {
		t.Fatal("synthetic WOFF2 blob is not detected as WOFF")
	}
	sfnt, err := Decode(w2)
	if err != nil {
		t.Fatal(err)
	}
	// Flavor + numTables check.
	if got := binary.BigEndian.Uint32(sfnt[0:4]); got != 0x00010000 {
		t.Errorf("flavor: got %#x, want 0x00010000", got)
	}
	if got := binary.BigEndian.Uint16(sfnt[4:6]); int(got) != len(tables) {
		t.Errorf("numTables: got %d, want %d", got, len(tables))
	}
	// Body round-trip check.
	for i, tb := range tables {
		rec := 12 + 16*i
		wantTag := tb.tag
		for len(wantTag) < 4 {
			wantTag += " "
		}
		if gotTag := string(sfnt[rec : rec+4]); gotTag != wantTag {
			t.Errorf("table %d tag: got %q, want %q", i, gotTag, wantTag)
			continue
		}
		off := binary.BigEndian.Uint32(sfnt[rec+8 : rec+12])
		length := binary.BigEndian.Uint32(sfnt[rec+12 : rec+16])
		body := sfnt[off : off+length]
		if !bytes.Equal(body, tb.body) {
			t.Errorf("table %d body mismatch", i)
		}
	}
}
