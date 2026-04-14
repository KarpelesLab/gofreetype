// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

// Package bdf parses the Adobe Bitmap Distribution Format — the plain-
// text glyph-per-character format used for X11 terminal fonts and a
// handful of retro display use cases. A BDF file is essentially a
// sequence of STARTCHAR / ENDCHAR blocks each carrying metrics, the
// glyph's bounding box, and its bitmap as hex-encoded rows.
//
// The spec is in Adobe Technical Note #5005.
package bdf

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"strconv"
	"strings"

	"github.com/KarpelesLab/gofreetype/raster"
)

// FormatError reports malformed BDF input.
type FormatError string

func (e FormatError) Error() string { return "bdf: invalid: " + string(e) }

// Font is a parsed BDF font.
type Font struct {
	Name          string
	PointSize     int
	XRes, YRes    int // display resolution the font was designed for
	PixelSize     int // vertical em in pixels
	FontAscent    int
	FontDescent   int
	BoundingBoxX  int // default FONTBOUNDINGBOX width
	BoundingBoxY  int
	BoundingBoxOx int // default FONTBOUNDINGBOX x offset
	BoundingBoxOy int
	Glyphs        []Glyph

	// runeToIndex maps a codepoint (ENCODING line) to a Glyphs index.
	runeToIndex map[rune]int
}

// Glyph is a single BDF glyph.
type Glyph struct {
	Name     string
	Encoding rune // -1 if the glyph is unencoded
	Advance  int  // DWIDTH (horizontal advance, in pixels)
	BBX      int
	BBY      int
	BBOx     int
	BBOy     int
	Bitmap   *raster.Bitmap
}

// Parse decodes a BDF file.
func Parse(data []byte) (*Font, error) {
	f := &Font{runeToIndex: make(map[rune]int)}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 1<<16), 1<<20)

	var cur *Glyph
	inBitmap := false
	var bitmapRows []string
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n\t ")
		if line == "" {
			continue
		}
		if inBitmap {
			if line == "ENDCHAR" {
				inBitmap = false
				if err := finalizeGlyph(cur, bitmapRows); err != nil {
					return nil, err
				}
				f.Glyphs = append(f.Glyphs, *cur)
				if cur.Encoding >= 0 {
					f.runeToIndex[cur.Encoding] = len(f.Glyphs) - 1
				}
				cur = nil
				bitmapRows = nil
				continue
			}
			bitmapRows = append(bitmapRows, line)
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "FONT":
			if len(fields) >= 2 {
				f.Name = strings.Join(fields[1:], " ")
			}
		case "SIZE":
			if len(fields) >= 4 {
				f.PointSize, _ = strconv.Atoi(fields[1])
				f.XRes, _ = strconv.Atoi(fields[2])
				f.YRes, _ = strconv.Atoi(fields[3])
			}
		case "PIXEL_SIZE":
			if len(fields) >= 2 {
				f.PixelSize, _ = strconv.Atoi(fields[1])
			}
		case "FONT_ASCENT":
			if len(fields) >= 2 {
				f.FontAscent, _ = strconv.Atoi(fields[1])
			}
		case "FONT_DESCENT":
			if len(fields) >= 2 {
				f.FontDescent, _ = strconv.Atoi(fields[1])
			}
		case "FONTBOUNDINGBOX":
			if len(fields) >= 5 {
				f.BoundingBoxX, _ = strconv.Atoi(fields[1])
				f.BoundingBoxY, _ = strconv.Atoi(fields[2])
				f.BoundingBoxOx, _ = strconv.Atoi(fields[3])
				f.BoundingBoxOy, _ = strconv.Atoi(fields[4])
			}
		case "STARTCHAR":
			cur = &Glyph{Encoding: -1}
			if len(fields) >= 2 {
				cur.Name = fields[1]
			}
		case "ENCODING":
			if cur != nil && len(fields) >= 2 {
				v, err := strconv.Atoi(fields[1])
				if err == nil && v >= 0 {
					cur.Encoding = rune(v)
				}
			}
		case "DWIDTH":
			if cur != nil && len(fields) >= 2 {
				cur.Advance, _ = strconv.Atoi(fields[1])
			}
		case "BBX":
			if cur != nil && len(fields) >= 5 {
				cur.BBX, _ = strconv.Atoi(fields[1])
				cur.BBY, _ = strconv.Atoi(fields[2])
				cur.BBOx, _ = strconv.Atoi(fields[3])
				cur.BBOy, _ = strconv.Atoi(fields[4])
			}
		case "BITMAP":
			if cur == nil {
				return nil, FormatError("BITMAP without preceding STARTCHAR")
			}
			inBitmap = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return f, nil
}

// finalizeGlyph decodes the hex-encoded bitmap rows into a raster.Bitmap.
// Each row is a hex string with enough hex digits to cover the glyph's
// width (rounded up to the nearest byte). Leftmost pixel = high bit of
// the first byte.
func finalizeGlyph(g *Glyph, rows []string) error {
	if g.BBX <= 0 || g.BBY <= 0 {
		return nil
	}
	if len(rows) != g.BBY {
		// Some BDF files trim trailing empty rows; tolerate that.
		if len(rows) > g.BBY {
			return FormatError(fmt.Sprintf("glyph %q: %d bitmap rows but BBY=%d", g.Name, len(rows), g.BBY))
		}
	}
	bytesPerRow := (g.BBX + 7) / 8
	bm := raster.NewBitmap(image.Rect(0, 0, g.BBX, g.BBY))
	for y, row := range rows {
		if y >= g.BBY {
			break
		}
		// Decode hex. Rows are expected to be exactly 2*bytesPerRow hex digits.
		// Be lenient with padding: accept short rows as zero-padded.
		if len(row) > 2*bytesPerRow {
			row = row[:2*bytesPerRow]
		}
		for i := 0; i < len(row); i += 2 {
			hi := hexNibble(row[i])
			var lo byte
			if i+1 < len(row) {
				lo = hexNibble(row[i+1])
			}
			b := (hi << 4) | lo
			byteIdx := i / 2
			for bit := 0; bit < 8; bit++ {
				x := byteIdx*8 + bit
				if x >= g.BBX {
					break
				}
				if b&(1<<(7-uint(bit))) != 0 {
					bm.SetBit(x, y, true)
				}
			}
		}
	}
	g.Bitmap = bm
	return nil
}

func hexNibble(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return 10 + c - 'a'
	case c >= 'A' && c <= 'F':
		return 10 + c - 'A'
	}
	return 0
}

// Glyph returns the glyph for rune r, or nil if absent.
func (f *Font) Glyph(r rune) *Glyph {
	if f == nil {
		return nil
	}
	idx, ok := f.runeToIndex[r]
	if !ok {
		return nil
	}
	return &f.Glyphs[idx]
}
