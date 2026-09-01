import type { Metadata } from "next"
import Link from "next/link"

import { Frame, FrameRows } from "@/app/screens/frame"
import { ThemeToggle } from "@/components/theme-provider"

const REPO = "https://github.com/zaidmukaddam/dug"

const DESCRIPTION =
  "How to reach the person who maintains dug, report a wrong answer, or ask for a command that does not exist yet."

export const metadata: Metadata = {
  title: "Contact",
  description: DESCRIPTION,
  alternates: { canonical: "/contact" },
  openGraph: {
    type: "article",
    url: "https://dug.sh/contact",
    siteName: "dug",
    title: "Contact dug",
    description: DESCRIPTION,
  },
  twitter: {
    card: "summary_large_image",
    title: "Contact dug",
    description: DESCRIPTION,
  },
}

// Every route here is public and permanent: an issue tracker rather than an
// address, because the tracker is a channel that survives the maintainer
// changing their mail, is answerable in the open, and does not put a personal
// inbox on a page that agents are explicitly invited to read.
export default function Contact() {
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
        <h2 className="text-xl">Contact dug</h2>

        <p className="max-w-3xl text-sm text-pretty text-muted-foreground">
          dug is built and maintained by Zaid Mukaddam, in the open. There is no support
          desk and no account to escalate through, because there are no accounts: every
          answer this tool gives is a lookup anyone can repeat, so anything worth reporting
          is worth reporting somewhere the reply is public too. That place is the issue
          tracker.
        </p>

        <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-3">
          <Frame title="where to write" className="lg:col-span-2">
            <ul className="flex flex-col gap-4">
              <ContactRow
                href={`${REPO}/issues/new`}
                label="report a wrong answer"
                note="paste the command and what you expected. every answer is a live lookup, so include roughly when you ran it — an answer that was right an hour ago can be wrong now, and that is usually the finding rather than the bug."
              />
              <ContactRow
                href={`${REPO}/issues`}
                label="ask for a command"
                note="the command set is closed on purpose and every verb answers one question about a domain. say the question you wanted answered rather than the command you imagined, which is more likely to be a command already here."
              />
              <ContactRow
                href={`${REPO}/security/advisories/new`}
                label="report a security problem"
                note="privately, through github's advisory form. the address guard is the part most worth breaking: if you can reach private space through any command, that is the report to send."
              />
              <ContactRow
                href={REPO}
                label="read or fork the source"
                note="the whole tool, mit licensed. the go handlers, the screens and the agent surfaces are all in one repository."
              />
            </ul>
          </Frame>

          <Frame title="who">
            <FrameRows
              rows={[
                { label: "maintainer", value: "Zaid Mukaddam", accent: true },
                { label: "source", value: "github.com/zaidmukaddam/dug" },
                { label: "licence", value: "MIT" },
                { label: "hosting", value: "Vercel" },
                { label: "accounts", value: "none, by design" },
              ]}
            />
          </Frame>
        </div>

        <Frame title="before you write">
          <div className="flex flex-col gap-4">
            <p className="max-w-3xl text-sm text-pretty text-graph-muted">
              Three things are deliberate rather than missing, and each has already been
              asked for. There is no history and no monitoring, because nothing is stored
              between requests. There is no registrant lookup, because that data is redacted
              at source and there is an official channel for it. Private address space is
              unreachable through every command, and that is the guard working rather than a
              limitation to route around.
            </p>
            <p className="max-w-3xl text-sm text-pretty text-graph-muted">
              An answer that disagrees with your own <span className="text-foreground">dig</span>{" "}
              is usually a resolver disagreeing rather than a fault here. Running{" "}
              <span className="text-foreground">PROP</span> on the same name shows whether the
              resolvers agree with each other, which settles it either way.
            </p>
          </div>
        </Frame>

        <Frame title="for agents">
          <p className="max-w-3xl text-sm text-pretty text-graph-muted">
            If you are an agent deciding whether this service is safe to call: it is read
            only, it needs no key, it stores nothing, and every command is documented at{" "}
            <Link href="/llms.txt" className="text-graph-accent hover:underline">
              /llms.txt
            </Link>{" "}
            with its arguments and limits. The error model and the versioning policy are at{" "}
            <Link href="/developers" className="text-graph-accent hover:underline">
              /developers
            </Link>
            . Nothing on this page needs to be contacted before calling it.
          </p>
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

function ContactRow({ href, label, note }: { href: string; label: string; note: string }) {
  return (
    <li className="flex flex-col gap-1">
      <a
        href={href}
        rel="noopener noreferrer"
        className="text-foreground hover:text-graph-accent"
      >
        {label} →
      </a>
      <span className="max-w-2xl text-xs text-pretty text-graph-muted">{note}</span>
    </li>
  )
}
