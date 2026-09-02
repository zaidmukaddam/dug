import type { Metadata } from "next"
import Link from "next/link"

import { COMMANDS, FAMILIES } from "@/app/commands/grammar"
import { Frame, FrameRows } from "@/app/screens/frame"
import { ThemeToggle } from "@/components/theme-provider"

const DESCRIPTION =
  "The dug REST API: every command as a GET, no key and no signup, in text, JSON or markdown. Error model, versioning policy, OpenAPI and MCP."

export const metadata: Metadata = {
  // "dug API documentation" rather than the template's "Developers · dug".
  // A name-based search is for the product plus a developer word, and the
  // template put the product name last and the generic word first.
  title: { absolute: "dug API documentation" },
  description: DESCRIPTION,
  alternates: { canonical: "/developers" },
  openGraph: {
    type: "article",
    url: "https://dug.sh/developers",
    siteName: "dug",
    title: "dug API for developers and agents",
    description: DESCRIPTION,
  },
  twitter: {
    card: "summary_large_image",
    title: "dug API for developers and agents",
    description: DESCRIPTION,
  },
}

// Rendered from the same COMMANDS the browser parses and the Go registry
// mirrors, so this page cannot list a command that does not exist or miss one
// that does. The prose here is only the parts a table cannot say.
export default function Developers() {
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
        {/* Named rather than "The API", so a search for the product plus a
            developer word has something to match on the page itself. */}
        <h2 className="text-xl">The dug API</h2>

        <p className="max-w-3xl text-sm text-pretty text-muted-foreground">
          Every command is a GET. There’s no key, no signup and no request body to build, and
          every response publishes the quota. The same URL answers in three representations,
          and the browser app at{" "}
          <Link href="/" className="text-graph-accent">
            /
          </Link>{" "}
          is one of them, not a separate product.
        </p>

        <Frame title="calling it">
          <div className="flex flex-col gap-3 text-sm">
            <p className="text-graph-muted">
              <span className="text-graph-accent">$</span> curl https://dug.sh/tls/github.com
            </p>
            <p className="text-graph-muted">
              <span className="text-graph-accent">$</span> curl https://dug.sh/dig/example.com/MX
            </p>
            <p className="text-graph-muted">
              <span className="text-graph-accent">$</span> curl -H &apos;Accept:
              application/json&apos; https://dug.sh/mail/github.com
            </p>
            <p className="pt-2 text-xs text-graph-muted">
              The query form is equivalent and is what the browser app uses:{" "}
              <span className="text-foreground">
                /api/tls?command=TLS&amp;target=github.com
              </span>
            </p>
          </div>
        </Frame>

        <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-3">
          <Frame title="representations" className="lg:col-span-2">
            <FrameRows
              rows={[
                {
                  label: "text/plain",
                  value: "the default for curl and other terminal clients",
                  accent: true,
                },
                { label: "application/json", value: "Accept: application/json, or ?format=json" },
                { label: "text/markdown", value: "Accept: text/markdown, on the pages" },
                { label: "text/html", value: "the browser app; a command url opened in a browser lands there with the command run" },
                { label: "Vary", value: "Accept, User-Agent, so negotiation is cache safe" },
                { label: "Cache-Control", value: "derived from the answer’s own TTL, floored at 30s" },
              ]}
            />
          </Frame>

          <Frame title="auth">
            <p className="text-sm text-graph-muted">
              None. Reads are open and there’s no key to obtain, so there’s nothing here to
              sandbox. The guard validates every destination before connect, which is what makes
              open reads safe. The quota below applies per address, since there’s no key.
            </p>
          </Frame>
        </div>

        <Frame title="errors">
          <div className="flex flex-col gap-4">
            <p className="text-sm text-pretty text-graph-muted">
              An upstream that fails mid-answer is <span className="text-foreground">200</span>:
              the failure is named in <span className="text-foreground">degraded</span> and the
              rest of the answer is real. Arguments that are wrong are{" "}
              <span className="text-foreground">400</span>, because nothing was looked up. Only
              a 4xx carries <span className="text-foreground">error</span>, so its presence and
              the status always agree.
            </p>
            <FrameRows
              rows={[
                { label: "error.code", value: "missing_argument, invalid_argument", accent: true },
                { label: "error.message", value: "the refusal in one sentence" },
                { label: "error.hint", value: "what a corrected call looks like" },
              ]}
            />
            <p className="text-xs text-graph-muted">
              Branch on the code, never on the message. The text representation carries the
              same code on a line reading &quot;error &lt;code&gt;&quot;.
            </p>
          </div>
        </Frame>

        <Frame title="rate limits">
          <div className="flex flex-col gap-4">
            <FrameRows
              rows={[
                { label: "RateLimit-Limit", value: "60 requests per window", accent: true },
                { label: "RateLimit-Remaining", value: "what is left in this window" },
                { label: "RateLimit-Reset", value: "seconds until it resets" },
                { label: "RateLimit-Policy", value: '"fixed";q=60;w=60, per RFC 9331' },
                { label: "over the quota", value: "429, error.code rate_limited, Retry-After" },
              ]}
            />
            <p className="max-w-3xl text-sm text-pretty text-graph-muted">
              Every response carries these, including a 200, so a caller can pace itself
              instead of discovering the ceiling by hitting it. The count lives in the memory
              of one proxy instance, and a serverless deployment runs more than one, so a
              client spread across instances gets more than 60. It’s a real backstop and a
              truthful number to pace against. It isn’t a security control.
            </p>
          </div>
        </Frame>

        {/* The policy itself moved to /deprecation. It was a fragment here, and
            a fragment cannot be indexed, cited or linked from a catalog — which
            is exactly why nothing ever found it. This keeps the part a caller
            needs while reading about the API and sends them to the page for the
            timeline. */}
        <Frame title="versioning and deprecation">
          <div className="flex flex-col gap-4">
            <p className="max-w-3xl text-sm text-pretty text-graph-muted">
              The paths are unversioned and additive. New commands, blocks and fields may appear
              at any time, so parse defensively and ignore what you don’t recognise. A change
              that would break an existing caller ships under a{" "}
              <span className="text-foreground">/v2/</span> prefix instead of changing these.
            </p>
            <FrameRows
              rows={[
                {
                  label: "X-API-Version",
                  value: "on every response, naming the contract it ran under",
                  accent: true,
                },
                { label: "Deprecation", value: "an http-date, the day it became deprecated" },
                { label: "Sunset", value: "an http-date, the day it stops answering" },
                { label: "notice", value: "at least 180 days between the two dates" },
              ]}
            />
            <p className="text-xs text-graph-muted">
              Nothing is deprecated today, and the absence of a Deprecation header is how you can
              tell. The full timeline is at{" "}
              <Link href="/deprecation" className="text-graph-accent">
                /deprecation
              </Link>
              .
            </p>
          </div>
        </Frame>

        <h2 className="pt-2 text-xl">Endpoints</h2>

        <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-2">
          {FAMILIES.map((family) => (
            <Frame key={family} title={family}>
              <ul className="flex flex-col gap-3">
                {COMMANDS.filter((spec) => spec.family === family && spec.endpoint).map((spec) => (
                  <li key={spec.name} className="flex flex-col">
                    <span className="text-foreground">
                      <span className="text-graph-muted">GET </span>/{spec.name.toLowerCase()}
                      {spec.argument === "none" ? "" : "/{target}"}
                    </span>
                    <span className="text-xs text-graph-muted">{spec.summary}</span>
                  </li>
                ))}
              </ul>
            </Frame>
          ))}
        </div>

        <Frame title="machine readable">
          <ul className="grid grid-cols-1 gap-x-16 gap-y-3 sm:grid-cols-2 lg:grid-cols-4">
            <DevLink href="/llms.txt" note="the grammar, the limits, when to use this" />
            <DevLink href="/openapi.json" note="openapi 3.1, one operation per command" />
            <DevLink href="/.well-known/api-catalog" note="rfc 9727 linkset" />
            <DevLink href="/.well-known/ai-catalog.json" note="ai catalog, typed by protocol" />
            <DevLink
              href="/.well-known/mcp/server-card.json"
              note="mcp server card, every tool without connecting"
            />
            <DevLink href="/server.json" note="mcp server manifest" />
            <DevLink href="/mcp" note="mcp over streamable http, post only" />
            <DevLink href="/deprecation" note="how a route is retired" />
            <DevLink href="/contact" note="report a wrong answer" />
          </ul>
        </Frame>

        <footer className="pt-4 pb-2 text-xs text-muted-foreground">
          <Link href="/" className="hover:text-foreground">
            back to the terminal
          </Link>
        </footer>
      </section>
    </main>
  )
}

function DevLink({ href, note }: { href: string; note: string }) {
  return (
    <li>
      <Link href={href} className="group flex flex-col">
        <span className="text-foreground group-hover:text-graph-accent">{href}</span>
        <span className="text-xs text-graph-muted">{note}</span>
      </Link>
    </li>
  )
}
