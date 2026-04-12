// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import (
	"encoding/binary"
	"strings"
	"testing"
)

// mkCmap returns a synthetic cmap table containing a single subtable with the
// given PID/PSID, positioned after the 4-byte cmap header + 8-byte subtable
// record.
func mkCmap(pid, psid uint16, body []byte) []byte {
	cmap := make([]byte, 12+len(body))
	binary.BigEndian.PutUint16(cmap[0:], 0)  // version
	binary.BigEndian.PutUint16(cmap[2:], 1)  // numSubtables
	binary.BigEndian.PutUint16(cmap[4:], pid)
	binary.BigEndian.PutUint16(cmap[6:], psid)
	binary.BigEndian.PutUint32(cmap[8:], 12) // subtable offset
	copy(cmap[12:], body)
	return cmap
}

// mkCmapMulti returns a synthetic cmap table containing multiple subtables.
// Each entry's body is appended after the header/records block.
func mkCmapMulti(entries []struct {
	pid, psid uint16
	body      []byte
}) []byte {
	nSub := len(entries)
	headerLen := 4 + 8*nSub
	buf := make([]byte, headerLen)
	binary.BigEndian.PutUint16(buf[0:], 0)
	binary.BigEndian.PutUint16(buf[2:], uint16(nSub))
	off := uint32(headerLen)
	for i, e := range entries {
		binary.BigEndian.PutUint16(buf[4+8*i:], e.pid)
		binary.BigEndian.PutUint16(buf[4+8*i+2:], e.psid)
		binary.BigEndian.PutUint32(buf[4+8*i+4:], off)
		off += uint32(len(e.body))
	}
	for _, e := range entries {
		buf = append(buf, e.body...)
	}
	return buf
}

// buildFormat0Body builds a cmap format 0 subtable body (262 bytes).
func buildFormat0Body(mapping [256]byte) []byte {
	b := make([]byte, 262)
	binary.BigEndian.PutUint16(b[0:], 0)   // format
	binary.BigEndian.PutUint16(b[2:], 262) // length
	binary.BigEndian.PutUint16(b[4:], 0)   // language
	copy(b[6:], mapping[:])
	return b
}

// buildFormat6Body builds a cmap format 6 subtable body.
func buildFormat6Body(firstCode uint16, glyphIDs []uint16) []byte {
	length := 10 + 2*len(glyphIDs)
	b := make([]byte, length)
	binary.BigEndian.PutUint16(b[0:], 6)
	binary.BigEndian.PutUint16(b[2:], uint16(length))
	binary.BigEndian.PutUint16(b[4:], 0) // language
	binary.BigEndian.PutUint16(b[6:], firstCode)
	binary.BigEndian.PutUint16(b[8:], uint16(len(glyphIDs)))
	for i, g := range glyphIDs {
		binary.BigEndian.PutUint16(b[10+2*i:], g)
	}
	return b
}

// buildFormat4Body builds a minimal cmap format 4 subtable body with the
// given segments (end, start, delta). idRangeOffset is zero for every
// segment (direct delta mapping, no glyphIdArray indirection).
func buildFormat4Body(segs []struct{ start, end, delta uint16 }) []byte {
	segCount := len(segs)
	length := 16 + 8*segCount // header 14 + reservedPad 2 + 4 arrays of segCount*2 bytes
	b := make([]byte, length)
	binary.BigEndian.PutUint16(b[0:], 4)
	binary.BigEndian.PutUint16(b[2:], uint16(length))
	binary.BigEndian.PutUint16(b[4:], 0) // language
	binary.BigEndian.PutUint16(b[6:], uint16(2*segCount))
	off := 14
	for _, s := range segs {
		binary.BigEndian.PutUint16(b[off:], s.end)
		off += 2
	}
	off += 2 // reservedPad
	for _, s := range segs {
		binary.BigEndian.PutUint16(b[off:], s.start)
		off += 2
	}
	for _, s := range segs {
		binary.BigEndian.PutUint16(b[off:], s.delta)
		off += 2
	}
	// idRangeOffset array left as zeros.
	return b
}

// buildFormat12Body builds a cmap format 12 subtable body.
func buildFormat12Body(groups []struct{ start, end, startGID uint32 }) []byte {
	return buildFormat12Or13Body(12, groups)
}

// buildFormat13Body builds a cmap format 13 subtable body. The layout is
// identical to format 12 on disk; only the semantics of startGID differ
// (shared by every codepoint in the range rather than incrementing).
func buildFormat13Body(groups []struct{ start, end, startGID uint32 }) []byte {
	return buildFormat12Or13Body(13, groups)
}

