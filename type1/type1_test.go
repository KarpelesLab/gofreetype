// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package type1

import (
	"bytes"
	"testing"
)

func TestEexecRoundTrip(t *testing.T) {
	plain := []byte("dup 1 256 {/Encoding exch def} for\n")
	cipher := encryptEexec(plain)
	got := decryptEexec(cipher)
	if !bytes.Equal(got, plain) {
		t.Errorf("round-trip: got %q, want %q", got, plain)
	}
}

func TestCharstringRoundTrip(t *testing.T) {
	plain := []byte{0x8b, 0x8b, 22, 14} // push 0, push 0, hmoveto, endchar
	cipher := encryptCharstring(plain, 4)
	got := decryptCharstring(cipher, 4)
	if !bytes.Equal(got, plain) {
		t.Errorf("charstring round-trip: got %v, want %v", got, plain)
	}
}

func TestIsPFB(t *testing.T) {
	cases := []struct {
		in   []byte
		want bool
	}{
		{[]byte{}, false},
		{[]byte{0x80, 1, 0, 0, 0, 0}, true},
		{[]byte{0x80, 2, 0, 0, 0, 0}, true},
		{[]byte{0x80, 3, 0, 0, 0, 0}, false}, // EOF is not a start marker
		{[]byte("wOFF"), false},
	}
	for _, tc := range cases {
		if got := IsPFB(tc.in); got != tc.want {
			t.Errorf("IsPFB(%v): got %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSplitPFB(t *testing.T) {
	header := []byte("%!PS-AdobeFont-1.0\n/FontName /Test def\n")
	body := []byte{0x01, 0x02, 0x03, 0x04, 0xaa, 0xbb}
	trailer := []byte("mark currentfile closefile\ncleartomark\n")

	// Assemble PFB: [ASCII header segment][binary body segment][ASCII trailer segment][EOF marker].
	var pfb []byte
	appendSegment := func(typ byte, data []byte) {
		pfb = append(pfb, 0x80, typ)
		n := uint32(len(data))
		pfb = append(pfb,
			byte(n),
			byte(n>>8),
			byte(n>>16),
			byte(n>>24),
		)
		pfb = append(pfb, data...)
	}
	appendSegment(pfbSegmentASCII, header)
	appendSegment(pfbSegmentBinary, body)
	appendSegment(pfbSegmentASCII, trailer)
	pfb = append(pfb, 0x80, pfbSegmentEOF)

	s, err := splitPFB(pfb)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(s.header, header) {
		t.Errorf("header: got %q, want %q", s.header, header)
	}
	if !bytes.Equal(s.body, body) {
		t.Errorf("body: got %v, want %v", s.body, body)
	}
	if !bytes.Equal(s.trailer, trailer) {
		t.Errorf("trailer: got %q, want %q", s.trailer, trailer)
	}
}

func TestSplitPFBInterleaved(t *testing.T) {
	// Some PFB files interleave ASCII + binary across multiple segments.
	// Our splitter must concatenate them in order.
	var pfb []byte
	appendSeg := func(typ byte, data []byte) {
		pfb = append(pfb, 0x80, typ)
		n := uint32(len(data))
		pfb = append(pfb, byte(n), byte(n>>8), byte(n>>16), byte(n>>24))
		pfb = append(pfb, data...)
	}
	appendSeg(pfbSegmentASCII, []byte("header-part1 "))
	appendSeg(pfbSegmentASCII, []byte("header-part2"))
	appendSeg(pfbSegmentBinary, []byte{0xAA})
	appendSeg(pfbSegmentBinary, []byte{0xBB, 0xCC})
	appendSeg(pfbSegmentASCII, []byte("trailer"))
	pfb = append(pfb, 0x80, pfbSegmentEOF)

	s, err := splitPFB(pfb)
	if err != nil {
		t.Fatal(err)
	}
	if string(s.header) != "header-part1 header-part2" {
		t.Errorf("header: got %q", s.header)
	}
	if !bytes.Equal(s.body, []byte{0xAA, 0xBB, 0xCC}) {
		t.Errorf("body: got %v", s.body)
	}
	if string(s.trailer) != "trailer" {
		t.Errorf("trailer: got %q", s.trailer)
	}
}

func TestSplitPFBBadMagic(t *testing.T) {
	_, err := splitPFB([]byte{'X', 'X'})
	if err == nil {
		t.Fatal("expected error for non-PFB input")
	}
}
