// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

// Package shape glues the GSUB substitution and GPOS positioning layers
// into a usable text-shaping API. Given a string (or a slice of runes),
// a Font, a script tag, and a list of feature tags to enable, Shape
// returns a slice of Glyph records with final glyph ids, advances, and
// positioning offsets.
//
// The shaper implements a default-script processing path suitable for
// Latin/Greek/Cyrillic and the many other scripts that do not need
// contextual reordering. Proper Arabic cursive joining and Indic
// reordering live above this layer and can be added as additional
// processing stages; this package intentionally keeps its scope to the
// GSUB/GPOS substrate so higher layers (script shapers) can build on top.
package shape

import (
	"github.com/KarpelesLab/gofreetype/gpos"
	"github.com/KarpelesLab/gofreetype/gsub"
	"github.com/KarpelesLab/gofreetype/layout"
	"github.com/KarpelesLab/gofreetype/truetype"
)

// SequenceLookupRecord re-exports the layout.SequenceLookupRecord so shape
// callers don't have to import layout just to read the nested actions.
type SequenceLookupRecord = layout.SequenceLookupRecord

// Glyph is one glyph in a shaped output buffer.
type Glyph struct {
	// GID is the glyph index in the font.
	GID uint16

	// Cluster is the byte offset (or rune offset, depending on caller
	// convention) of the source text this glyph came from. When a
	// ligature substitution merges several source runes into one glyph,
	// all merged glyphs share the first rune's cluster value.
	Cluster int

	// XAdvance is the horizontal advance in font design units.
	XAdvance int32

	// YAdvance is the vertical advance in font design units (0 for the
	// common horizontal case).
	YAdvance int32

	// XOffset / YOffset are placement offsets applied before drawing.
	XOffset int32
	YOffset int32

	// gdefClass caches the GDEF class of GID — 0 if no GDEF, else one of
	// gdef.ClassBase, gdef.ClassLigature, gdef.ClassMark, gdef.ClassComponent.
	gdefClass uint16
}

// Options controls which features the shaper enables and in what script/
// language context. Call Default() for a sensible Latin-style bundle.
type Options struct {
	Script   layout.Tag
	Language layout.Tag

	// Features is the ordered list of feature tags to apply. Order matters
	// for GSUB (substitutions change the glyph stream that later features
	// see) but not for GPOS. Default() returns a conventional bundle.
	Features []layout.Tag
}

// Default returns an Options preset that enables the conventional text-
// shaping bundle: contextual alternates, required ligatures, standard
// ligatures (all GSUB), then kerning and mark positioning (GPOS). Works
// well for Latin, Greek, Cyrillic, and other simple scripts.
func Default(script, lang layout.Tag) Options {
	return Options{
		Script:   script,
		Language: lang,
		Features: []layout.Tag{
			layout.MakeTag("ccmp"),
			layout.MakeTag("rlig"),
			layout.MakeTag("liga"),
			layout.MakeTag("clig"),
			layout.MakeTag("calt"),
			layout.MakeTag("kern"),
			layout.MakeTag("mark"),
			layout.MakeTag("mkmk"),
		},
	}
}

// ShapeString is a convenience wrapper over Shape for callers working with
// strings. The cluster value on each output Glyph is the byte offset of
// the source rune.
func ShapeString(f *truetype.Font, text string, opts Options) []Glyph {
	runes := make([]rune, 0, len(text))
	clusters := make([]int, 0, len(text))
	byteOff := 0
	for i, r := range text {
		_ = i
		runes = append(runes, r)
		clusters = append(clusters, byteOff)
		byteOff += len(string(r))
	}
	return Shape(f, runes, clusters, opts)
}

// Shape runs the GSUB then GPOS pipeline over the given runes/clusters
// pair and returns a sequence of positioned glyphs.
func Shape(f *truetype.Font, runes []rune, clusters []int, opts Options) []Glyph {
	// 1. cmap lookup — runes -> initial glyph buffer.
	glyphs := make([]Glyph, len(runes))
	for i, r := range runes {
		gid := uint16(f.Index(r))
		cluster := i
		if i < len(clusters) {
			cluster = clusters[i]
		}
		glyphs[i] = Glyph{
			GID:     gid,
			Cluster: cluster,
		}
	}

	// 2. GSUB — apply each enabled feature's lookups in declared order.
	glyphs = runGSUB(f, glyphs, opts)

	// 3. Initial advances — pull from hmtx.
	populateAdvances(f, glyphs)

	// 4. Populate GDEF classes for any later mark-filtering.
	populateGDEFClass(f, glyphs)

	// 5. GPOS — apply positioning lookups.
	runGPOS(f, glyphs, opts)

	return glyphs
}

// populateAdvances fills XAdvance for each glyph from the font's hmtx
// table. This is the pre-GPOS baseline advance.
func populateAdvances(f *truetype.Font, glyphs []Glyph) {
	for i := range glyphs {
		h := f.UnscaledHMetric(truetype.Index(glyphs[i].GID))
		glyphs[i].XAdvance = int32(h.AdvanceWidth)
	}
}

// populateGDEFClass fills the cached GDEF class for each glyph.
func populateGDEFClass(f *truetype.Font, glyphs []Glyph) {
	gd := f.GDEF()
	if gd == nil {
		return
	}
	for i := range glyphs {
		glyphs[i].gdefClass = gd.Class(glyphs[i].GID)
	}
}

// enabledLookupIndices returns the lookup indices that should run for the
// given layout table, script, language, and feature tag list, in the
// order listed by `features`. Features are iterated first-to-last; within
// a feature, lookups are iterated in their declared order.
func enabledLookupIndices(tbl *layout.Table, opts Options, features []layout.Tag) []uint16 {
	if tbl == nil {
		return nil
	}
	ls := tbl.FindLanguage(opts.Script, opts.Language)
	if ls == nil {
		return nil
	}
	// Build a set of feature tags we want.
	want := make(map[layout.Tag]bool, len(features))
	for _, t := range features {
		want[t] = true
	}
	// Walk the language's feature indices in order so that the enabled
	// features are applied in their per-font order (spec-recommended).
	var out []uint16
	for _, fi := range ls.FeatureIndexes {
		feat := tbl.FindFeature(fi)
		if feat == nil || !want[feat.Tag] {
			continue
		}
		out = append(out, feat.LookupIndices...)
	}
	// Also include the required feature (if any).
	if ls.RequiredFeatureIndex != 0xffff {
		if feat := tbl.FindFeature(ls.RequiredFeatureIndex); feat != nil {
			out = append(out, feat.LookupIndices...)
		}
	}
	return out
}

// gsubOf and gposOf are trivial accessors used by the run drivers.
func gsubOf(f *truetype.Font) *gsub.Table { return f.GSUB() }
func gposOf(f *truetype.Font) *gpos.Table { return f.GPOS() }
