// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package type1

import (
	"bytes"
	"strconv"
	"strings"
)

// Font is a parsed Type 1 font.
type Font struct {
	// FontName is the PostScript name from the font's /FontName entry.
	FontName string

	// FontMatrix is the transform from design coords to 1000-unit em
	// space. Default (0.001, 0, 0, 0.001, 0, 0).
	FontMatrix [6]float64

	// FontBBox is the tight bounding box [xMin, yMin, xMax, yMax] in
	// design units.
	FontBBox [4]float64

	// CharStrings maps glyph name -> decrypted, lenIV-stripped Type 1
	// charstring body.
	CharStrings map[string][]byte

	// Subrs is the /Subrs array, each entry decrypted and lenIV-stripped.
	Subrs [][]byte

	// lenIV is the number of random lead bytes prefixed to each
	// encrypted charstring (default 4).
	lenIV int
}

// Parse decodes a PFA (ASCII) or PFB (segmented) Type 1 font.
func Parse(data []byte) (*Font, error) {
	sec, err := readContainer(data)
	if err != nil {
		return nil, err
	}
	body := decryptEexec(sec.body)
	if body == nil {
		return nil, FormatError("eexec decryption failed")
	}

	f := &Font{
		FontMatrix:  [6]float64{0.001, 0, 0, 0.001, 0, 0},
		CharStrings: make(map[string][]byte),
		lenIV:       4,
	}
	parseHeaderDict(sec.header, f)
	if err := parsePrivateDict(body, f); err != nil {
		return nil, err
	}
	return f, nil
}

// readContainer dispatches on the PFB magic byte. PFA files are already
// ASCII text and can be scanned directly for the eexec block.
func readContainer(data []byte) (*sections, error) {
	if IsPFB(data) {
		return splitPFB(data)
	}
	// PFA: split at "currentfile eexec\n" and unhex the body before the
	// "cleartomark" trailer. PFA support is not common today; we return
	// unsupported rather than a sloppy parser.
	if bytes.Contains(data, []byte("currentfile eexec")) {
		return nil, UnsupportedError("PFA (ASCII) Type 1 fonts")
	}
	return nil, FormatError("not a recognized Type 1 container")
}

// parseHeaderDict pulls the font name, FontMatrix, and FontBBox out of
// the plain-text header (which is valid PostScript).
func parseHeaderDict(header []byte, f *Font) {
	s := string(header)
	if v := extractLineValue(s, "/FontName"); v != "" {
		// v looks like "/Foo def" — strip leading / and trailing " def".
		v = strings.TrimPrefix(v, "/")
		if i := strings.Index(v, " "); i >= 0 {
			v = v[:i]
		}
		f.FontName = v
	}
	if arr := extractArrayValue(s, "/FontMatrix"); len(arr) == 6 {
		copy(f.FontMatrix[:], arr)
	}
	if arr := extractArrayValue(s, "/FontBBox"); len(arr) == 4 {
		copy(f.FontBBox[:], arr)
	}
}

// extractLineValue finds "key <value> def" in s and returns the <value>
// string, minus the trailing " def".
func extractLineValue(s, key string) string {
	i := strings.Index(s, key)
	if i < 0 {
		return ""
	}
	rest := s[i+len(key):]
	rest = strings.TrimLeft(rest, " \t")
	end := strings.IndexAny(rest, "\r\n")
	if end < 0 {
		end = len(rest)
	}
	line := rest[:end]
	line = strings.TrimSuffix(strings.TrimSpace(line), "def")
	return strings.TrimSpace(line)
}

