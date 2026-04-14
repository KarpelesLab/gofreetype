// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import "testing"

func TestNamesLuxiSans(t *testing.T) {
	f, _, err := parseTestdataFont("luxisr")
	if err != nil {
		t.Fatal(err)
	}
	recs := f.Names()
	if len(recs) == 0 {
		t.Fatal("Names() returned empty list")
	}
	// At minimum, the font family name (NameID 1) should be present and
	// equal to "Luxi Sans".
	var familyNames []string
	for _, r := range recs {
		if r.NameID == NameIDFontFamily {
			familyNames = append(familyNames, r.Value)
		}
	}
	if len(familyNames) == 0 {
		t.Fatal("no Family name record found")
	}
	gotOne := false
	for _, name := range familyNames {
		if name == "Luxi Sans" {
			gotOne = true
			break
		}
	}
	if !gotOne {
		t.Errorf("Family names: got %v, expected one to be \"Luxi Sans\"", familyNames)
	}
}

func TestNameByLanguagePrefersEnglish(t *testing.T) {
	f, _, err := parseTestdataFont("luxisr")
	if err != nil {
		t.Fatal(err)
	}
	// 0x0409 is en-US; even if luxisr doesn't have exactly that language
	// ID, the fallback logic should still return an English family name.
	name := f.NameByLanguage(NameIDFontFamily, 0x0409)
	if name != "Luxi Sans" {
		t.Errorf("NameByLanguage(Family, 0x0409): got %q, want %q", name, "Luxi Sans")
	}
}

func TestDecodeUTF16BE(t *testing.T) {
	// "A" in UTF-16BE is 0x00 0x41.
	if got := decodeUTF16BE([]byte{0, 'A', 0, 'B'}); got != "AB" {
		t.Errorf("decodeUTF16BE: got %q, want %q", got, "AB")
	}
	// Japanese "あ" (U+3042) in UTF-16BE: 0x30 0x42.
	if got := decodeUTF16BE([]byte{0x30, 0x42}); got != "\u3042" {
		t.Errorf("decodeUTF16BE Japanese: got %q, want %q", got, "\u3042")
	}
	// Surrogate pair: U+1F600 = D83D DE00.
	if got := decodeUTF16BE([]byte{0xD8, 0x3D, 0xDE, 0x00}); got != "\U0001F600" {
		t.Errorf("decodeUTF16BE surrogate: got %q, want %q", got, "\U0001F600")
	}
}

func TestIsEnglishLanguage(t *testing.T) {
	cases := []struct {
		platform, language uint16
		want               bool
	}{
		{3, 0x0409, true},  // en-US
		{3, 0x0809, true},  // en-GB
		{3, 0x0411, false}, // ja-JP
		{1, 0, true},       // Mac English
		{1, 11, false},     // Mac Japanese
		{0, 0, true},
	}
	for _, c := range cases {
		if got := isEnglishLanguage(c.platform, c.language); got != c.want {
			t.Errorf("isEnglishLanguage(%d, %#x): got %v, want %v",
				c.platform, c.language, got, c.want)
		}
	}
}
