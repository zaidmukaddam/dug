"use client"

// The route error boundary.
//
// Handlers already turn an upstream failure into a screen rather than an error,
// so reaching this page means the frontend itself broke. It says that plainly
// instead of implying the lookup failed, because the difference matters to
// whoever is reading it.

import { Notice } from "@/app/screens/notice"

export default function ErrorScreen({
  error,
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  const rows = [
    { label: "where", value: "the browser, not the lookup", accent: true },
    { label: "reason", value: error.message || "no message was attached" },
  ]

  // The digest is the only handle on a production stack trace, which is
  // stripped before it reaches the client.
  if (error.digest) {
    rows.push({ label: "digest", value: error.digest })
  }

  return (
    <Notice
      headline="this screen failed to render"
      detail="nothing was answered incorrectly. the request may not have run at all, so treat the last screen as stale rather than wrong."
      title="failure"
      rows={rows}
      links={[
        { href: "/", label: "/", note: "back to the terminal" },
        { href: "/src", label: "/src", note: "whether the upstreams are answering" },
      ]}
    >
      <div>
        <button
          type="button"
          onClick={reset}
          className="graph-frame px-3 py-2 font-mono text-sm hover:text-graph-accent"
        >
          retry this screen
        </button>
      </div>
    </Notice>
  )
}
