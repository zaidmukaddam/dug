// One question, several lookups, one case file.
//
// Every command answers one question about one target, which suits someone who
// already knows which question to ask. Nobody debugging mail knows to ask four.
// They know the symptom, and the expertise is knowing which lookups follow from
// it — which is why the plan is not stored here. An agent writes it and the page
// runs it; the list below is only the landing's worked examples, for someone who
// arrived without one.

import type { ParseFailure } from "@/app/commands/grammar"

export type Investigation = {
  id: string
  question: string
  // In the order someone who knew what they were doing would run them. A case
  // reads top to bottom.
  steps: (target: string) => string[]
}

// `WHY <topic> <target> [, <target>...]` at the prompt, so a person can run one
// against their own domain, or several at once, rather than only against the
// landing's examples.
//
// Handled before parse() rather than added to the command grammar, because it
// is not a command: there is no endpoint that could answer it. One request
// returns one screen, and an investigation is several — which is the same
// reason it is not on the MCP server either.
// null means this is not a WHY at all and belongs to parse(). A failure means it
// is one and is wrong, which has to be said here: falling through would report
// "why is not a command" for a mistyped topic, when why is exactly what it is.
export type InvestigationMatch =
  | { ok: true; investigation: Investigation; targets: string[] }
  | { ok: false; failure: ParseFailure }

export function matchInvestigation(text: string): InvestigationMatch | null {
  const trimmed = text.trim()
  const [verb, topic, ...rest] = trimmed.split(/\s+/)
  if (verb?.toUpperCase() !== "WHY") {
    return null
  }

  const topics = INVESTIGATIONS.map((entry) => entry.id).join(", ")
  const investigation = INVESTIGATIONS.find((entry) => entry.id === topic?.toLowerCase())
  if (!investigation) {
    return {
      ok: false,
      failure: {
        input: trimmed,
        message: topic ? `there’s no ${topic} investigation` : "why needs something to investigate",
        hint: `try one of ${topics}, as in WHY mail example.com`,
      },
    }
  }

  const targets = rest
    .join(" ")
    .split(/[\s,]+/)
    .map((target) => target.trim())
    .filter(Boolean)
  if (!targets.length) {
    return {
      ok: false,
      failure: {
        input: trimmed,
        message: `why ${investigation.id} needs a domain`,
        hint: `WHY ${investigation.id} example.com`,
      },
    }
  }
  return { ok: true, investigation, targets }
}

// A step written with {target} runs once per target, in target order, so a
// fleet reads as one case per domain rather than one command across all of
// them. A step without the placeholder runs once as written.
export function expandSteps(steps: string[], targets: string[]): string[] {
  if (!steps.some((step) => step.includes("{target}"))) {
    return steps
  }
  return targets.flatMap((target) =>
    steps.map((step) => step.replaceAll("{target}", target))
  )
}

export const INVESTIGATIONS: Investigation[] = [
  {
    id: "mail",
    question: "why is mail from this domain going to spam",
    steps: (target) => [`MAIL ${target}`, `SPF ${target}`, `DIG ${target} MX`, `RDAP ${target}`],
  },
  {
    id: "dns",
    question: "has my dns change taken effect",
    steps: (target) => [`PROP ${target}`, `TTL ${target}`, `NS ${target}`, `DNSSEC ${target}`],
  },
  {
    id: "tls",
    question: "is this site’s certificate and transport healthy",
    steps: (target) => [`TLS ${target}`, `WATCH ${target}`, `HTTP ${target}`, `TRACE ${target}`],
  },
  {
    id: "reachability",
    question: "why is this host slow or unreachable",
    steps: (target) => [`PING ${target}`, `ROUTE ${target}`, `TRACE ${target}`],
  },
  {
    id: "agents",
    question: "can an agent read and understand this site",
    steps: (target) => [`SEO ${target}`, `AEO ${target}`, `WEBMCP ${target}`, `OG ${target}`],
  },
]
