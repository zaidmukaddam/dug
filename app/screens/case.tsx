"use client"

// A case file: several screens held under the one question that produced them.
//
// Without this an investigation is four unrelated screens in reverse order, and
// the person reading them has to remember that they belong together and that
// the last one is the first thing that ran. The header is not decoration; it is
// the difference between evidence and a pile.

import { Screen } from "@/app/screens/screen"
import type { Payload } from "@/lib/cache"
import { cn } from "@/lib/utils"

// Indexed by position in the plan, not appended in completion order.
//
// Appending meant a step that did not run left no gap: the fourth command's
// result landed in the third slot, so a plan with one bad command in it
// reported the wrong command as broken and the one after it as still running.
export type CaseStep =
  | { state: "done"; id: number; label: string; payload: Payload }
  | { state: "failed" }

export function CaseFile({
  question,
  target,
  steps,
  planned,
}: {
  question: string
  target: string
  steps: (CaseStep | undefined)[]
  planned: string[]
}) {
  // The first slot with nothing in it yet is the one in flight.
  const pending = planned.findIndex((_, index) => !steps[index])

  return (
    <section className="flex flex-col gap-12" aria-label={`${question}: ${target}`}>
      <header className="flex flex-col gap-4">
        <p className="text-xs tracking-wide text-graph-muted uppercase">
          investigating · <span className="text-foreground">{target}</span>
        </p>
        <h2 className="text-xl text-pretty text-foreground lowercase">{question}?</h2>

        {/* The plan, stated before it finishes. A person watching four lookups
            arrive one at a time should be able to see what is still coming,
            otherwise a slow step is indistinguishable from a finished case. */}
        <ol className="flex flex-wrap items-center gap-x-2 gap-y-1 font-mono text-xs">
          {planned.map((command, index) => {
            const step = steps[index]
            const state = step?.state ?? (index === pending ? "running" : "waiting")

            return (
              <li key={`${index}-${command}`} className="flex items-center gap-2">
                {index > 0 ? <span className="text-graph-frame">·</span> : null}
                <span
                  className={cn(
                    state === "done" && "text-graph-accent",
                    state === "running" && "text-foreground",
                    state === "failed" && "text-graph-muted line-through",
                    state === "waiting" && "text-graph-muted"
                  )}
                  title={state === "failed" ? "this command did not run" : undefined}
                >
                  {MARK[state]} {command}
                </span>
              </li>
            )
          })}
        </ol>
      </header>

      {steps.map((step) =>
        step?.state === "done" ? <Screen key={step.id} payload={step.payload} /> : null
      )}
    </section>
  )
}

const MARK: Record<string, string> = {
  done: "[x]",
  running: "[~]",
  failed: "[!]",
  waiting: "[ ]",
}
