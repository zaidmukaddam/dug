"use client"

// The last resort. This replaces the root layout, so it renders its own html
// and body and cannot rely on anything the layout set up: not the font
// variables, not the theme class, not necessarily the stylesheet. Everything
// here is inline and self-contained on purpose, because a boundary that depends
// on the thing that might have broken is not a boundary.
//
// app/error.tsx handles the ordinary case and is the one worth reading; this
// only fires when the layout itself throws.

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  const mono = "ui-monospace, SFMono-Regular, Menlo, monospace"

  return (
    <html lang="en">
      <body style={{ margin: 0, background: "#0a0a0a", color: "#ededed" }}>
        {/* Only the two colours change with the scheme, so a media query beats
            shipping a theme system into the failure path. */}
        <style>{`@media (prefers-color-scheme: light){body{background:#fff!important;color:#171717!important}}`}</style>

        <main
          style={{
            fontFamily: mono,
            fontSize: 14,
            lineHeight: 1.6,
            margin: "0 auto",
            maxWidth: "48rem",
            padding: "3rem 1.25rem",
          }}
        >
          <p style={{ opacity: 0.6, textTransform: "uppercase", margin: 0 }}>dug</p>

          <p style={{ fontSize: 20, lineHeight: 1.3, margin: "3rem 0 0" }}>
            <span aria-hidden="true" style={{ opacity: 0.6 }}>
              [!]{" "}
            </span>
            the app failed to start
          </p>

          <p style={{ opacity: 0.6, margin: "0.75rem 0 0", paddingLeft: "2.25rem" }}>
            this is a fault in the application itself, not a failed lookup. no answer was
            produced, correct or otherwise.
          </p>

          {error.digest ? (
            <p style={{ opacity: 0.6, margin: "2rem 0 0" }}>digest {error.digest}</p>
          ) : null}

          <button
            type="button"
            onClick={reset}
            style={{
              background: "transparent",
              border: "1px dashed currentColor",
              color: "inherit",
              cursor: "pointer",
              font: "inherit",
              marginTop: "2rem",
              padding: "0.5rem 0.75rem",
            }}
          >
            reload
          </button>
        </main>
      </body>
    </html>
  )
}
