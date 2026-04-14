// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import "unicode/utf16"

// NameRecord is one (platformID, encodingID, languageID, nameID) entry
// from the font's name table along with the decoded string.
type NameRecord struct {
	PlatformID uint16
	EncodingID uint16
	LanguageID uint16
	NameID     NameID
	Value      string
}

// Names decodes and returns all records in the font's name table. When
// a record's text is stored as UTF-16BE (the common Windows / Unicode
// encoding), it is decoded to UTF-8 here; Mac Roman (platform 1,
// encoding 0) is decoded as Latin-1. Other encodings are returned as
// raw bytes reinterpreted as Latin-1, which loses accuracy for Chinese
// / Japanese / Korean legacy encodings but avoids a large encoding
// table dependency.
func (f *Font) Names() []NameRecord {
	if len(f.name) < 6 {
		return nil
	}
	n := int(u16(f.name, 2))
	stringsOff := int(u16(f.name, 4))
	if 6+12*n > len(f.name) {
		return nil
	}
	out := make([]NameRecord, 0, n)
	for i := 0; i < n; i++ {
		rec := 6 + 12*i
		length := int(u16(f.name, rec+8))
		off := stringsOff + int(u16(f.name, rec+10))
		if off+length > len(f.name) {
			continue
		}
		r := NameRecord{
			PlatformID: u16(f.name, rec),
			EncodingID: u16(f.name, rec+2),
			LanguageID: u16(f.name, rec+4),
			NameID:     NameID(u16(f.name, rec+6)),
		}
		r.Value = decodeNameString(f.name[off:off+length], r.PlatformID, r.EncodingID)
		out = append(out, r)
	}
	return out
}

// NameByLanguage returns the value of the named record preferring the
// given Windows-style language ID (0x0409 is en-US). If no record
// matches that language, any English record is returned; failing that,
// the first record for the requested NameID.
//
// Returns "" if the font has no name with the given NameID.
func (f *Font) NameByLanguage(id NameID, languageID uint16) string {
	recs := f.Names()
	var anyEnglish, anyMatch string
	for _, r := range recs {
		if r.NameID != id {
			continue
		}
		if r.LanguageID == languageID && r.PlatformID == 3 {
			return r.Value
		}
		// 0x0409 = en-US; 0x0000 (platform 0, Mac) = English.
		if anyEnglish == "" && isEnglishLanguage(r.PlatformID, r.LanguageID) {
			anyEnglish = r.Value
		}
		if anyMatch == "" {
			anyMatch = r.Value
		}
	}
	if anyEnglish != "" {
		return anyEnglish
	}
	return anyMatch
}

func isEnglishLanguage(platformID, languageID uint16) bool {
	switch platformID {
	case 0: // Unicode — language is a Unicode language tag index; 0 ==
		// "multilingual/not specified", which most fonts use for English.
		return true
	case 1: // Mac — 0 is English.
		return languageID == 0
	case 3: // Microsoft — 0x0409 is en-US; others starting with 0x09 are English.
		return languageID&0x00FF == 0x09
	}
	return false
}

// decodeNameString interprets raw name-table bytes according to the
// platform/encoding IDs.
func decodeNameString(src []byte, platformID, encodingID uint16) string {
	switch platformID {
	case 0, 3:
		// Unicode or Microsoft: UTF-16BE.
		return decodeUTF16BE(src)
	case 1:
		if encodingID == 0 {
			// Mac Roman — strictly we should map via the full table, but
			// the first 128 codepoints are ASCII-compatible and serving
			// those through as-is works for most fonts.
			return string(src)
		}
	}
	// Unknown encoding: return as-is.
	return string(src)
}

// decodeUTF16BE decodes a big-endian UTF-16 byte slice to a UTF-8 string.
func decodeUTF16BE(src []byte) string {
	if len(src)&1 != 0 {
		// Pad with a null so we don't drop the tail.
		src = append(src, 0)
	}
	u := make([]uint16, len(src)/2)
	for i := range u {
		u[i] = uint16(src[2*i])<<8 | uint16(src[2*i+1])
	}
	return string(utf16.Decode(u))
}
