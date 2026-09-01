"use client"

// The terminal viewport.
//
// Newest answer at the top. A terminal usually appends downward, but every
// screen here is a page of graphs rather than a line, so appending would put
// the thing you just asked for below a fold that keeps moving.

import { AnimatePresence, motion, useReducedMotion } from "motion/react"
import Link from "next/link"
import { useCallback, useRef, useState } from "react"

import { COMMANDS, parse, type CommandSpec, type ParseFailure } from "@/app/commands/grammar"
import { Palette } from "@/app/commands/palette"
import { Frame, FrameRows } from "@/app/screens/frame"
import { helpPayload } from "@/app/screens/help"
import { Screen } from "@/app/screens/screen"
import { ThemeToggle } from "@/components/theme-provider"
import { cacheKey, cacheSize, readCache, writeCache, type Payload } from "@/lib/cache"
import { easeOutCubic } from "@/lib/graph-motion"
import { RESOLVERS } from "@/lib/resolvers"
import { cn } from "@/lib/utils"
import { useWebMcp } from "@/lib/webmcp"

type Entry = {
  id: number
  label: string
  payload: Payload
}

type Status = "idle" | "running"

const PLACEHOLDER = "try TLS example.com, or HELP"

export default function Page() {
  const [input, setInput] = useState("")
  const [entries, setEntries] = useState<Entry[]>([])
  const [status, setStatus] = useState<Status>("idle")
  const [failure, setFailure] = useState<ParseFailure | null>(null)
  const [history, setHistory] = useState<string[]>([])
  const [cursor, setCursor] = useState(-1)
  const nextId = useRef(0)
  const reduce = useReducedMotion()

  // Nothing asked for and nothing on the way: the one state the page is allowed
  // to centre itself in.
  const resting = entries.length === 0 && status === "idle"

  const focus = useCallback(() => {
    document.querySelector<HTMLInputElement>('input[aria-label="command"]')?.focus()
  }, [])

  // Back to the landing. Answers are the only thing cleared: the history the
  // arrows walk and the cache the next query might hit both outlive the screen
  // they were made on, the same way clearing a terminal does not forget what
  // you typed into it.
  const clear = useCallback(() => {
    setEntries([])
    setFailure(null)
    focus()
  }, [focus])

  const push = useCallback((label: string, payload: Payload) => {
    nextId.current += 1
    setEntries((current) => [{ id: nextId.current, label, payload }, ...current].slice(0, 12))
  }, [])

  // One path for every caller. A WebMCP tool call runs this too, so an agent
  // and a person get the same parse, the same cache and the same screen, and
  // whatever the agent asked for is left on the page for the person to read.
  const run = useCallback(
    async (text: string): Promise<Payload | null> => {
      const result = parse(text)
      if (!result.ok) {
        setFailure(result.failure)
        return null
      }

      setFailure(null)
      const { spec, target, cacheExtra, url, label } = result.command

      if (spec.name === "HELP") {
        const payload = helpPayload()
        push(label, payload)
        return payload
      }

      const key = cacheKey(spec.name, target, cacheExtra)
      const hit = readCache(key)
      if (hit) {
        push(label, hit)
        return hit
      }

      setStatus("running")
      try {
        const response = await fetch(url, { headers: { accept: "application/json" } })
        const payload = (await response.json()) as Payload
        writeCache(key, payload)
        push(label, payload)
        return payload
      } catch (error) {
        setFailure({
          input: label,
          message: error instanceof Error ? error.message : "the request failed",
          hint: "the answer is not being shown as partial. try again",
        })
        return null
      } finally {
        setStatus("idle")
      }
    },
    [push]
  )

  useWebMcp(run)

  const submit = useCallback(async () => {
    const text = input.trim()
    if (!text || status === "running") {
      return
    }

    setHistory((current) => [text, ...current.filter((item) => item !== text)].slice(0, 50))
    setCursor(-1)

    if (await run(text)) {
      setInput("")
    }
  }, [input, run, status])

  // Shell history, only reachable when the completion list is closed. The
  // palette decides which of the two owns the arrow keys.
  const walkHistory = useCallback(
    (direction: -1 | 1) => {
      if (history.length === 0) {
        return
      }

      const next = direction === -1 ? Math.min(cursor + 1, history.length - 1) : cursor - 1
      if (next < -1) {
        return
      }

      setCursor(next)
      setInput(next >= 0 ? history[next] : "")
    },
    [cursor, history]
  )

  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-6xl flex-col px-5 py-6 sm:px-8">
      {/* The masthead is the only place the product names itself, so it carries
          the one piece of scale on the page above the prompt. Still no hero
          sentence: the prompt is the first thing you can use, the way a
          terminal opens. */}
      <header className="flex flex-wrap items-end justify-between gap-x-6 gap-y-2 pb-7">
        {/* Clickable because it is a link home on every other page, so this is
            where people already aim to get back. It is the only route to the
            landing once an answer is on screen. */}
        <h1 className="font-mono text-2xl leading-none tracking-tight lowercase">
          <button
            type="button"
            onClick={clear}
            title="back to the landing"
            className="text-foreground hover:text-graph-accent"
          >
            dug
          </button>
        </h1>
        <p className="text-xs text-muted-foreground">
          <span className="tabular-nums">{cacheSize()}</span> cached · <ThemeToggle /> ·{" "}
          <Link
            href="/developers"
            className="underline-offset-4 hover:text-foreground hover:underline"
          >
            api
          </Link>{" "}
          ·{" "}
          <Link href="/about" className="underline-offset-4 hover:text-foreground hover:underline">
            about
          </Link>
        </p>
      </header>

      {/* Before the first query this group takes the rest of the viewport and
          centres itself in it, so the prompt sits in the middle of the page
          rather than under a masthead with a screen of nothing below it. The
          moment a query starts it goes back to its natural height at the top,
          which is the only place it can be once an answer is thousands of
          pixels long. Submitting is what moves it, so the shift reads as the
          page starting rather than as a jump. */}
      <div className={cn("flex flex-col", resting && "flex-1 justify-center pb-[5vh]")}>
        <Palette
          value={input}
          onValueChange={setInput}
          onSubmit={() => void submit()}
          onHistory={walkHistory}
          onClear={clear}
          clearable={entries.length > 0}
          running={status === "running"}
          history={history}
          placeholder={PLACEHOLDER}
        />

        {/* Errors sit directly under the input that produced them. */}
        {failure ? (
          <p
            role="status"
            className={cn(
              "pt-3 text-xs text-pretty",
              failure.hint ? "text-foreground" : "text-muted-foreground"
            )}
          >
            <span className="font-mono">[!]</span> {failure.message}
            {failure.hint ? <span className="text-muted-foreground"> · {failure.hint}</span> : null}
          </p>
        ) : null}

      {/* The landing and the skeleton are the same slot: at most one is ever on
          screen, and each is replaced by the other or by an answer. They share
          one AnimatePresence so the swap is a crossfade rather than a cut.
          popLayout is the load-bearing part: it takes the leaving element out
          of flow so its replacement gets the space in the same frame. Without
          it both are mounted for the length of the exit, the page is briefly a
          thousand pixels tall, and the collapse it was meant to smooth just
          happens later. Nothing animates in; only out. */}
        <div className="relative">
          <AnimatePresence mode="popLayout" initial={false}>
            {status === "running" ? (
              <motion.div
                key="skeleton"
                className="pt-12"
                exit={{ opacity: 0 }}
                transition={{ duration: reduce ? 0 : 0.12, ease: easeOutCubic }}
              >
                <ScreenSkeleton />
              </motion.div>
            ) : entries.length === 0 ? (
              <motion.div
                key="landing"
                // Opacity alone when reduced motion is asked for, and never the
                // shared graphTransition, which returns duration 0 and would give
                // those users the abrupt swap this exists to remove.
                exit={reduce ? { opacity: 0 } : { opacity: 0, y: -8 }}
                transition={{ duration: reduce ? 0.1 : 0.16, ease: easeOutCubic }}
              >
                <Landing
                  onPick={(example) => {
                    setInput(example)
                    focus()
                  }}
                />
              </motion.div>
            ) : null}
          </AnimatePresence>
        </div>
      </div>

      {/* Only when there is something in it. An empty flex column still spends
          its pt-12, which is 48px of nothing under the landing page. */}
      {entries.length > 0 ? (
        <div className="flex flex-col gap-20 pt-12">
          {entries.map((entry) => (
            <Screen key={entry.id} payload={entry.payload} />
          ))}
        </div>
      ) : null}
    </main>
  )
}

