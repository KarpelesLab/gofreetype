// Copyright 2026 The gofreetype Authors. All rights reserved.
// Use of this source code is governed by your choice of either the
// FreeType License or the GNU General Public License version 2 (or
// any later version), both of which can be found in the LICENSE file.

package cff

import "math"

// Stem describes one horizontal (hstem) or vertical (vstem) stem
// declared by a charstring. Edge and Width are in font design units,
// before any scaling. For hstems, Edge is the bottom edge's Y value;
// for vstems, it is the left edge's X value.
type Stem struct {
	Horizontal bool
	Edge       float64
	Width      float64
}

// HintedGlyph is the output of LoadGlyphHinted: the same Segment stream
// as LoadGlyph, plus the list of stems the charstring declared.
type HintedGlyph struct {
	Glyph
	Stems []Stem
}

// LoadGlyphHinted decodes glyph gid's charstring and returns the
// ordinary segment stream plus the stems the charstring declared. The
// stems are not applied here - see SnapToPixelGrid for a simple
// grid-fitting pass that snaps stem edges to pixel boundaries at the
// caller's target scale.
func (f *Font) LoadGlyphHinted(gid int) (*HintedGlyph, error) {
	if gid < 0 || gid >= len(f.CharStrings) {
		return nil, FormatError("glyph index out of range")
	}
	locals := f.LocalSubrs
	nominalWidth := f.NominalWidthX
	defaultWidth := f.DefaultWidthX
	if f.IsCIDKeyed {
		if gid >= len(f.FDSelect) {
			return nil, FormatError("FDSelect has no entry for glyph")
		}
		fd := int(f.FDSelect[gid])
		if fd >= len(f.FDSubrs) {
			return nil, FormatError("FDSelect points past FDArray")
		}
		locals = f.FDSubrs[fd]
		nominalWidth = f.FDNominalWidthX[fd]
		defaultWidth = f.FDDefaultWidthX[fd]
	}
	p := &interp{
		globals:      f.GlobalSubrs,
		locals:       locals,
		nominalWidth: nominalWidth,
		defaultWidth: defaultWidth,
		hintSink:     &stemSink{},
	}
	if err := p.run(f.CharStrings[gid]); err != nil {
		return nil, err
	}
	out := &HintedGlyph{
		Glyph: Glyph{
			Segments: p.segments,
			Width:    p.width,
			HasWidth: p.hasWidth,
		},
	}
	if p.hintSink != nil {
		out.Stems = p.hintSink.stems
	}
	return out, nil
}

// stemSink accumulates stems declared by the Type 2 VM during a
// charstring run.
type stemSink struct {
	// currentY/currentX track running stem-edge positions as consecutive
	// hstem/vstem operands are emitted with delta encoding.
	currentY, currentX float64
	stems              []Stem
}

// SnapToPixelGrid returns a copy of g.Segments with any point whose
// coordinate matches a stem edge nudged to the nearest pixel at the
// caller-provided scale (pixels per font unit).
//
// Points that don't align with a stem edge are left at their decoded
// positions. The result is a minimal grid-fitting pass: good enough
// to sharpen vertical and horizontal stems at integer pixel widths
// without the full complexity of FreeType's CFF hinter.
func (g *HintedGlyph) SnapToPixelGrid(scale float64) []Segment {
	if scale <= 0 || len(g.Stems) == 0 {
		out := make([]Segment, len(g.Segments))
		copy(out, g.Segments)
		return out
	}

	// Build sets of y-edges (hstem) and x-edges (vstem).
	yEdges := make(map[float64]float64)
	xEdges := make(map[float64]float64)
	for _, s := range g.Stems {
		e1 := s.Edge
		e2 := s.Edge + s.Width
		s1 := snapToPixel(e1, scale)
		s2 := snapToPixel(e2, scale)
		if s.Horizontal {
			yEdges[e1] = s1
			yEdges[e2] = s2
		} else {
			xEdges[e1] = s1
			xEdges[e2] = s2
		}
	}

	out := make([]Segment, len(g.Segments))
	for i, seg := range g.Segments {
		out[i] = seg
		if sy, ok := yEdges[seg.Y]; ok {
			out[i].Y = sy
		}
		if sx, ok := xEdges[seg.X]; ok {
			out[i].X = sx
		}
		// Control points follow the same snapping rule so the curve
		// stays aligned with the endpoint.
		if seg.Op == SegCubicTo {
			if sy, ok := yEdges[seg.CY1]; ok {
				out[i].CY1 = sy
			}
			if sx, ok := xEdges[seg.CX1]; ok {
				out[i].CX1 = sx
			}
			if sy, ok := yEdges[seg.CY2]; ok {
				out[i].CY2 = sy
			}
			if sx, ok := xEdges[seg.CX2]; ok {
				out[i].CX2 = sx
			}
		}
	}
	return out
}

// snapToPixel rounds `coord * scale` to the nearest integer pixel, then
// divides back out by scale so the result is expressed in font units.
func snapToPixel(coord, scale float64) float64 {
	return math.Round(coord*scale) / scale
}