func buildFormat12Or13Body(format uint16, groups []struct{ start, end, startGID uint32 }) []byte {
	n := len(groups)
	length := 16 + 12*n
	b := make([]byte, length)
	binary.BigEndian.PutUint16(b[0:], format)
	binary.BigEndian.PutUint16(b[2:], 0) // reserved
	binary.BigEndian.PutUint32(b[4:], uint32(length))
	binary.BigEndian.PutUint32(b[8:], 0) // language
	binary.BigEndian.PutUint32(b[12:], uint32(n))
	off := 16
	for _, g := range groups {
		binary.BigEndian.PutUint32(b[off:], g.start)
		binary.BigEndian.PutUint32(b[off+4:], g.end)
		binary.BigEndian.PutUint32(b[off+8:], g.startGID)
		off += 12
	}
	return b
}

// buildFormat10Body builds a cmap format 10 subtable body.
func buildFormat10Body(firstCode uint32, glyphIDs []uint16) []byte {
	length := 20 + 2*len(glyphIDs)
	b := make([]byte, length)
	binary.BigEndian.PutUint16(b[0:], 10)
	binary.BigEndian.PutUint16(b[2:], 0) // reserved
	binary.BigEndian.PutUint32(b[4:], uint32(length))
	binary.BigEndian.PutUint32(b[8:], 0) // language
	binary.BigEndian.PutUint32(b[12:], firstCode)
	binary.BigEndian.PutUint32(b[16:], uint32(len(glyphIDs)))
	for i, g := range glyphIDs {
		binary.BigEndian.PutUint16(b[20+2*i:], g)
	}
	return b
}

// parseCmapOnly constructs a Font with only the given cmap data and runs parseCmap.
func parseCmapOnly(cmap []byte) (*Font, error) {
	f := &Font{cmap: cmap}
	return f, f.parseCmap()
}

func TestCmapFormat0(t *testing.T) {
	var mapping [256]byte
	mapping['A'] = 1
	mapping['Z'] = 26
	body := buildFormat0Body(mapping)
	f, err := parseCmapOnly(mkCmap(0, 3, body)) // Unicode BMP
	if err != nil {
		t.Fatalf("parseCmap: %v", err)
	}
	if f.cmapFormat != cmapFormat0 {
		t.Errorf("cmapFormat: got %d, want %d", f.cmapFormat, cmapFormat0)
	}
	if got, want := f.Index('A'), Index(1); got != want {
		t.Errorf("Index('A'): got %d, want %d", got, want)
	}
	if got, want := f.Index('Z'), Index(26); got != want {
		t.Errorf("Index('Z'): got %d, want %d", got, want)
	}
	if got := f.Index('Q'); got != 0 {
		t.Errorf("Index('Q') for unmapped byte: got %d, want 0", got)
	}
	if got := f.Index(0x1F600); got != 0 {
		t.Errorf("Index(>255): got %d, want 0", got)
	}
}

func TestCmapFormat6(t *testing.T) {
	body := buildFormat6Body(0x0030, []uint16{100, 101, 102, 103, 104}) // '0'-'4' -> 100-104
	f, err := parseCmapOnly(mkCmap(0, 3, body))
	if err != nil {
		t.Fatalf("parseCmap: %v", err)
	}
	if f.cmapFormat != cmapFormat6 {
		t.Errorf("cmapFormat: got %d, want %d", f.cmapFormat, cmapFormat6)
	}
	for i, r := range []rune{'0', '1', '2', '3', '4'} {
		if got, want := f.Index(r), Index(100+i); got != want {
			t.Errorf("Index(%q): got %d, want %d", r, got, want)
		}
	}
	if got := f.Index('/'); got != 0 {
		t.Errorf("Index below firstCode: got %d, want 0", got)
	}
	if got := f.Index('5'); got != 0 {
		t.Errorf("Index past entryCount: got %d, want 0", got)
	}
}

