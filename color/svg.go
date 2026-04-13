// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package color

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
)

// SVG is a parsed SVG (SVG glyph) table. Each document spans a range of
// glyph ids; we store the decoded ranges so Document(gid) is a simple
// search.
type SVG struct {
	Records []SVGRecord
}

// SVGRecord represents one SVG document covering [StartGlyphID, EndGlyphID].
// The rendering library on top of gofreetype decides how to rasterize the
// SVG (crosslinking to gioui.org/svg, github.com/srwiley/oksvg, etc.) —
// the color package only extracts the raw bytes.
type SVGRecord struct {
	StartGlyphID uint16
	EndGlyphID   uint16
	Document     []byte // decompressed if the source was gzipped
}

// ParseSVG decodes an SVG table.
//
// Header:
//
//	uint16 version (0)
//	Offset32 svgDocumentListOffset
//	uint32 reserved (0)
//
// SVGDocumentList at that offset:
//
//	uint16 numEntries
//	SVGDocumentRecord records[numEntries]
//
// SVGDocumentRecord (12 bytes):
//
//	uint16 startGlyphID
//	uint16 endGlyphID
//	Offset32 svgDocOffset    (from document list start)
//	uint32 svgDocLength
//
// Documents may be gzipped (detected by the 0x1f 0x8b magic); we
// transparently decompress them so callers always see uncompressed SVG.
func ParseSVG(data []byte) (*SVG, error) {
	if len(data) < 10 {
		return nil, FormatError("SVG table too short")
	}
	version := u16(data, 0)
	if version != 0 {
		return nil, FormatError(fmt.Sprintf("SVG version %d (expected 0)", version))
	}
	listOff := int(u32(data, 2))
	if listOff+2 > len(data) {
		return nil, FormatError("SVG document list out of bounds")
	}
	n := int(u16(data, listOff))
	if listOff+2+12*n > len(data) {
		return nil, FormatError("SVG document records truncated")
	}

	out := &SVG{Records: make([]SVGRecord, 0, n)}
	for i := 0; i < n; i++ {
		rec := listOff + 2 + 12*i
		start := u16(data, rec)
		end := u16(data, rec+2)
		docOff := listOff + int(u32(data, rec+4))
		docLen := int(u32(data, rec+8))
		if docOff+docLen > len(data) {
			continue
		}
		doc := data[docOff : docOff+docLen]
		if len(doc) >= 2 && doc[0] == 0x1f && doc[1] == 0x8b {
			if decompressed, err := gunzip(doc); err == nil {
				doc = decompressed
			} else {
				continue
			}
		}
		out.Records = append(out.Records, SVGRecord{
			StartGlyphID: start,
			EndGlyphID:   end,
			Document:     doc,
		})
	}
	return out, nil
}

// Document returns the SVG document covering glyph id gid, or nil if
// gid has no SVG representation.
func (s *SVG) Document(gid uint16) []byte {
	if s == nil {
		return nil
	}
	for _, r := range s.Records {
		if gid >= r.StartGlyphID && gid <= r.EndGlyphID {
			return r.Document
		}
	}
	return nil
}

func gunzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
