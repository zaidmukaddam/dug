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
import { CaseFile, type CaseStep } from "@/app/screens/case"
import { Frame, FrameRows } from "@/app/screens/frame"
import { helpPayload } from "@/app/screens/help"
import { Screen } from "@/app/screens/screen"
import { ThemeToggle } from "@/components/theme-provider"
import { useMountEffect } from "@/hooks/use-mount-effect"
import { cacheKey, cacheSize, readCache, writeCache, type Payload } from "@/lib/cache"
import { easeOutCubic } from "@/lib/graph-motion"
import { plan, type PlanOutcome } from "@/app/plan"
import {
  expandSteps,
  INVESTIGATIONS,
  type Investigation,
  matchInvestigation,
} from "@/lib/investigations"
import { RESOLVERS } from "@/lib/resolvers"
import { cn } from "@/lib/utils"
import { useWebMcp } from "@/lib/webmcp"

// One answer, or one question that took several.
//
// A case is created before its first lookup returns and filled in as they
// arrive, so the person watches the evidence assemble rather than waiting on a
// spinner and being handed a finished page.
// Who asked. Rendered on the answer, because a page two people are using at
// once should say which of them a screen came from. "dug" is the page's own
// planner, standing in for an agent that is not there.
type Source = "you" | "agent" | "dug"

type Entry =
  | { kind: "screen"; id: number; label: string; payload: Payload; source: Source }
  | {
      kind: "case"
      id: number
      question: string
      targets: string[]
      planned: string[]
      source: Source
      // Sparse until the plan finishes: index n is the nth planned command.
      steps: (CaseStep | undefined)[]
    }

// Which case, and which step of it, a result belongs to.
type Slot = { caseId: number; index: number }

type Status = "idle" | "running"

// A command, resolved as far as it can be without the network. Separating this
// from performing it is what lets a caller know whether a fetch is coming
// before it commits to showing a skeleton for one.
type Lookup =
  | { kind: "failed"; failure: ParseFailure }
  | { kind: "ready"; label: string; payload: Payload }
  | { kind: "fetch"; label: string; url: string; key: string }

type Outcome =
  | { ok: true; label: string; payload: Payload }
  | { ok: false; failure: ParseFailure }

function lookup(text: string): Lookup {
  const result = parse(text)
  if (!result.ok) {
    return { kind: "failed", failure: result.failure }
  }

  const { spec, target, cacheExtra, url, label } = result.command
  if (spec.name === "HELP") {
    return { kind: "ready", label, payload: helpPayload() }
  }

  const key = cacheKey(spec.name, target, cacheExtra)
  const hit = readCache(key)
  if (hit) {
    return { kind: "ready", label, payload: hit }
  }
  return { kind: "fetch", label, url, key }
}

async function perform(step: Lookup): Promise<Outcome> {
  if (step.kind === "failed") {
    return { ok: false, failure: step.failure }
  }
  if (step.kind === "ready") {
    return { ok: true, label: step.label, payload: step.payload }
  }

  try {
    const response = await fetch(step.url, { headers: { accept: "application/json" } })
    const payload = (await response.json()) as Payload
    // A refusal and a rate limit arrive as the same envelope with error set and
    // a 4xx status. Neither is an answer: it is not cached, not pushed as a
    // screen, and an agent calling through a tool sees it as the failure it is.
    if (!response.ok || payload.error) {
      return {
        ok: false,
        failure: {
          input: step.label,
          message: payload.error?.message ?? `the api answered ${response.status}`,
          hint: payload.error?.hint,
        },
      }
    }
    writeCache(step.key, payload)
    return { ok: true, label: step.label, payload }
  } catch (error) {
    return {
      ok: false,
      failure: {
        input: step.label,
        message: error instanceof Error ? error.message : "the request failed",
        hint: "the answer isn’t being shown as partial. try again",
      },
    }
  }
}

const PLACEHOLDER = "try TLS example.com, HELP, or ask in plain words"
// About thirty characters is what a phone shows of the prompt before the
// frame cuts it off, so the narrow form keeps the command and the invitation
// and drops the rest.
const PLACEHOLDER_NARROW = "try TLS example.com, or ask"

const VERBS = new Set<string>(COMMANDS.map((spec) => spec.name))

