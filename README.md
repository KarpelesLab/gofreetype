# gofreetype

A pure-Go font engine aiming at feature parity with the C
[FreeType](https://freetype.org/) library. Originally forked from
[github.com/golang/freetype](https://github.com/golang/freetype), extended
to cover the formats and features modern applications need: OpenType/CFF,
GSUB/GPOS shaping, variable fonts, color emoji, web fonts, and the
legacy Adobe Type 1 and X11 bitmap formats.

## Install

```
go get github.com/KarpelesLab/gofreetype
```

No runtime dependencies outside `golang.org/x/image`.

## Packages

| Package | Purpose |
|---|---|
| `truetype` | TTF/OTF parser, hinter, `font.Face`, WOFF/TTC auto-unwrap, variable-font integration, `ColorGlyph` + `BitmapGlyph` |
| `cff` | Compact Font Format v1 container + Type 2 charstring VM + charset/glyph names + stem-snap hinter |
| `type1` | Adobe Type 1 (PFB): container + eexec + charstring VM + PostScript dict scanner |
| `raster` | Anti-aliased, monochrome, LCD subpixel, and signed-distance-field rasterizers; stroker; gamma |
| `color` | CPAL + COLR v0 + sbix + CBDT/CBLC + SVG tables |
| `varfont` | fvar, avar, gvar, HVAR, MVAR, STAT + shared ItemVariationStore |
| `layout` | Shared OpenType Layout: Script/Feature/Lookup, Coverage, ClassDef, context matcher |
| `gsub` | Glyph substitution: all 8 Lookup types |
| `gpos` | Glyph positioning: all 9 Lookup types |
| `gdef` | Glyph class definitions, mark glyph sets |
| `shape` | End-to-end GSUB + GPOS pipeline on top of the layout packages |
| `woff` | WOFF 1.0 decoder (auto-detected by `truetype.Parse`) |
| `bdf` | Adobe BDF bitmap fonts + `font.Face` adapter |
| `pcf` | X11 PCF bitmap fonts + `font.Face` adapter |

Top-level `freetype` package: legacy `Context`-based text drawing API,
preserved for compatibility with the original `golang/freetype`.

## Quick start

```go
data, _ := os.ReadFile("some-font.ttf") // or .otf, .woff, .ttc, .pfb
f, err := truetype.Parse(data)
if err != nil {
    log.Fatal(err)
}

face := truetype.NewFace(f, &truetype.Options{Size: 14, DPI: 72})
defer face.Close()

dst := image.NewRGBA(image.Rect(0, 0, 600, 80))
draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
d := &font.Drawer{
    Dst:  dst,
    Src:  image.Black,
    Face: face,
    Dot:  fixed.P(10, 40),
}
d.DrawString("Hello, world!")
```

## Shaped text (kern pairs, ligatures, marks)

```go
opts := shape.Default(layout.MakeTag("latn"), layout.MakeTag("dflt"))
glyphs := shape.ShapeString(f, "Hello fi", opts)
for _, g := range glyphs {
    // g.GID, g.XAdvance, g.XOffset, ...
}
```

## Variable fonts

```go
f.SetVariation("wght", 700) // bold
f.SetVariation("wdth", 125) // extra-wide
// subsequent face renders use the new axis coords
```

## Color glyphs

```go
if f.COLR() != nil && f.COLR().IsColorGlyph(gid) {
    dr, rgba, _, _, _ := face.(*truetype.face).ColorGlyph(dot, '\u2603', 0, nil)
    draw.Draw(dst, dr, rgba, rgba.Bounds().Min, draw.Over)
}
```

## Features

Format coverage is comprehensive for what real-world fonts actually ship:

- **TrueType**: all cmap formats (0/4/6/10/12/13/14 including variation
  selectors), TrueType bytecode hinter, composite glyphs, kerning
  (multiple subtables), OS/2 rich metadata, post table glyph names,
  multi-language name records with UTF-16 + surrogate-pair decoding
- **OpenType/CFF**: CFF v1 container, Type 2 charstring VM, CFF charset
  with predefined standard strings, CID-keyed fonts, stem-snap hinter
- **OpenType Layout**: all 8 GSUB and 9 GPOS lookup types (single,
  multiple, alternate, ligature, contextual, chaining contextual,
  reverse chaining, extension), shared context matcher, GDEF mark
  classes, STAT style attributes
- **Color**: CPAL palettes, COLR v0 layered color glyphs (composite to
  RGBA through `face.ColorGlyph`), sbix + CBDT/CBLC PNG bitmaps
  (decoded through `face.BitmapGlyph`), SVG table
- **Variable fonts**: fvar axes + named instances, avar non-linear
  remapping, gvar per-glyph tuple variations with shared + private
  point sets and packed deltas, HVAR metric variations, MVAR font-wide
  metric variations, STAT
- **Containers**: TTC / OTC collections (`ParseIndex` / `NumFonts`),
  WOFF 1.0 (auto-detected by `Parse`), Type 1 PFB with eexec
  decryption, BDF + PCF bitmap fonts
- **Rasterizers**: anti-aliased 8-bit, 1-bit packed `Bitmap`, LCD
  subpixel (horizontal + vertical, RGB + BGR) with FIR filter, signed
  distance field with Chamfer distance transform

## Testing

```
go test ./...         # unit + integration tests
go vet ./...          # clean across every package
go test -fuzz=...     # fuzz targets on every parser
```

## License

Derived from `golang/freetype`, which is derived from the original C
FreeType. Distributed under the BSD-style license in `LICENSE`.
