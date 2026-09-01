// The card every link to this tool unfurls as.
//
// Drawn rather than screenshotted, because a screenshot of the landing goes
// stale the moment the landing changes, and at 1200x630 the command grid is
// unreadable anyway. What survives at that size is the thing the tool actually
// does: a verdict in one sentence with its evidence line under it.
//
// Satori, not a browser. It lays out a subset of flexbox and needs a font
// handed to it, so every container here is explicitly display:flex and the
// typeface is the vendored file rather than next/font.

import { readFile } from "node:fs/promises"
import { join } from "node:path"

import { ImageResponse } from "next/og"

export const alt = "dug — a command driven terminal for domain and network diagnostics"
export const size = { width: 1200, height: 630 }
export const contentType = "image/png"

// The dark palette, resolved. Satori does not run the cascade, so the oklch
// custom properties in globals.css are of no use here and these are their
// sRGB equivalents.
const BACKGROUND = "#0a0a0a"
const FOREGROUND = "#fafafa"
const MUTED = "#8a8a8a"
const ACCENT = "#84a9f0"
const RULE = "#3d3d3d"

export default async function Image() {
  // Vendored next to the app rather than fetched: this route is prerendered at
  // build, and a build that reaches a CDN is a build that can fail for reasons
  // that have nothing to do with the code. If the read ever does fail, the card
  // is still worth shipping in whatever face Satori falls back to.
  const geistMono = await readFile(
    join(process.cwd(), "lib/fonts/geist-mono-regular.ttf")
  ).catch(() => null)

  // The corner marks and the caption both sit on the border and have to punch a
  // hole in it, which they do by painting the page colour behind themselves.
  // Same trick the real frame uses in css, for the same reason.
  const corner = {
    position: "absolute" as const,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    width: 28,
    height: 28,
    background: BACKGROUND,
    color: RULE,
    fontSize: 28,
  }

  return new ImageResponse(
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        background: BACKGROUND,
        color: FOREGROUND,
        fontFamily: "Geist Mono",
        padding: 56,
      }}
    >
      {/* The frame itself. Satori will not draw the repeating gradient the
          graph-frame utility uses, but it does draw a dashed border, and at
          this size the two are indistinguishable. */}
      <div
        style={{
          position: "relative",
          display: "flex",
          flexDirection: "column",
          justifyContent: "center",
          flex: 1,
          border: `2px dashed ${RULE}`,
          padding: 72,
        }}
      >
        <div style={{ ...corner, top: -14, left: -14 }}>+</div>
        <div style={{ ...corner, top: -14, right: -14 }}>+</div>
        <div style={{ ...corner, bottom: -14, left: -14 }}>+</div>
        <div style={{ ...corner, bottom: -14, right: -14 }}>+</div>

        {/* Named on its own top edge, the way every frame in the tool is. */}
        <div
          style={{
            position: "absolute",
            top: -20,
            left: 0,
            right: 0,
            display: "flex",
            justifyContent: "center",
          }}
        >
          <div
            style={{
              display: "flex",
              background: BACKGROUND,
              color: ACCENT,
              padding: "0 18px",
              fontSize: 26,
            }}
          >
            [ dug ]
          </div>
        </div>

        <div style={{ display: "flex", fontSize: 62, letterSpacing: -2 }}>dug</div>
        <div style={{ display: "flex", marginTop: 18, fontSize: 25, color: MUTED }}>
          domain and network diagnostics
        </div>

        {/* One real answer, in the shape the tool prints them: the glyph and
            the sentence, then the evidence under it. */}
        <div style={{ display: "flex", flexDirection: "column", marginTop: 52 }}>
          <div style={{ display: "flex", fontSize: 23, color: MUTED }}>
            <span style={{ color: ACCENT }}>&gt;</span>
            <span style={{ marginLeft: 16 }}>TLS github.com</span>
          </div>

          <div style={{ display: "flex", marginTop: 24, fontSize: 33, color: FOREGROUND }}>
            <span style={{ color: ACCENT }}>[x]</span>
            <span style={{ marginLeft: 20 }}>a valid certificate for 30 more days</span>
          </div>

          <div style={{ display: "flex", marginTop: 16, fontSize: 21, color: MUTED }}>
            live, held 3600s · 87ms · 5 lookups
          </div>
        </div>
      </div>
    </div>,
    {
      ...size,
      fonts: geistMono
        ? [{ name: "Geist Mono", data: geistMono, style: "normal", weight: 400 }]
        : [],
    }
  )
}
