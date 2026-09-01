// The shape every non-answer page takes.
//
// A diagnostics tool that renders a verdict and its evidence should not switch
// to a different visual language the moment it has bad news. So a 404 and a
// crash are rendered as screens too: the same masthead, the same verdict line
// and glyph, the same dashed frames. The only difference is that the evidence
// is about the request rather than about a domain.

import Link from "next/link"

import { Frame, FrameRows } from "@/app/screens/frame"
import { ThemeToggle } from "@/components/theme-provider"

export type NoticeRow = {
  label: string
  value: string
  accent?: boolean
}

export type NoticeLink = {
  href: string
  label: string
  note?: string
}

export function Notice({
  headline,
  detail,
  title,
  rows,
  links,
  children,
}: {
  headline: string
  detail?: string
  title: string
  rows: NoticeRow[]
  links?: NoticeLink[]
  children?: React.ReactNode
}) {
  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-6xl flex-col px-5 py-6 sm:px-8">
      <header className="flex flex-wrap items-end justify-between gap-x-6 gap-y-2">
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

      <section className="flex flex-col gap-8 pt-14">
        <div className="flex flex-col gap-1.5">
          <p className="flex items-baseline gap-3 text-xl leading-tight text-balance">
            <span aria-hidden="true" className="shrink-0 font-mono text-sm text-muted-foreground">
              [!]
            </span>
            <span>{headline}</span>
          </p>

          {detail ? (
            <p className="max-w-3xl pl-9 text-sm text-pretty text-muted-foreground">
              {detail}
            </p>
          ) : null}
        </div>

        <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-3">
          <Frame title={title} className="lg:col-span-2">
            <FrameRows rows={rows} />
          </Frame>

          {links && links.length > 0 ? (
            <Frame title="elsewhere">
              <ul className="flex flex-col gap-2">
                {links.map((link) => (
                  <li key={link.href}>
                    <Link href={link.href} className="group flex flex-col">
                      <span className="text-foreground group-hover:text-graph-accent">
                        {link.label}
                      </span>
                      {link.note ? (
                        <span className="text-xs text-graph-muted">{link.note}</span>
                      ) : null}
                    </Link>
                  </li>
                ))}
              </ul>
            </Frame>
          ) : null}
        </div>

        {children}
      </section>
    </main>
  )
}