func TestCmapFormat4(t *testing.T) {
	segs := []struct{ start, end, delta uint16 }{
		{start: 'A', end: 'Z', delta: uint16(-('A' - 1) & 0xffff)}, // 'A' -> 1
		{start: 0xffff, end: 0xffff, delta: 1},                    // required sentinel
	}
	body := buildFormat4Body(segs)
	f, err := parseCmapOnly(mkCmap(0, 3, body))
	if err != nil {
		t.Fatalf("parseCmap: %v", err)
	}
	if f.cmapFormat != cmapFormat4 {
		t.Errorf("cmapFormat: got %d, want %d", f.cmapFormat, cmapFormat4)
	}
	if got, want := f.Index('A'), Index(1); got != want {
		t.Errorf("Index('A'): got %d, want %d", got, want)
	}
	if got, want := f.Index('Z'), Index(26); got != want {
		t.Errorf("Index('Z'): got %d, want %d", got, want)
	}
	if got := f.Index('a'); got != 0 {
		t.Errorf("Index('a') for unmapped codepoint: got %d, want 0", got)
	}
}

func TestCmapFormat12(t *testing.T) {
	body := buildFormat12Body([]struct{ start, end, startGID uint32 }{
		{start: 'A', end: 'Z', startGID: 1},
		{start: 0x1F600, end: 0x1F604, startGID: 200},
	})
	f, err := parseCmapOnly(mkCmap(0, 4, body)) // Unicode Full
	if err != nil {
		t.Fatalf("parseCmap: %v", err)
	}
	if f.cmapFormat != cmapFormat12 {
		t.Errorf("cmapFormat: got %d, want %d", f.cmapFormat, cmapFormat12)
	}
	if got, want := f.Index('A'), Index(1); got != want {
		t.Errorf("Index('A'): got %d, want %d", got, want)
	}
	if got, want := f.Index(0x1F602), Index(202); got != want {
		t.Errorf("Index(emoji): got %d, want %d", got, want)
	}
}

func TestCmapFormat10(t *testing.T) {
	// Map U+1F600..U+1F604 to glyph ids 500..504.
	body := buildFormat10Body(0x1F600, []uint16{500, 501, 502, 503, 504})
	f, err := parseCmapOnly(mkCmap(0, 4, body))
	if err != nil {
		t.Fatalf("parseCmap: %v", err)
	}
	if f.cmapFormat != cmapFormat10 {
		t.Errorf("cmapFormat: got %d, want %d", f.cmapFormat, cmapFormat10)
	}
	for i, r := range []rune{0x1F600, 0x1F601, 0x1F602, 0x1F603, 0x1F604} {
		if got, want := f.Index(r), Index(500+i); got != want {
			t.Errorf("Index(%U): got %d, want %d", r, got, want)
		}
	}
	if got := f.Index(0x1F5FF); got != 0 {
		t.Errorf("Index below firstCode: got %d, want 0", got)
	}
	if got := f.Index(0x1F605); got != 0 {
		t.Errorf("Index past numChars: got %d, want 0", got)
	}
}

func TestCmapFormat13(t *testing.T) {
	// A "Last Resort"-style mapping: every codepoint in a range points to
	// the same placeholder glyph.
	body := buildFormat13Body([]struct{ start, end, startGID uint32 }{
		{start: 'A', end: 'Z', startGID: 7},    // every ASCII letter -> 7
		{start: 0x0400, end: 0x04FF, startGID: 8}, // every Cyrillic glyph -> 8
	})
	f, err := parseCmapOnly(mkCmap(0, 4, body))
	if err != nil {
		t.Fatalf("parseCmap: %v", err)
	}
	if f.cmapFormat != cmapFormat13 {
		t.Errorf("cmapFormat: got %d, want %d", f.cmapFormat, cmapFormat13)
	}
	for _, r := range []rune{'A', 'M', 'Z'} {
		if got, want := f.Index(r), Index(7); got != want {
			t.Errorf("Index(%q): got %d, want %d", r, got, want)
		}
	}
	for _, r := range []rune{0x0400, 0x0450, 0x04FF} {
		if got, want := f.Index(r), Index(8); got != want {
			t.Errorf("Index(%U): got %d, want %d", r, got, want)
		}
	}
	if got := f.Index('a'); got != 0 {
		t.Errorf("Index('a') outside any range: got %d, want 0", got)
	}
}

func TestCmapTruncated(t *testing.T) {
	// Format 4 with a truncated body.
	body := []byte{
		0, 4, // format
		0, 20, // length
		0, 0, // language
		0, 4, // segCountX2 = 4 (segCount = 2), promising 16 more bytes that don't exist
	}
	_, err := parseCmapOnly(mkCmap(0, 3, body))
	if err == nil {
		t.Fatal("expected error on truncated format 4, got nil")
	}
	if _, ok := err.(FormatError); !ok {
		t.Errorf("expected FormatError, got %T: %v", err, err)
	}
}

