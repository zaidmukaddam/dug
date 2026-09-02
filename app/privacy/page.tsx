import type { Metadata } from "next"
import Link from "next/link"

import { Frame, FrameRows } from "@/app/screens/frame"
import { ThemeToggle } from "@/components/theme-provider"

const DESCRIPTION =
  "What dug stores, which is nothing between requests, and what the two analytics scripts on the page collect."

export const metadata: Metadata = {
  title: "Privacy",
  description: DESCRIPTION,
  alternates: { canonical: "/privacy" },
  openGraph: {
    type: "article",
    url: "https://dug.sh/privacy",
    siteName: "dug",
    title: "Privacy at dug",
    description: DESCRIPTION,
  },
  twitter: {
    card: "summary_large_image",
    title: "Privacy at dug",
    description: DESCRIPTION,
  },
}

// Every claim here is a property of the code as it stands: there is no database
// in this project, no session, no cookie set by the app, and the third parties
// are the two analytics scripts and, only for the planner, the model provider.
// Nothing on this page is a promise about intent; it is a description of what
// the deployment does.
export default function Privacy() {
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
        <h2 className="text-xl">Privacy</h2>

        <p className="max-w-3xl text-sm text-pretty text-muted-foreground">
          dug has no accounts, no database and no session. There’s nothing to sign up for and
          nothing to sign in to, so there’s no profile to build. Each query goes to the
          upstreams it names, comes back, and then dug forgets it. The only copies that outlive
          the request are the one in your browser tab and the one in a shared HTTP cache, held
          for the lifetime the answer itself declares.
        </p>

        <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-2">
          <Frame title="what is stored">
            <FrameRows
              rows={[
                { label: "queries", value: "not stored, not logged to any datastore", accent: true },
                { label: "targets", value: "not retained after the request that named them" },
                { label: "accounts", value: "none exist" },
                { label: "cookies", value: "none set by this app" },
                { label: "history", value: "kept in the tab, lost on reload" },
                { label: "cache", value: "in the tab, and in a shared http cache by ttl" },
              ]}
            />
          </Frame>

          <Frame title="third parties">
            <div className="flex flex-col gap-4">
              <p className="text-sm text-pretty text-graph-muted">
                The site is hosted on Vercel, which terminates TLS and therefore sees request
                metadata as any host does. Two Vercel scripts run on the page, and one model
                provider is called from the prompt:
              </p>
              <FrameRows
                rows={[
                  {
                    label: "Analytics",
                    value: "aggregate page views, no cookie and no cross-site identifier",
                    accent: true,
                  },
                  {
                    label: "Speed Insights",
                    value: "anonymous page timing, sampled",
                  },
                  {
                    label: "OpenAI",
                    value:
                      "receives the sentence you type in plain words, only when the planner runs, with retention turned off",
                  },
                ]}
              />
              <p className="text-xs text-graph-muted">
                Vercel injects both scripts only on a deployment, so a local run of this project
                sends no beacons at all. A command such as TLS example.com never reaches the
                model provider: the planner runs only for a question in plain words, sends that
                text and nothing else, and asks the provider not to store it.
              </p>
            </div>
          </Frame>
        </div>

        <Frame title="what your queries reach">
          <div className="flex flex-col gap-4">
            <p className="max-w-3xl text-sm text-pretty text-graph-muted">
              The deployment makes the lookup, not your browser, so the target sees a request
              from this service, not from you. That cuts both ways. Your address isn’t exposed
              to the domain you ask about, and the domain you ask about is visible to the
              resolvers and registries the command names.
            </p>
            <p className="max-w-3xl text-sm text-pretty text-graph-muted">
              PORTS is the one command that completes a TCP handshake against the target, so it
              appears in that target’s logs as a connection from this deployment. Only run it
              against hosts you’re allowed to probe.
            </p>
          </div>
        </Frame>

        <footer className="flex flex-col gap-2 pt-4 pb-2 text-xs text-muted-foreground">
          <p>
            This page describes the deployment at dug.sh. Running the project yourself removes
            the analytics scripts and every third party except the upstreams a command queries.
          </p>
          <p>
            Questions about any of this go to the issue tracker, which is linked from{" "}
            <Link href="/contact" className="hover:text-foreground">
              /contact
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