// Shaped like the screen that is coming, so nothing shifts when it arrives:
// a verdict line, then a grid of frames.
function ScreenSkeleton() {
  return (
    <section aria-busy="true" aria-live="polite" className="flex flex-col gap-6">
      <span className="sr-only">Running the query</span>
      <div className="flex flex-col gap-2">
        <div className="h-3 w-64 rounded bg-muted" />
        <div className="h-5 w-md max-w-full rounded bg-muted" />
      </div>
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        {[3, 1, 1, 2, 1].map((span, index) => (
          <div
            key={index}
            className={cn(
              "graph-frame h-40",
              span === 3 ? "lg:col-span-3" : span === 2 ? "lg:col-span-2" : "lg:col-span-1"
            )}
          />
        ))}
      </div>
    </section>
  )
}

const STARTERS = ["TLS github.com", "MAIL github.com", "RDAP example.com", "PROP cloudflare.com"]

// The landing is built out of the same dashed frames an answer is built out of,
// so the first thing on screen is already an example of what the tool puts
// there. It used to be a bare description list, which is why it read as the
// manual for a product rather than as the product.
//
// The verbs carry no summary next to them. Twenty-one descriptions at one size
// is the flat grey wall this was, and the arithmetic does not work either: the
// longest is 48 characters, which needs about 420px beside a verb, and two of
// those do not fit inside a frame at this width without wrapping. They are one
// hover away instead, on the reserved line under the grid, and still written
// out in full by HELP and by /about.
// The grid does not use items-start, unlike an answer screen. There are three
// frames here rather than a dozen graphs, and at that count a short frame beside
// a tall one just reads as broken. Stretching is only the backstop though: the
// commands frame is laid out two families wide so its natural height already
// lands close to the column beside it.
function Landing({ onPick }: { onPick: (example: string) => void }) {
  const [hovered, setHovered] = useState<CommandSpec | null>(null)
  const families = Array.from(new Set(COMMANDS.map((spec) => spec.family)))

  return (
    <section className="grid grid-cols-1 gap-6 pt-8 lg:grid-cols-3">
      <Frame title="commands" className="lg:col-span-2">
        <div className="flex h-full flex-col justify-between gap-5">
          <div className="grid grid-cols-2 gap-x-8 gap-y-5 sm:grid-cols-3">
            {families.map((family) => (
              <div key={family} className="flex min-w-0 flex-col gap-1.5">
                <p className="text-xs tracking-wide text-graph-muted uppercase">{family}</p>

                {COMMANDS.filter((spec) => spec.family === family).map((spec) => (
                  <button
                    key={spec.name}
                    type="button"
                    onClick={() => onPick(spec.argument === "none" ? spec.name : `${spec.name} `)}
                    onMouseEnter={() => setHovered(spec)}
                    onMouseLeave={() => setHovered(null)}
                    onFocus={() => setHovered(spec)}
                    onBlur={() => setHovered(null)}
                    className="-mx-1.5 rounded px-1.5 py-0.5 text-left hover:bg-muted hover:text-graph-accent"
                  >
                    {spec.name.toLowerCase()}
                    {/* The summary belongs in the document, not only in hover
                        state. A screen reader announced these buttons as one
                        bare verb with no hint of what it does, and a client
                        that does not run javascript never saw the summaries at
                        all — twenty-one of them, which is most of what this
                        page actually says. Same text the hover line shows. */}
                    <span className="sr-only">: {spec.summary}</span>
                  </button>
                ))}
              </div>
            ))}
          </div>

          {/* Reserved rather than conditional: a line that appears on hover
              would resize the frame under the cursor and shift every verb the
              moment you point at one. */}
          <p className="min-h-5 text-xs text-graph-muted">
            {hovered ? (
              <>
                <span className="text-foreground">{hovered.example}</span> · {hovered.summary}
              </>
            ) : (
              "point at one, or type it"
            )}
          </p>
        </div>
      </Frame>

      <div className="flex flex-col gap-6">
        <Frame title="start here">
          <ul className="flex flex-col gap-1">
            {STARTERS.map((example) => {
              const [verb, ...rest] = example.split(" ")
              return (
                <li key={example}>
                  <button
                    type="button"
                    onClick={() => onPick(example)}
                    className="group -mx-1.5 flex w-full items-baseline gap-2 rounded px-1.5 py-1 text-left hover:bg-muted"
                  >
                    <span className="text-graph-accent">{verb.toLowerCase()}</span>
                    <span className="min-w-0 truncate text-graph-muted group-hover:text-foreground">
                      {rest.join(" ")}
                    </span>
                  </button>
                </li>
              )
            })}
          </ul>
        </Frame>

        {/* The list is a constant in the codebase, not something a query can
            point somewhere else, so naming it here is a fact about the tool
            rather than a boast about it.

            flex-1 so this frame, and not the gap under the column, takes up
            whatever height the commands frame beside it does not use. Which of
            the two is taller flips with the viewport, so neither can be the one
            that is allowed to come up short. */}
        <Frame title="resolvers" className="flex-1">
          <FrameRows
            rows={RESOLVERS.map((resolver) => ({ label: resolver.name, value: resolver.ip }))}
          />
        </Frame>
      </div>

      {/* Set below the frames and at footnote weight, so the prompt is still
          the first thing on the page rather than a sentence about the page.
          It is here because the landing otherwise says almost nothing in prose:
          a client that does not run javascript, which includes most crawlers
          and a good number of agents, saw twenty-two verbs and a resolver list
          and had to infer the rest. */}
      <p className="max-w-3xl text-xs leading-relaxed text-pretty text-graph-muted lg:col-span-3">
        A command driven terminal for domain and network diagnostics. Every answer is a live
        query made when you ask for it: nothing is precomputed, nothing is stored between
        requests, and each answer is labelled with how old it is. The same URLs answer in plain
        text for a terminal, JSON for a program and markdown for an agent —{" "}
        <span className="text-foreground">curl dug.sh/tls/github.com</span> needs no key and no
        signup. The grammar, the limits and the error model are written out at{" "}
        <Link href="/developers" className="text-graph-accent hover:underline">
          /developers
        </Link>
        ,{" "}
        <Link href="/llms.txt" className="text-graph-accent hover:underline">
          /llms.txt
        </Link>{" "}
        and{" "}
        <Link href="/openapi.json" className="text-graph-accent hover:underline">
          /openapi.json
        </Link>
        .
      </p>
    </section>
  )
}