// Three or more words that do not open with a command. Short input is a typo
// until proven otherwise, and a typo should cost a parse error, not a model.
function looksLikeAQuestion(text: string): boolean {
  const words = text.split(/\s+/)
  return words.length >= 3 && !VERBS.has(words[0].toUpperCase())
}

// The action handles every outcome it can name. What it cannot is the transport
// itself failing, which after a deployment includes the client holding an
// action id the server no longer has. That surfaces as a sentence under the
// prompt with a command to fall back on, and a reload recovers it.
async function askPlanner(ask: string): Promise<PlanOutcome> {
  try {
    return await plan(ask)
  } catch {
    return {
      ok: false,
      failure: {
        input: ask,
        message: "the planner could not be reached",
        hint: "a command still works: TLS example.com, or WHY mail example.com",
      },
    }
  }
}

export default function Page() {
  const [input, setInput] = useState("")
  const [entries, setEntries] = useState<Entry[]>([])
  // A counter rather than a boolean: overlapping callers of run() each own
  // their increment and decrement, so the first to finish does not clear the
  // skeleton out from under the one still in flight.
  const [inFlight, setInFlight] = useState(0)
  const status: Status = inFlight > 0 ? "running" : "idle"
  const [failure, setFailure] = useState<ParseFailure | null>(null)
  const [history, setHistory] = useState<string[]>([])
  const [cursor, setCursor] = useState(-1)
  const nextId = useRef(0)
  const submitting = useRef(false)
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

  const push = useCallback((label: string, payload: Payload, source: Source) => {
    nextId.current += 1
    setEntries((current) => [
      { kind: "screen" as const, id: nextId.current, label, payload, source },
      ...current,
    ].slice(0, 12))
  }, [])

  // A lookup landing in its own slot inside an open case rather than beside it.
  const pushInto = useCallback((slot: Slot, step: CaseStep) => {
    setEntries((current) =>
      current.map((entry) => {
        if (entry.kind !== "case" || entry.id !== slot.caseId) {
          return entry
        }
        const steps = entry.steps.slice()
        steps[slot.index] = step
        return { ...entry, steps }
      })
    )
  }, [])

  // One command, onto the page. A WebMCP tool call runs this too, so an agent
  // and a person get the same parse, the same cache and the same screen.
  //
  // The skeleton is raised only for a step that will actually fetch, which is
  // why lookup and perform are separate: a cache hit resolves in a microtask
  // and would otherwise show a frame of skeleton on its way past.
  const run = useCallback(
    async (text: string, source: Source = "you"): Promise<Payload | null> => {
      const step = lookup(text)
      const fetching = step.kind === "fetch"
      if (fetching) {
        setInFlight((inFlight) => inFlight + 1)
      }

      try {
        const outcome = await perform(step)
        if (!outcome.ok) {
          setFailure(outcome.failure)
          return null
        }
        setFailure(null)
        push(outcome.label, outcome.payload, source)
        return outcome.payload
      } finally {
        if (fetching) {
          setInFlight((inFlight) => inFlight - 1)
        }
      }
    },
    [push]
  )

  // One question, several lookups, one case file.
  //
  // Sequential rather than parallel on purpose: each screen appears as it
  // lands, so the first is readable while the fourth is still in flight. That
  // is the reason this renders onto a page instead of returning a summary.
  const investigate = useCallback(
    async (asked: string, targets: string[], steps: string[], source: Source = "agent") => {
      // The case adds the question mark, so one an agent already wrote is
      // dropped: "landing in spam??" was on screen for a whole demo take.
      const question = asked.replace(/[?\s]+$/, "")
      const planned = expandSteps(steps, targets)
      nextId.current += 1
      const caseId = nextId.current
      setEntries((current) => [
        { kind: "case" as const, id: caseId, question, targets, planned, source, steps: [] },
        ...current,
      ].slice(0, 12))

      setFailure(null)

      // No status here: a case draws its own progress, and the shared skeleton
      // would flash between every step. A step that fails keeps its slot so the
      // plan can strike that command out without shifting the ones after it.
      const found: { command: string; payload: Payload }[] = []
      for (const [index, command] of planned.entries()) {
        const outcome = await perform(lookup(command))
        if (!outcome.ok) {
          pushInto({ caseId, index }, { state: "failed" })
          continue
        }
        nextId.current += 1
        pushInto(
          { caseId, index },
          { state: "done", id: nextId.current, label: outcome.label, payload: outcome.payload }
        )
        found.push({ command, payload: outcome.payload })
      }
      return { planned, found }
    },
    [pushInto]
  )

  // Every tool call arrives already labelled, so an answer on screen says who
  // asked for it without run() having to guess.
  const runForAgent = useCallback((text: string) => run(text, "agent"), [run])
  const webmcp = useWebMcp(runForAgent, investigate)

  // A shared link. The proxy redirects dug.sh/tls/github.com opened in a
  // browser to /?run=TLS+github.com, and this runs it as if it had been typed.
  // The query is cleared first, so a reload after the answer is the landing
  // and the second mount React performs in development finds nothing to run.
  useMountEffect(() => {
    const line = new URLSearchParams(window.location.search).get("run")?.trim()
    if (!line) {
      return
    }
    window.history.replaceState(null, "", "/")
    void run(line)
  })

  const submit = useCallback(async () => {
    const text = input.trim()
    if (!text || submitting.current) {
      return
    }
    submitting.current = true
    try {
      setHistory((current) => [text, ...current.filter((item) => item !== text)].slice(0, 50))
      setCursor(-1)

      // WHY is answered here rather than by parse, because it is not a command:
      // no endpoint returns several screens, so there is nothing for the grammar
      // to route it to.
      const why = matchInvestigation(text)
      if (why?.ok) {
        setInput("")
        setFailure(null)
        await investigate(why.investigation.question, why.targets, why.investigation.steps, "you")
        return
      }

      // A WHY that did not match is one of two things. Three words or fewer is
      // the keyword form with a typo, and the topic error is the useful answer.
      // Longer is a sentence that happens to start with "why", and it belongs to
      // the planner: "why is mail from acme.com going to spam" is not a request
      // for an investigation called "is".
      if (why && text.split(/\s+/).length <= 3) {
        setFailure(why.failure)
        return
      }

      // Plain words go to the planner. Anything that starts with a command, or is
      // too short to be a sentence, still goes through parse so a typo gets the
      // normal error rather than a model call.
      if (looksLikeAQuestion(text)) {
        setInFlight((inFlight) => inFlight + 1)
        // Tracks whether the increment above has already been matched by a
        // decrement, so the finally below does not double-decrement on the
        // success path, which turns the shared skeleton off early (see the
        // comment on investigate()).
        let settled = false
        setFailure(null)
        try {
          const plan = await askPlanner(text)
          if (!plan.ok) {
            setFailure(plan.failure)
            return
          }
          setInput("")
          setInFlight((inFlight) => inFlight - 1)
          settled = true
          await investigate(plan.question, plan.targets, plan.steps, "dug")
        } finally {
          if (!settled) {
            setInFlight((inFlight) => inFlight - 1)
          }
        }
        return
      }

      if (await run(text)) {
        setInput("")
      }
    } finally {
      submitting.current = false
    }
  }, [input, investigate, run])

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
          agentTools={webmcp.state === "registered" ? webmcp.tools : 0}
          running={status === "running"}
          history={history}
          placeholder={PLACEHOLDER}
          placeholderNarrow={PLACEHOLDER_NARROW}
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

      {/* The landing and the skeleton share this slot; at most one is on screen.

          Only the skeleton animates out, because it is replaced by the answer it
          stood in for. The landing must not: popLayout takes a leaving element
          out of flow, and an exit that does not complete leaves the landing at
          position absolute and full opacity over the answer underneath. */}
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
            ) : null}
          </AnimatePresence>

          {resting ? (
            <Landing
              onPick={(example) => {
                setInput(example)
                focus()
              }}
              onInvestigate={(investigation) => {
                void investigate(
                  investigation.question,
                  [investigation.sample],
                  investigation.steps,
                  "you"
                )
              }}
            />
          ) : null}
        </div>
      </div>

      {/* Only when there is something in it. An empty flex column still spends
          its pt-12, which is 48px of nothing under the landing page. */}
      {entries.length > 0 ? (
        <div className="flex flex-col gap-20 pt-12">
          {entries.map((entry) =>
            entry.kind === "case" ? (
              <CaseFile
                key={entry.id}
                question={entry.question}
                targets={entry.targets}
                steps={entry.steps}
                planned={entry.planned}
                plannedBy={entry.source === "you" ? null : entry.source}
              />
            ) : (
              <div key={entry.id} className="flex flex-col gap-3">
                {entry.source === "agent" ? <AskedByAgent /> : null}
                <Screen payload={entry.payload} />
              </div>
            )
          )}
        </div>
      ) : null}
    </main>
  )
}

