// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package color

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"testing"
)

// buildSVG builds a synthetic SVG table with the given records. If gzip
// is true, the document is stored gzipped (the parser must decompress it).
func buildSVG(records []struct {
	startGID, endGID uint16
	doc              []byte
	gzipIt           bool
}) []byte {
	// Layout: header (10) + doc list header (2) + records (12 each) + doc blobs.
	headerLen := 10
	docListOff := headerLen
	listHeaderLen := 2 + 12*len(records)

	// Optionally gzip each doc.
	docs := make([][]byte, len(records))
	for i, r := range records {
		if r.gzipIt {
			var b bytes.Buffer
			w := gzip.NewWriter(&b)
			w.Write(r.doc)
			w.Close()
			docs[i] = b.Bytes()
		} else {
			docs[i] = r.doc
		}
	}

	// Compute doc offsets (relative to docListOff).
	docOffs := make([]uint32, len(records))
	cursor := uint32(listHeaderLen)
	for i, d := range docs {
		docOffs[i] = cursor
		cursor += uint32(len(d))
	}

	out := make([]byte, headerLen+int(cursor))
	binary.BigEndian.PutUint16(out[0:], 0) // version
	binary.BigEndian.PutUint32(out[2:], uint32(docListOff))
	// reserved at offset 6: zero.

	binary.BigEndian.PutUint16(out[docListOff:], uint16(len(records)))
	for i, r := range records {
		rec := docListOff + 2 + 12*i
		binary.BigEndian.PutUint16(out[rec:], r.startGID)
		binary.BigEndian.PutUint16(out[rec+2:], r.endGID)
		binary.BigEndian.PutUint32(out[rec+4:], docOffs[i])
		binary.BigEndian.PutUint32(out[rec+8:], uint32(len(docs[i])))
	}
	for i, d := range docs {
		copy(out[docListOff+int(docOffs[i]):], d)
	}
	return out
}

func TestParseSVG(t *testing.T) {
	svgPlain := []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)
	svgGzipped := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`)
	data := buildSVG([]struct {
		startGID, endGID uint16
		doc              []byte
		gzipIt           bool
	}{
		{startGID: 10, endGID: 12, doc: svgPlain, gzipIt: false},
		{startGID: 20, endGID: 20, doc: svgGzipped, gzipIt: true},
	})
	tbl, err := ParseSVG(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl.Records) != 2 {
		t.Fatalf("Records: got %d, want 2", len(tbl.Records))
	}
	// Plain document stored verbatim.
	if got := tbl.Document(11); string(got) != string(svgPlain) {
		t.Errorf("Document(11): got %q, want %q", got, svgPlain)
	}
	// Gzipped document transparently decompressed.
	if got := tbl.Document(20); string(got) != string(svgGzipped) {
		t.Errorf("Document(20) decompressed: got %q, want %q", got, svgGzipped)
	}
	// Out-of-range glyph.
	if got := tbl.Document(99); got != nil {
		t.Errorf("Document(99) for absent gid: got %q, want nil", got)
	}
}
