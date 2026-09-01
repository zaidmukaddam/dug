import type { Metadata } from "next"
import Link from "next/link"

import { Frame, FrameRows } from "@/app/screens/frame"
import { ThemeToggle } from "@/components/theme-provider"

const DESCRIPTION =
  "How the dug API signals that a route is going away: Deprecation and Sunset headers, at least 180 days of notice, and a replacement that ships first."

export const metadata: Metadata = {
  title: "Deprecation and sunset policy",
  description: DESCRIPTION,
  alternates: { canonical: "/deprecation" },
  openGraph: {
    type: "article",
    url: "https://dug.sh/deprecation",
    siteName: "dug",
    title: "dug API deprecation and sunset policy",
    description: DESCRIPTION,
  },
  twitter: {
    card: "summary_large_image",
    title: "dug API deprecation and sunset policy",
    description: DESCRIPTION,
  },
}

// Its own page rather than a section of /developers.
//
// It lived there as an anchor, and nothing linked to the anchor — not the
// catalog, not llms.txt, not the page itself — so the policy was, in practice,
// unfindable. An agent deciding whether to integrate asks two questions: how
// will I be told, and how long do I get. Both answers need a url that can be
// cited, indexed and linked, which a fragment is not.
export default function Deprecation() {
  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-6xl flex-col px-5 py-6 sm:px-8">
      <header className="flex flex-wrap items-end justify-between gap-x-6 gap-y-2 pb-7">
        <Link
          href="/"
          className="font-mono text-2xl leading-none tracking-tight text-foreground lowercase"
        >
          dug
        </Link>
        <p className="text-xs text-muted-foreground">
          <ThemeToggle />
        </p>
      </header>

      <section className="flex flex-col gap-6 pt-6">
        <h2 className="text-xl">The dug API deprecation and sunset policy</h2>

        <p className="max-w-3xl text-sm text-pretty text-muted-foreground">
          Nothing is deprecated today, and the absence of a{" "}
          <span className="text-foreground">Deprecation</span> header is how you can tell.
          Checking for one on each response is enough. Nothing here disappears without it, and
          you don’t need to watch this page to stay ahead of a removal.
        </p>

        <Frame title="how you are told">
          <div className="flex flex-col gap-4">
            <FrameRows
              rows={[
                {
                  label: "Deprecation",
                  value: "the day it became deprecated, as an HTTP date (RFC 9745)",
                  accent: true,
                },
                { label: "Sunset", value: "the day it stops answering, as an HTTP date (RFC 8594)" },
                { label: "Link", value: 'rel="deprecation", pointing at what to read' },
                { label: "where", value: "on the deprecated route’s own responses, not only here" },
              ]}
            />
            <p className="max-w-3xl text-sm text-pretty text-graph-muted">
              The signal travels with the thing being deprecated. A client that reads response
              headers already has everything it needs and never has to poll a changelog.
            </p>
          </div>
        </Frame>

        <Frame title="how long you get">
          <div className="flex flex-col gap-4">
            <FrameRows
              rows={[
                { label: "notice", value: "at least 180 days between the two dates", accent: true },
                { label: "replacement", value: "ships under /v2/ before either date is set" },
                { label: "order", value: "new surface first, then Deprecation, then Sunset" },
                { label: "after sunset", value: "410 Gone, with a Link to the replacement" },
              ]}
            />
            <p className="max-w-3xl text-sm text-pretty text-graph-muted">
              A replacement existing before the clock starts is the part that matters. Six months
              of warning isn’t much use if there’s nothing to migrate to for five of them.
            </p>
          </div>
        </Frame>

        <Frame title="what doesn’t count as a breaking change">
          <div className="flex flex-col gap-4">
            <p className="max-w-3xl text-sm text-pretty text-graph-muted">
              The paths are unversioned and additive. These happen without notice, so parse
              defensively and ignore what you don’t recognise:
            </p>
            <FrameRows
              rows={[
                { label: "new commands", value: "a new route appears, nothing existing moves", accent: true },
                { label: "new fields", value: "added to the envelope or to a block's props" },
                { label: "new blocks", value: "a screen grows a component you haven’t seen" },
                { label: "new notes", value: "provenance and limits get more specific" },
              ]}
            />
            <p className="text-xs text-graph-muted">
              A change that would break an existing caller isn’t one of these. It ships under a
              new path prefix, and the old one follows the timeline above.
            </p>
          </div>
        </Frame>

        <Frame title="versions">
          <p className="max-w-3xl text-sm text-pretty text-graph-muted">
            Every response carries <span className="text-foreground">X-API-Version</span>, naming
            the contract that produced it. Send the same header to pin it. Ask for a version this
            surface doesn’t serve and you get{" "}
            <span className="text-foreground">error.code unsupported_version</span>, not a silent
            answer from a different one.
          </p>
        </Frame>

        <footer className="flex flex-col gap-2 pt-4 pb-2 text-xs text-muted-foreground">
          <p>
            The rest of the contract (representations, the error model, the quota) is on{" "}
            <Link href="/developers" className="hover:text-foreground">
              /developers
            </Link>
            , and the machine-readable copy is in{" "}
            <Link href="/llms.txt" className="hover:text-foreground">
              /llms.txt
            </Link>
            .
          </p>
          <Link href="/" className="hover:text-foreground">
            back to the terminal
          </Link>
        </footer>
      </section>
    </main>
  )
}