// Two people are using this page and only one of them typed. Marking the
// answers an agent asked for is what makes that visible instead of implied.
export function AskedByAgent() {
  return (
    <p className="flex items-center gap-2 text-xs tracking-wide text-graph-accent uppercase">
      <span aria-hidden="true">[~]</span> asked by your agent
    </p>
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

// Built out of the same dashed frames an answer is, so the first thing on
// screen is already an example of what the tool puts there.
//
// The verbs carry no summary beside them: the longest needs about 420px, which
// does not fit two-up at this width. They are one hover away on the reserved
// line under the grid, and written out in full by HELP and /about.
//
// No items-start, unlike an answer screen — with three frames rather than a
// dozen, a short one beside a tall one reads as broken.
function Landing({
  onPick,
  onInvestigate,
}: {
  onPick: (example: string) => void
  onInvestigate: (investigation: Investigation) => void
}) {
  const [hovered, setHovered] = useState<CommandSpec | null>(null)
  const families = Array.from(new Set(COMMANDS.map((spec) => spec.family)))

  return (
    <section className="grid grid-cols-1 gap-6 pt-8 lg:grid-cols-3">
      {/* First, and full width, because it is the thing a person arrives with.
          Nobody debugging mail knows they need four lookups; they know the
          symptom. Every command below answers one question, and these are the
          questions people actually have. */}
      <Frame title="investigations" className="lg:col-span-3">
        <div className="flex flex-col gap-5">
          <div className="grid grid-cols-1 gap-x-8 gap-y-3 sm:grid-cols-2 lg:grid-cols-3">
            {INVESTIGATIONS.map((investigation) => {
              return (
                <button
                  key={investigation.id}
                  type="button"
                  onClick={() => onInvestigate(investigation)}
                  className="group -mx-1.5 flex min-w-0 flex-col gap-1 rounded px-1.5 py-1 text-left hover:bg-muted"
                >
                  <span className="text-pretty text-foreground group-hover:text-graph-accent">
                    {investigation.question}?
                  </span>
                  {/* The command that runs it, so clicking teaches the syntax
                      for running the same thing against your own domain. */}
                  <span className="truncate text-xs text-graph-muted">
                    <span className="text-graph-accent">why {investigation.id}</span>{" "}
                    {investigation.sample} ·{" "}
                    {investigation.steps.map((step) => step.split(" ")[0].toLowerCase()).join(" ")}
                  </span>
                </button>
              )
            })}
          </div>

          {/* Two sentences, on purpose. The tools are registered in any
              browser and nothing on the page can tell whether an agent is
              listening, so this says where to get one and stops. */}
          <p className="max-w-3xl text-xs text-pretty text-graph-muted">
            Click one, or type <span className="text-foreground">why mail yourdomain.com</span>.
            An agent in this page plans its own. No agent here? Open this page in{" "}
            <span className="text-foreground">ChatGPT’s browser</span>,{" "}
            <span className="text-foreground">Codex</span> or{" "}
            <span className="text-foreground">Chrome with WebMCP enabled</span>, or call{" "}
            <Link href="/mcp" className="text-graph-accent hover:underline">
              /mcp
            </Link>{" "}
            from outside one.
          </p>
        </div>
      </Frame>

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
                    {/* In the document, not only in hover state: otherwise a
                        screen reader gets a bare verb, and a client running no
                        javascript never sees the summaries at all. */}
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
        Live domain and network diagnostics. Every answer is a fresh lookup that says how old it
        is, and the same URLs answer in text for a terminal, JSON for a program and markdown for
        an agent: <span className="text-foreground">curl dug.sh/tls/github.com</span> needs no
        key. Details at{" "}
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
