// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

// Package layout parses the shared OpenType Layout infrastructure used by
// both GSUB (glyph substitution) and GPOS (glyph positioning): the
// Script/Feature/Lookup lists, Coverage tables, and ClassDef tables.
//
// Higher-level packages (gsub, gpos) consume the Lookup subtable bytes this
// package exposes and dispatch on the per-feature lookup type.
package layout

import (
	"fmt"
)

// A Tag is a 4-byte ASCII OpenType tag (e.g. "latn", "arab", "liga").
// When specifying "no language" OpenType uses "DFLT" for scripts and
// "dflt" for default-language fallback; both are representable here.
type Tag uint32

// MakeTag builds a Tag from a 4-character ASCII string. Extra characters
// are truncated; short strings are right-padded with spaces (0x20).
func MakeTag(s string) Tag {
	var b [4]byte
	for i := 0; i < 4; i++ {
		if i < len(s) {
			b[i] = s[i]
		} else {
			b[i] = ' '
		}
	}
	return Tag(uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]))
}

// String renders a Tag as a 4-character ASCII string, trimmed of trailing
// spaces.
func (t Tag) String() string {
	b := [4]byte{byte(t >> 24), byte(t >> 16), byte(t >> 8), byte(t)}
	// Trim trailing spaces.
	n := 4
	for n > 0 && b[n-1] == ' ' {
		n--
	}
	return string(b[:n])
}

// FormatError reports a malformed OT Layout structure.
type FormatError string

func (e FormatError) Error() string { return "layout: invalid: " + string(e) }

// UnsupportedError reports an OT Layout feature we do not implement.
type UnsupportedError string

func (e UnsupportedError) Error() string { return "layout: unsupported: " + string(e) }

// Table represents either a GSUB or GPOS table. Both share the same shape.
type Table struct {
	// Data is the raw table bytes; subslices below index into it.
	Data []byte

	// Version major/minor.
	MajorVersion, MinorVersion uint16

	Scripts  []Script
	Features []Feature
	Lookups  []Lookup

	// FeatureVariationsOffset is the absolute offset of the FeatureVariations
	// table introduced in version 1.1 (used by variable fonts). Zero when
	// absent.
	FeatureVariationsOffset int
}

// A Script is a parsed Script table record.
type Script struct {
	Tag           Tag
	DefaultLang   *LangSys // may be nil
	Languages     []Language
}

// A Language is a parsed LangSys table record.
type Language struct {
	Tag     Tag
	LangSys LangSys
}

// LangSys enumerates the feature indices enabled for a particular language.
type LangSys struct {
	// RequiredFeatureIndex is 0xffff when no feature is required.
	RequiredFeatureIndex uint16
	FeatureIndexes       []uint16
}

// A Feature describes one named feature (e.g. "liga") and the Lookup indices
// it applies.
type Feature struct {
	Tag           Tag
	LookupIndices []uint16
}

// A Lookup is a parsed Lookup table. SubtableData is the raw bytes of each
// subtable, indexed from the Lookup's start; the caller interprets them
// according to the Lookup's Type.
type Lookup struct {
	Type             uint16
	Flag             uint16
	MarkFilteringSet uint16 // valid iff Flag & 0x0010 != 0
	SubtableData     [][]byte
}

// Parse decodes a GSUB or GPOS table header plus its ScriptList, FeatureList,
// and LookupList. Lookup subtables are sliced but not dispatched on; that is
// the job of the consumer packages.
func Parse(data []byte) (*Table, error) {
	if len(data) < 10 {
		return nil, FormatError("table too short")
	}
	major := u16(data, 0)
	minor := u16(data, 2)
	if major != 1 {
		return nil, UnsupportedError(fmt.Sprintf("major version %d", major))
	}
	scriptOff := int(u16(data, 4))
	featureOff := int(u16(data, 6))
	lookupOff := int(u16(data, 8))
	featureVarOff := 0
	if minor == 1 {
		if len(data) < 14 {
			return nil, FormatError("table too short for v1.1")
		}
		featureVarOff = int(u32(data, 10))
	}

	t := &Table{
		Data:                    data,
		MajorVersion:            major,
		MinorVersion:            minor,
		FeatureVariationsOffset: featureVarOff,
	}

	var err error
	if t.Scripts, err = parseScriptList(data, scriptOff); err != nil {
		return nil, fmt.Errorf("ScriptList: %w", err)
	}
	if t.Features, err = parseFeatureList(data, featureOff); err != nil {
		return nil, fmt.Errorf("FeatureList: %w", err)
	}
	if t.Lookups, err = parseLookupList(data, lookupOff); err != nil {
		return nil, fmt.Errorf("LookupList: %w", err)
	}
	return t, nil
}

// FindScript returns a pointer to the Script matching tag, or nil if
// absent. Use "DFLT" as a fallback tag.
func (t *Table) FindScript(tag Tag) *Script {
	for i := range t.Scripts {
		if t.Scripts[i].Tag == tag {
			return &t.Scripts[i]
		}
	}
	return nil
}

// FindLanguage returns the LangSys for the (script, lang) pair, falling back
// to the script's DefaultLang (and then to the DFLT script's default). If
// nothing matches, returns nil.
func (t *Table) FindLanguage(script Tag, lang Tag) *LangSys {
	if s := t.FindScript(script); s != nil {
		for i := range s.Languages {
			if s.Languages[i].Tag == lang {
				return &s.Languages[i].LangSys
			}
		}
		if s.DefaultLang != nil {
			return s.DefaultLang
		}
	}
	if s := t.FindScript(MakeTag("DFLT")); s != nil && s.DefaultLang != nil {
		return s.DefaultLang
	}
	return nil
}

