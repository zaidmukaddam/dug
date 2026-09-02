import type { Metadata } from "next"
import Link from "next/link"

import { NOT_HERE } from "@/app/commands/grammar"
import { Frame, FrameRows } from "@/app/screens/frame"
import { ThemeToggle } from "@/components/theme-provider"
import { RESOLVERS } from "@/lib/resolvers"
import { MACHINE_READABLE, READING_A_SCREEN, THE_GUARD } from "@/lib/site-copy"

const ABOUT_DESCRIPTION =
  "How a screen is read, what the address guard refuses, and where the answers come from."

export const metadata: Metadata = {
  // A plain string, so the root layout's "%s · dug" template applies and this
  // does not end up titled "About dug · dug".
  title: "About",
  description: ABOUT_DESCRIPTION,
  alternates: { canonical: "/about" },
  openGraph: {
    type: "article",
    url: "https://dug.sh/about",
    siteName: "dug",
    title: "About dug",
    description: ABOUT_DESCRIPTION,
  },
  twitter: {
    card: "summary_large_image",
    title: "About dug",
    description: ABOUT_DESCRIPTION,
  },
}

// Facts the tool already holds, rendered from the same constants the commands
// use. Nothing here is written twice: the resolver list and the non-goals come
// from source, so this page cannot drift from what the tool actually does.
export default function About() {
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

      <section className="flex flex-col gap-6 pt-8">
        <p className="max-w-2xl text-sm text-pretty text-muted-foreground">
          Live domain and network diagnostics. Every screen is a lookup made when you asked
          for it. Nothing is precomputed, nothing is stored between requests, and each answer
          says how old it is.
        </p>

        <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-3">
          <Frame title="reading a screen" className="lg:col-span-2">
            <FrameRows rows={READING_A_SCREEN} />
          </Frame>

          <Frame title="not here">
            <ul className="flex flex-col gap-3">
              {NOT_HERE.map((item) => (
                <li key={item.label} className="flex flex-col">
                  <span className="text-foreground">{item.label}</span>
                  <span className="text-xs text-graph-muted">{item.reason}</span>
                </li>
              ))}
            </ul>
          </Frame>
        </div>

        <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-3">
          <Frame title="the guard" className="lg:col-span-2">
            <FrameRows rows={THE_GUARD} />
          </Frame>

          <Frame title="resolvers">
            <ul className="flex flex-col gap-2">
              {RESOLVERS.map((resolver) => (
                <li key={resolver.id} className="grid grid-cols-[6rem_minmax(0,1fr)] gap-x-3">
                  <span className="text-graph-muted">{resolver.name}</span>
                  <span className="text-foreground">{resolver.ip}</span>
                </li>
              ))}
            </ul>
          </Frame>
        </div>

        <Frame title="machine readable">
          <ul className="grid grid-cols-1 gap-x-16 gap-y-2 sm:grid-cols-3">
            {MACHINE_READABLE.map((item) => (
              <AboutLink key={item.href} href={item.href} note={item.note} />
            ))}
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

function AboutLink({ href, note }: { href: string; note: string }) {
  return (
    <li>
      <Link href={href} className="group flex flex-col">
        <span className="text-foreground group-hover:text-graph-accent">{href}</span>
        <span className="text-xs text-graph-muted">{note}</span>
      </Link>
    </li>
  )
}
