# gofreetype

A pure-Go FreeType font engine.

This project is a fork of [github.com/golang/freetype](https://github.com/golang/freetype)
with the goal of reaching feature parity with the upstream C
[FreeType](https://freetype.org/) library: full cmap coverage, CFF/OpenType
outlines, OpenType layout (GSUB/GPOS), color/bitmap fonts, variable fonts,
autohinting, SDF and monochrome rasterizers.

## Install

```
go get github.com/KarpelesLab/gofreetype
```

## Status

Currently supports what the original `golang/freetype` supported:

- TrueType outline parsing and the TrueType bytecode hinter
- Anti-aliased vector rasterizer with stroker
- Basic `cmap` (formats 0, 4, 6, 12), `kern`, and core metrics

See `ROADMAP` commits for the in-progress parity work.

## Implementation notes

Internally a 26.6 fixed-point coordinate system is used everywhere, as opposed
to the original FreeType's mix of 26.6 (or 10.6 on 16-bit systems) and 24.8 in
the "smooth" rasterizer.

## License

`gofreetype` is derived from `golang/freetype`, which is in turn derived from
the original C FreeType. FreeType is copyright 1996-2010 David Turner, Robert
Wilhelm, and Werner Lemberg. The Freetype-Go Authors are listed in `AUTHORS`.

Unless otherwise noted, the source files in this repository are distributed
under the BSD-style license in `LICENSE`.