// FindFeature finds a feature by index in the FeatureList. Returns nil when
// the index is out of range.
func (t *Table) FindFeature(index uint16) *Feature {
	if int(index) >= len(t.Features) {
		return nil
	}
	return &t.Features[index]
}

func parseScriptList(data []byte, off int) ([]Script, error) {
	if off+2 > len(data) {
		return nil, FormatError("ScriptList header truncated")
	}
	n := int(u16(data, off))
	recOff := off + 2
	if recOff+6*n > len(data) {
		return nil, FormatError("ScriptList records truncated")
	}
	scripts := make([]Script, n)
	for i := 0; i < n; i++ {
		tag := Tag(u32(data, recOff+6*i))
		so := off + int(u16(data, recOff+6*i+4))
		s, err := parseScript(data, so)
		if err != nil {
			return nil, fmt.Errorf("Script %s: %w", tag, err)
		}
		s.Tag = tag
		scripts[i] = s
	}
	return scripts, nil
}

func parseScript(data []byte, off int) (Script, error) {
	if off+4 > len(data) {
		return Script{}, FormatError("Script header truncated")
	}
	defLangOff := int(u16(data, off))
	nLang := int(u16(data, off+2))
	recOff := off + 4
	if recOff+6*nLang > len(data) {
		return Script{}, FormatError("Script lang records truncated")
	}
	var s Script
	if defLangOff != 0 {
		ls, err := parseLangSys(data, off+defLangOff)
		if err != nil {
			return Script{}, fmt.Errorf("DefaultLangSys: %w", err)
		}
		s.DefaultLang = &ls
	}
	s.Languages = make([]Language, nLang)
	for i := 0; i < nLang; i++ {
		tag := Tag(u32(data, recOff+6*i))
		lo := off + int(u16(data, recOff+6*i+4))
		ls, err := parseLangSys(data, lo)
		if err != nil {
			return Script{}, fmt.Errorf("LangSys %s: %w", tag, err)
		}
		s.Languages[i] = Language{Tag: tag, LangSys: ls}
	}
	return s, nil
}

func parseLangSys(data []byte, off int) (LangSys, error) {
	if off+6 > len(data) {
		return LangSys{}, FormatError("LangSys header truncated")
	}
	// uint16 lookupOrderOffset (reserved, 0)
	required := u16(data, off+2)
	n := int(u16(data, off+4))
	if off+6+2*n > len(data) {
		return LangSys{}, FormatError("LangSys feature indices truncated")
	}
	indexes := make([]uint16, n)
	for i := 0; i < n; i++ {
		indexes[i] = u16(data, off+6+2*i)
	}
	return LangSys{RequiredFeatureIndex: required, FeatureIndexes: indexes}, nil
}

func parseFeatureList(data []byte, off int) ([]Feature, error) {
	if off+2 > len(data) {
		return nil, FormatError("FeatureList header truncated")
	}
	n := int(u16(data, off))
	recOff := off + 2
	if recOff+6*n > len(data) {
		return nil, FormatError("FeatureList records truncated")
	}
	features := make([]Feature, n)
	for i := 0; i < n; i++ {
		tag := Tag(u32(data, recOff+6*i))
		fo := off + int(u16(data, recOff+6*i+4))
		if fo+4 > len(data) {
			return nil, FormatError("Feature header truncated")
		}
		nLookup := int(u16(data, fo+2))
		if fo+4+2*nLookup > len(data) {
			return nil, FormatError("Feature lookup indices truncated")
		}
		lookups := make([]uint16, nLookup)
		for j := 0; j < nLookup; j++ {
			lookups[j] = u16(data, fo+4+2*j)
		}
		features[i] = Feature{Tag: tag, LookupIndices: lookups}
	}
	return features, nil
}

func parseLookupList(data []byte, off int) ([]Lookup, error) {
	if off+2 > len(data) {
		return nil, FormatError("LookupList header truncated")
	}
	n := int(u16(data, off))
	if off+2+2*n > len(data) {
		return nil, FormatError("LookupList offsets truncated")
	}
	lookups := make([]Lookup, n)
	for i := 0; i < n; i++ {
		lo := off + int(u16(data, off+2+2*i))
		l, err := parseLookup(data, lo)
		if err != nil {
			return nil, fmt.Errorf("Lookup #%d: %w", i, err)
		}
		lookups[i] = l
	}
	return lookups, nil
}

func parseLookup(data []byte, off int) (Lookup, error) {
	if off+6 > len(data) {
		return Lookup{}, FormatError("Lookup header truncated")
	}
	typ := u16(data, off)
	flag := u16(data, off+2)
	nSub := int(u16(data, off+4))
	if off+6+2*nSub > len(data) {
		return Lookup{}, FormatError("Lookup subtable offsets truncated")
	}
	subtables := make([][]byte, nSub)
	for i := 0; i < nSub; i++ {
		so := off + int(u16(data, off+6+2*i))
		if so >= len(data) {
			return Lookup{}, FormatError("Lookup subtable offset out of bounds")
		}
		subtables[i] = data[so:]
	}
	l := Lookup{Type: typ, Flag: flag, SubtableData: subtables}
	if flag&0x0010 != 0 {
		// UseMarkFilteringSet — a trailing uint16 mark filter set index.
		mfsOff := off + 6 + 2*nSub
		if mfsOff+2 > len(data) {
			return Lookup{}, FormatError("Lookup MarkFilteringSet truncated")
		}
		l.MarkFilteringSet = u16(data, mfsOff)
	}
	return l, nil
}

// u16 / u32 reading helpers (big-endian).

func u16(b []byte, i int) uint16 {
	return uint16(b[i])<<8 | uint16(b[i+1])
}

func u32(b []byte, i int) uint32 {
	return uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3])
}
