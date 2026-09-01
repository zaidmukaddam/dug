"use client"

// A case file: several screens held under the one question that produced them.
// The header is what separates evidence from a pile of unrelated answers.

import { Screen } from "@/app/screens/screen"
import type { Payload } from "@/lib/cache"
import { cn } from "@/lib/utils"

// Indexed by position in the plan. A step that does not run still occupies its
// slot, so one bad command cannot shift the results after it.
export type CaseStep =
  | { state: "done"; id: number; label: string; payload: Payload }
  | { state: "failed" }

export function CaseFile({
  question,
  target,
  steps,
  planned,
  plannedBy,
}: {
  question: string
  target: string
  steps: (CaseStep | undefined)[]
  planned: string[]
  // Who chose the steps. null is a person clicking a landing question or
  // typing WHY; the sequence was fixed and nobody planned it.
  plannedBy: "agent" | "dug" | null
}) {
  // The first slot with nothing in it yet is the one in flight.
  const pending = planned.findIndex((_, index) => !steps[index])

  return (
    <section className="flex flex-col gap-12" aria-label={`${question}: ${target}`}>
      <header className="flex flex-col gap-4">
        <p className="text-xs tracking-wide text-graph-muted uppercase">
          investigating · <span className="text-foreground">{target}</span>
          {plannedBy ? (
            <>
              {" · "}
              <span className="text-graph-accent">
                {plannedBy === "agent" ? "planned by your agent" : "planned by dug"}
              </span>
            </>
          ) : null}
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