// extractArrayValue finds "key [a b c d] def" and returns the numeric
// values.
func extractArrayValue(s, key string) []float64 {
	i := strings.Index(s, key)
	if i < 0 {
		return nil
	}
	rest := s[i+len(key):]
	open := strings.Index(rest, "[")
	close := strings.Index(rest, "]")
	if open < 0 || close < 0 || close <= open {
		return nil
	}
	fields := strings.Fields(rest[open+1 : close])
	out := make([]float64, 0, len(fields))
	for _, t := range fields {
		v, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

// parsePrivateDict walks the decrypted Private dict, extracting /lenIV,
// /Subrs, and /CharStrings.
func parsePrivateDict(body []byte, f *Font) error {
	// lenIV: "/lenIV <int> def". Default is 4; we've already set it.
	if v := findLenIV(body); v >= 0 {
		f.lenIV = v
	}

	// Subrs array. The entries look like:
	//   dup <index> <length> -| <length bytes> |-
	// where "-|" (or "RD") is a macro that pushes the next <length>
	// bytes onto the stack. The -| marker is followed by exactly one
	// space before the binary body.
	f.Subrs = extractIndexedEntries(body, []byte("/Subrs"), f.lenIV)

	// CharStrings dict:
	//   dup /<name> <length> -| <length bytes> |-
	f.CharStrings = extractNamedEntries(body, []byte("/CharStrings"), f.lenIV)

	if len(f.CharStrings) == 0 {
		return FormatError("no CharStrings found")
	}
	return nil
}

// findLenIV returns the integer value of /lenIV in body, or -1 if absent.
func findLenIV(body []byte) int {
	needle := []byte("/lenIV")
	i := bytes.Index(body, needle)
	if i < 0 {
		return -1
	}
	rest := body[i+len(needle):]
	// Skip whitespace.
	j := 0
	for j < len(rest) && (rest[j] == ' ' || rest[j] == '\t') {
		j++
	}
	// Collect digits.
	start := j
	for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
		j++
	}
	if j == start {
		return -1
	}
	v, err := strconv.Atoi(string(rest[start:j]))
	if err != nil {
		return -1
	}
	return v
}

// extractIndexedEntries scans for "dup <index> <length> -|<space><body>"
// patterns in body, returning the decrypted, lenIV-stripped bodies.
func extractIndexedEntries(body, marker []byte, lenIV int) [][]byte {
	scope := sliceAfter(body, marker)
	if scope == nil {
		return nil
	}
	var out [][]byte
	i := 0
	for i < len(scope) {
		dup := bytes.Index(scope[i:], []byte("dup "))
		if dup < 0 {
			break
		}
		i += dup + 4
		// Read: <index> <length>
		idx, n, ok := readUint(scope, &i)
		_ = idx
		if !ok {
			continue
		}
		length, m, ok := readUint(scope, &i)
		_ = m
		_ = n
		if !ok {
			continue
		}
		// Skip "RD" / "-|" / custom marker + single whitespace.
		mstart := i
		for mstart < len(scope) && (scope[mstart] == ' ' || scope[mstart] == '\t') {
			mstart++
		}
		// Token ends at whitespace.
		tokEnd := mstart
		for tokEnd < len(scope) && !isWhite(scope[tokEnd]) {
			tokEnd++
		}
		if tokEnd >= len(scope) {
			break
		}
		// Single whitespace byte then length body bytes.
		bodyStart := tokEnd + 1
		if bodyStart+length > len(scope) {
			break
		}
		enc := scope[bodyStart : bodyStart+length]
		dec := decryptCharstring(enc, lenIV)
		// Grow out slice as needed.
		if int(idx) >= len(out) {
			grow := int(idx) - len(out) + 1
			out = append(out, make([][]byte, grow)...)
		}
		out[int(idx)] = dec
		i = bodyStart + length
	}
	return out
}

// extractNamedEntries scans for "/<name> <length> -|<space><body>"
// patterns, returning a name-indexed map.
func extractNamedEntries(body, marker []byte, lenIV int) map[string][]byte {
	out := make(map[string][]byte)
	scope := sliceAfter(body, marker)
	if scope == nil {
		return out
	}
	i := 0
	for i < len(scope) {
		slash := bytes.IndexByte(scope[i:], '/')
		if slash < 0 {
			break
		}
		i += slash + 1
		// Read name: up to whitespace.
		nameStart := i
		for i < len(scope) && !isWhite(scope[i]) {
			i++
		}
		name := string(scope[nameStart:i])
		// Skip whitespace.
		for i < len(scope) && isWhite(scope[i]) {
			i++
		}
		length, _, ok := readUint(scope, &i)
		if !ok {
			continue
		}
		// Skip RD/-| token.
		for i < len(scope) && isWhite(scope[i]) {
			i++
		}
		for i < len(scope) && !isWhite(scope[i]) {
			i++
		}
		if i >= len(scope) {
			break
		}
		bodyStart := i + 1
		if bodyStart+length > len(scope) {
			break
		}
		enc := scope[bodyStart : bodyStart+length]
		out[name] = decryptCharstring(enc, lenIV)
		i = bodyStart + length
	}
	return out
}

// sliceAfter finds marker in data and returns the slice just past it,
// or nil if absent.
func sliceAfter(data, marker []byte) []byte {
	i := bytes.Index(data, marker)
	if i < 0 {
		return nil
	}
	return data[i+len(marker):]
}

// readUint reads an ASCII unsigned integer from data[*i:], skipping
// leading whitespace.
func readUint(data []byte, i *int) (val, consumed int, ok bool) {
	for *i < len(data) && isWhite(data[*i]) {
		*i++
	}
	start := *i
	for *i < len(data) && data[*i] >= '0' && data[*i] <= '9' {
		*i++
	}
	if *i == start {
		return 0, 0, false
	}
	v, err := strconv.Atoi(string(data[start:*i]))
	if err != nil {
		return 0, 0, false
	}
	return v, *i - start, true
}

func isWhite(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

// LoadGlyph decodes the Type 1 charstring for the named glyph and
// returns the resulting glyph. Returns nil, false if the name is not
// present in /CharStrings.
func (f *Font) LoadGlyph(name string) (*Glyph, error) {
	cs, ok := f.CharStrings[name]
	if !ok {
		return nil, FormatError("glyph not found: " + name)
	}
	return Decode(cs, f.Subrs)
}
