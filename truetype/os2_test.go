// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package truetype

import (
	"testing"
)

func TestOS2LuxiSans(t *testing.T) {
	f, _, err := parseTestdataFont("luxisr")
	if err != nil {
		t.Fatal(err)
	}
	info := f.OS2()
	if info == nil {
		t.Fatal("OS2() is nil for luxisr")
	}
	// Luxi Sans Regular should be weight 400 (Regular), width 5 (Medium).
	if info.WeightClass == 0 || info.WeightClass > 1000 {
		t.Errorf("WeightClass: got %d, want in (0, 1000]", info.WeightClass)
	}
	// Regular font should not be marked bold or italic.
	if f.IsBold() {
		t.Error("luxisr IsBold() = true")
	}
	if f.IsItalic() {
		t.Error("luxisr IsItalic() = true")
	}
}

func TestOS2BoldWeightClass(t *testing.T) {
	f := &Font{
		os2Info: &OS2Info{WeightClass: 700},
	}
	if !f.IsBold() {
		t.Error("IsBold() = false for WeightClass=700")
	}
}

func TestOS2ItalicSelectionBit(t *testing.T) {
	f := &Font{
		os2Info: &OS2Info{Selection: StyleItalic},
	}
	if !f.IsItalic() {
		t.Error("IsItalic() = false for Selection & StyleItalic")
	}
}

func TestOS2BoldSelectionBit(t *testing.T) {
	f := &Font{
		os2Info: &OS2Info{Selection: StyleBold, WeightClass: 400},
	}
	if !f.IsBold() {
		t.Error("IsBold() = false for Selection & StyleBold")
	}
}
