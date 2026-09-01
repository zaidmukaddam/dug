"use client"

// Shell-style inline completion: the suffix sits greyed ahead of the caret,
// Tab or right arrow takes it. The ghost is a span under the input in the same
// monospace font with its typed prefix hidden, so equal advance widths land the
// suffix on the caret without measuring.

import { COMMANDS, type CommandSpec } from "@/app/commands/grammar"

const ARGUMENT_HINT: Record<string, string> = {
  domain: "a domain name",
  host: "a hostname",
  endpoint: "a hostname or an ip address",
  address: "an ip address",
  asn: "an as number",
  cidr: "a network, a /24 or smaller",
  pair: "two domains",
  none: "no argument",
}

export type PaletteProps = {
  value: string
  onValueChange: (next: string) => void
  onSubmit: () => void
  onHistory: (direction: -1 | 1) => void
  onClear: () => void
  running: boolean
  // Whether there is anything on screen for escape to clear, so the hint only
  // names the key while it does something. An advertised shortcut that does
  // nothing is how the dark mode hotkey read for months.
  clearable: boolean
  // How many WebMCP tools are registered, or 0 where the API is unavailable.
  agentTools: number
  history: string[]
  placeholder: string
}

function specFor(value: string): CommandSpec | null {
  const head = value.trim().split(/\s+/)[0]?.toUpperCase() ?? ""
  return COMMANDS.find((spec) => spec.name === head) ?? null
}

// Completes the verb before one is chosen, then the target from history.
function completionFor(value: string, history: string[]): string {
  if (value === "") {
    return ""
  }

  const spaced = /\s/.test(value)

  if (!spaced) {
    const typed = value.toUpperCase()
    const match = COMMANDS.find(
      (spec) => spec.name.startsWith(typed) && spec.name !== typed
    )
    if (match) {
      return match.name.slice(value.length).toLowerCase()
    }
    // An exactly typed verb still needs its space before an argument.
    const exact = COMMANDS.find((spec) => spec.name === typed)
    return exact && exact.argument !== "none" ? " " : ""
  }

  const previous = history.find(
    (entry) => entry.toLowerCase().startsWith(value.toLowerCase()) && entry.length > value.length
  )
  if (previous) {
    return previous.slice(value.length)
  }

  // Nothing typed after the verb yet, so offer the command's own example.
  const spec = specFor(value)
  if (spec && value.trimEnd().length === spec.name.length) {
    return spec.example.slice(spec.name.length + 1)
  }

  return ""
}

export function Palette({
  value,
  onValueChange,
  onSubmit,
  onHistory,
  onClear,
  running,
  clearable,
  agentTools,
  history,
  placeholder,
}: PaletteProps) {
  const completion = running ? "" : completionFor(value, history)
  const spec = specFor(value)

  function accept() {
    if (completion) {
      onValueChange(value + completion)
    }
  }

  return (
    <div className="flex flex-col gap-2">
      {/* The prompt is the only control on the page, so it is the only thing
          above the fold allowed a size of its own. Ghost, caret and input all
          have to carry the same one: the completion is positioned by matching
          advance widths rather than by measuring, so a caret at a different
          size would land the suffix off the cursor. */}
      <div className="graph-frame px-4 py-5 sm:px-6">
        <div className="flex items-baseline gap-3">
          <span
            aria-hidden="true"
            className="shrink-0 text-base text-graph-accent select-none sm:text-lg"
          >
            {running ? "..." : ">"}
          </span>

          {/* Ghost and input share one grid cell so they share a baseline. */}
          <div className="grid min-w-0 flex-1">
            <div
              aria-hidden="true"
              className="pointer-events-none col-start-1 row-start-1 overflow-hidden font-mono text-base whitespace-pre sm:text-lg"
            >
              {/* Zero width space keeps a baseline when the input is empty. */}
              <span className="invisible">{value || "​"}</span>
              <span className="text-muted-foreground/70">{completion}</span>
            </div>

            {/* Stays enabled while a query runs. Disabling it would block
                typing and pasting the next command for as long as the lookup
                takes, which on a PROP fan-out is seconds. Submission is
                gated in the Enter handler instead. */}
            <input
              value={value}
              onChange={(event) => onValueChange(event.target.value)}
              spellCheck={false}
              autoComplete="off"
              autoCapitalize="off"
              autoCorrect="off"
              // biome-ignore lint/a11y/noAutofocus: the page is one prompt and this is its only control, so a screen reader lands on the thing to type into
              autoFocus
              aria-label="command"
              aria-describedby="command-hint"
              placeholder={value ? undefined : placeholder}
              className="col-start-1 row-start-1 w-full bg-transparent font-mono text-base outline-none placeholder:text-muted-foreground sm:text-lg"
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  event.preventDefault()
                  onSubmit()
                  return
                }

                // Escape clears the line, and clears the screen once the line
                // is already empty. Two things on one key, but in that order
                // it is the same escalation a shell has: you back out of what
                // you are typing first, and only then out of what you ran.
                if (event.key === "Escape") {
                  event.preventDefault()
                  if (value) {
                    onValueChange("")
                  } else {
                    onClear()
                  }
                  return
                }

                if (event.key === "Tab" && completion) {
                  event.preventDefault()
                  accept()
                  return
                }

                // Right arrow only completes from the end of the line, so it
                // still moves the caret normally mid-edit.
                if (
                  event.key === "ArrowRight" &&
                  completion &&
                  event.currentTarget.selectionStart === value.length
                ) {
                  event.preventDefault()
                  accept()
                  return
                }

                if (event.key === "ArrowUp" || event.key === "ArrowDown") {
                  event.preventDefault()
                  onHistory(event.key === "ArrowUp" ? -1 : 1)
                }
              }}
            />
          </div>
        </div>
      </div>

      <p id="command-hint" className="px-1 text-xs text-pretty text-muted-foreground">
        {spec ? (
          <>
            <span className="font-mono text-foreground">{spec.name.toLowerCase()}</span>{" "}
            {ARGUMENT_HINT[spec.argument]} · {spec.summary}
          </>
        ) : (
          <>
            <span className="tabular-nums">{COMMANDS.length}</span> commands · tab completes
            {/* History is per page load, like the cache, so on a fresh page the
                arrows do nothing. Claiming otherwise makes a working feature
                look broken the one time most people try it. */}
            {history.length > 0 ? " · up and down walk history" : null}
            {clearable ? " · esc clears the screen" : null}
            {/* What is on the page for an agent, said where a person will see
                it. Deliberately "available to" and not "an agent is here": the
                tools are registered either way, and only the first is known. */}
            {agentTools > 0 ? (
              <>
                {" · "}
                <span className="text-graph-accent">
                  <span className="tabular-nums">{agentTools}</span> tools available to an agent in
                  this page
                </span>
              </>
            ) : null}
            {" · "}
            <span className="font-mono">why mail example.com</span> investigates · or ask in
            plain words
          </>
        )}
      </p>
    </div>
  )
}