func TestCmapFormat12Truncated(t *testing.T) {
	body := []byte{
		0, 12, // format
		0, 0, // reserved
		0, 0, 0, 40, // length
		0, 0, 0, 0, // language
		0, 0, 0, 2, // nGroups = 2, promising 24 more bytes that don't exist
	}
	_, err := parseCmapOnly(mkCmap(0, 4, body))
	if err == nil {
		t.Fatal("expected error on truncated format 12, got nil")
	}
	if _, ok := err.(FormatError); !ok {
		t.Errorf("expected FormatError, got %T: %v", err, err)
	}
}

func TestCmapSubtablePriority(t *testing.T) {
	// Provide both a BMP-only subtable (PID 0 PSID 3) and a Full subtable
	// (PID 0 PSID 4). The Full one must be selected, even though BMP comes
	// first in the subtable record list.
	bmpBody := buildFormat4Body([]struct{ start, end, delta uint16 }{
		{start: 'A', end: 'A', delta: uint16(-('A' - 99) & 0xffff)}, // 'A' -> 99
		{start: 0xffff, end: 0xffff, delta: 1},
	})
	fullBody := buildFormat12Body([]struct{ start, end, startGID uint32 }{
		{start: 'A', end: 'A', startGID: 777},
	})
	cmap := mkCmapMulti([]struct {
		pid, psid uint16
		body      []byte
	}{
		{0, 3, bmpBody},  // BMP-only, first
		{0, 4, fullBody}, // Full, second
	})
	f, err := parseCmapOnly(cmap)
	if err != nil {
		t.Fatalf("parseCmap: %v", err)
	}
	if got := f.Index('A'); got != 777 {
		t.Errorf("Index('A'): got %d, want 777 (Full should win over BMP)", got)
	}
}

func TestCmapSubtableMicrosoftUCS4Priority(t *testing.T) {
	// Microsoft UCS-4 (PID 3 PSID 10) should win over Unicode BMP-only (PID 0 PSID 3).
	bmpBody := buildFormat4Body([]struct{ start, end, delta uint16 }{
		{start: 'A', end: 'A', delta: uint16(-('A' - 55) & 0xffff)},
		{start: 0xffff, end: 0xffff, delta: 1},
	})
	ucs4Body := buildFormat12Body([]struct{ start, end, startGID uint32 }{
		{start: 'A', end: 'A', startGID: 321},
	})
	cmap := mkCmapMulti([]struct {
		pid, psid uint16
		body      []byte
	}{
		{0, 3, bmpBody},
		{3, 10, ucs4Body},
	})
	f, err := parseCmapOnly(cmap)
	if err != nil {
		t.Fatalf("parseCmap: %v", err)
	}
	if got := f.Index('A'); got != 321 {
		t.Errorf("Index('A'): got %d, want 321 (UCS-4 should win over BMP)", got)
	}
}

func TestCmapSubtableUnicodeFullBeatsUCS4(t *testing.T) {
	// Unicode Full (priority 5) should win over Microsoft UCS-4 (priority 4).
	fullBody := buildFormat12Body([]struct{ start, end, startGID uint32 }{
		{start: 'A', end: 'A', startGID: 111},
	})
	ucs4Body := buildFormat12Body([]struct{ start, end, startGID uint32 }{
		{start: 'A', end: 'A', startGID: 222},
	})
	cmap := mkCmapMulti([]struct {
		pid, psid uint16
		body      []byte
	}{
		{3, 10, ucs4Body}, // listed first
		{0, 4, fullBody},  // listed second, higher priority
	})
	f, err := parseCmapOnly(cmap)
	if err != nil {
		t.Fatalf("parseCmap: %v", err)
	}
	if got := f.Index('A'); got != 111 {
		t.Errorf("Index('A'): got %d, want 111 (Full should beat UCS-4)", got)
	}
}

func TestCmapUnsupportedFormat(t *testing.T) {
	// Format 14 (variation selectors) — not yet supported, must return
	// UnsupportedError, not panic.
	body := []byte{
		0, 14, // format
		0, 0, 0, 0, // length (minimal)
	}
	_, err := parseCmapOnly(mkCmap(0, 5, body))
	if err == nil {
		t.Fatal("expected error on format 14, got nil")
	}
	if _, ok := err.(UnsupportedError); !ok {
		// Unicode Variation Selectors PSID 5 is not a selected encoding anyway,
		// so parseSubtables may reject it first.
		if !strings.Contains(err.Error(), "encoding") {
			t.Errorf("expected UnsupportedError, got %T: %v", err, err)
		}
	}
}
