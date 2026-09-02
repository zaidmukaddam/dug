"use client"

// WebMCP: the same commands, offered to an agent running inside the page.
//
// /api/mcp answers an agent that is somewhere else. This is for one already
// here, and the difference is that a call lands on the page: every tool runs
// the ordinary command path, so the answer appears on screen where the person
// can read the evidence themselves.
//
// The API has moved twice, so the three details that matter:
//
//   entry point   document.modelContext. navigator.modelContext is a deprecated
//                 alias as of Chrome 150 and may be absent
//   registration  registerTool(tool, { signal }), one at a time. provideContext
//                 declared the whole list at once and was removed — on a page
//                 where two scripts register tools, replace-all is takeover
//   removal       abort the signal given at registration
//
// @mcp-b/global installs the API where the browser has not shipped it and wraps
// the native one where it has. Native additionally needs an origin-isolated
// document, which is why next.config sends Origin-Agent-Cluster: ?1.

import "@mcp-b/global"

import { useEffectEvent, useState } from "react"

import { COMMANDS } from "@/app/commands/grammar"
import { useMountEffect } from "@/hooks/use-mount-effect"
import type { Payload } from "@/lib/cache"
import { expandSteps, INVESTIGATIONS } from "@/lib/investigations"

type ToolDescriptor = {
  name: string
  description: string
  inputSchema: {
    type: "object"
    properties: Record<string, { type: string; description: string; items?: { type: string } }>
    required: string[]
  }
  execute: (input: Record<string, string>) => Promise<unknown>
  annotations?: { readOnlyHint?: boolean; untrustedContentHint?: boolean }
}

// registerTool is a promise in every implementation worth supporting, and the
// second argument carries the AbortSignal that removes the tool again.
type ModelContext = {
  registerTool: (tool: ToolDescriptor, options?: { signal?: AbortSignal }) => unknown
  getTools?: () => unknown
}

// Swallows a failure whether it arrives as a throw or as a rejected promise,
// and reports whether it worked. Registration is best-effort — a draft API on
// an unknown browser must never take the page down, and an unhandled rejection
// surfaces as a runtime error — but silently best-effort is how this ended up
// being described as working when it registered nothing at all.
function attempt(call: () => unknown): Promise<boolean> {
  try {
    return Promise.resolve(call()).then(
      () => true,
      () => false
    )
  } catch {
    return Promise.resolve(false)
  }
}

// The outcome, left on the root element as data-webmcp.
//
// navigator.modelContext is a draft that almost no browser ships yet, so the
// ordinary case is that nothing registers. Without this an agent looking for
// page level tools finds none and cannot tell whether the browser lacks the
// API, the registration failed, or the page never tried — three very different
// things, one of which is a bug and two of which are not. It also says where
// the tools actually are.
type WebMcpState = "unsupported" | "registered" | "failed"

// How long to keep asking getTools whether the set is up before calling it a
// failure, and how often. Generous, because the cost of waiting is a marker
// that appears late and the cost of giving up early is one that is wrong.
const SETTLE_BUDGET_MS = 5000
const SETTLE_STEP_MS = 100

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

// The names currently registered, or null where the implementation offers no
// way to ask.
async function present(context: ModelContext): Promise<Set<string> | null> {
  if (typeof context.getTools !== "function") {
    return null
  }
  try {
    const listed = await context.getTools()
    if (!Array.isArray(listed)) {
      return null
    }
    return new Set(
      listed
        .map((tool) => (tool as { name?: unknown })?.name)
        .filter((name): name is string => typeof name === "string")
    )
  } catch {
    return null
  }
}

function mark(state: WebMcpState) {
  if (typeof document === "undefined") {
    return
  }
  document.documentElement.dataset.webmcp = state
  // The short path, which is what llms.txt, the catalog and server.json all
  // name. /api/mcp is the same endpoint and still answers.
  document.documentElement.dataset.webmcpServer = "/mcp"
}

// document.modelContext is canonical. navigator.modelContext is the deprecated
// alias and is read only as a fallback, for a browser old enough to have the
// early shape but not the current one.
function modelContext(): ModelContext | null {
  if (typeof document !== "undefined") {
    const onDocument = (document as Document & { modelContext?: ModelContext }).modelContext
    if (onDocument) {
      return onDocument
    }
  }
  if (typeof navigator !== "undefined") {
    return (navigator as Navigator & { modelContext?: ModelContext }).modelContext ?? null
  }
  return null
}

const ARGUMENT_HINT: Record<string, string> = {
  domain: "a domain name",
  host: "a hostname",
  endpoint: "a hostname or an ip address",
  address: "an ip address",
  asn: "an as number, with or without the AS prefix",
  cidr: "a network in cidr form, a /24 or smaller",
  pair: "a domain name, compared against `other`",
}

// The second argument some commands take, by the same name the query form uses.
const SECOND: Record<string, { name: string; about: string; required: boolean }> = {
  DIG: { name: "type", about: "a single record type to ask for, such as MX", required: false },
  PORTS: { name: "ports", about: "ports to try, comma separated, ranges allowed", required: false },
  PING: { name: "count", about: "how many echoes to send, 1 to 10", required: false },
  VS: { name: "other", about: "the second domain name", required: true },
}

function describe(payload: Payload): string {
  const lines = [`${payload.command} ${payload.target}`, payload.verdict.headline]
  if (payload.verdict.detail) {
    lines.push(payload.verdict.detail)
  }
  for (const entry of payload.degraded) {
    lines.push(`degraded: ${entry.source}, ${entry.reason}`)
  }
  return lines.join("\n")
}

// A worked example for the tool description, taken from the landing's own
// presets so the two cannot describe different grammars.
function example(id: string): string {
  const investigation = INVESTIGATIONS.find((entry) => entry.id === id)
  if (!investigation) {
    return ""
  }
  return `"${investigation.question}" with steps ${JSON.stringify(investigation.steps("example.com"))}`
}

// What the page is allowed to say about the tools it put up.
//
// "registered" means the tools are on the page and callable. It does not mean
// an agent is reading them, and the page must not claim otherwise: @mcp-b/global
// installs the API in an ordinary browser too, so this is true whether or not
// anyone is listening. The honest claim is what is available, not who is there.
export type WebMcpStatus = { state: WebMcpState; tools: number }

export function useWebMcp(
  run: (text: string) => Promise<Payload | null>,
  investigate: (
    question: string,
    targets: string[],
    steps: string[]
  ) => Promise<{ command: string; payload: Payload }[]>
): WebMcpStatus {
  // Tools are registered once for the life of the page, so a tool invoked ten
  // minutes later must reach the current run rather than the one that happened
  // to be in scope at mount.
  const runLatest = useEffectEvent(run)
  const investigateLatest = useEffectEvent(investigate)

  const [status, setStatus] = useState<WebMcpStatus>({ state: "unsupported", tools: 0 })

  useMountEffect(() => {
    // The attribute is for an agent reading the dom; the state is for the page
    // telling a person the same thing. One call sets both so they cannot
    // disagree.
    const settle = (state: WebMcpState, count: number) => {
      mark(state)
      setStatus({ state, tools: count })
    }

    const context = modelContext()
    if (!context) {
      // The ordinary path today, and not a failure. The remote server at
      // /api/mcp serves the same commands to an agent that is not in the page.
      settle("unsupported", 0)
      return
    }

    const tools: ToolDescriptor[] = []

    for (const spec of COMMANDS) {
      const name = `dug_${spec.name.toLowerCase()}`
      const properties: ToolDescriptor["inputSchema"]["properties"] = {}
      const required: string[] = []

      if (spec.argument !== "none") {
        properties.target = { type: "string", description: ARGUMENT_HINT[spec.argument] }
        required.push("target")
      }

      const second = SECOND[spec.name]
      if (second) {
        properties[second.name] = { type: "string", description: second.about }
        if (second.required) {
          required.push(second.name)
        }
      }

      const tool: ToolDescriptor = {
        name,
        description:
          `${spec.summary}. Runs live against ${spec.name === "SRC" ? "the resolver list" : "the given target"} ` +
          `and renders the answer onto the page. Example: ${spec.example}`,
        inputSchema: { type: "object", properties, required },
        // Every command is a read. untrustedContentHint because the answer is
        // built from whatever a third party nameserver, registry or web server
        // returned, which is not content this page vouches for.
        annotations: { readOnlyHint: true, untrustedContentHint: true },
        execute: async (input) => {
          const words: string[] = [spec.name]
          if (input?.target) {
            words.push(input.target)
          }
          if (second && input?.[second.name]) {
            words.push(input[second.name])
          }

          const payload = await runLatest(words.join(" "))
          if (!payload) {
            return {
              content: [{ type: "text", text: `${spec.name} couldn’t run. check the argument.` }],
              isError: true,
            }
          }

          // Both shapes: a text block for agents that read MCP content, and the
          // payload itself for anything that wants to walk the evidence.
          return {
            content: [{ type: "text", text: describe(payload) }],
            structuredContent: payload,
          }
        },
      }

      tools.push(tool)
    }

    // The one tool with no counterpart on the remote server, because it is the
    // one whose output is the page. /mcp could run the same lookups and return
    // the same payloads; it cannot leave them on a screen for the person who
    // asked, which is the only thing an investigation adds.
    tools.push({
      name: "dug_investigate",
      description:
        "Answer a question that takes more than one lookup, and build the case on the page " +
        "while you do. You choose the commands and the order: this is your plan, not a preset, " +
        "and a step written with {target} runs once per target when targets has more than one. " +
        "Each screen is left on the page, in sequence, under the question that produced it, " +
        "so the person watching ends up with the evidence, not your summary of it. " +
        "Prefer this over calling the single-command tools yourself whenever the person has " +
        "described a symptom instead of naming a lookup, and whenever one answer won’t " +
        "settle it. Steps use the same grammar as the other tools: a command name, a target, " +
        "and an optional second argument. For example " +
        example("mail") +
        ", or " +
        example("dns") +
        ". A step naming a command that doesn’t exist is skipped and reported back to you; " +
        "the rest still run.",
      inputSchema: {
        type: "object",
        properties: {
          question: {
            type: "string",
            description:
              "The question being answered, in the words the person would use. It becomes " +
              "the heading the evidence is filed under. Not a restatement of the commands.",
          },
          target: {
            type: "string",
            description: "The one domain or host under investigation; use targets for more than one.",
          },
          targets: {
            type: "array",
            items: { type: "string" },
            description:
              "Several domains or hosts under one question. Write each step with {target} " +
              "where the name goes and it runs once per target, in this order.",
          },
          steps: {
            type: "array",
            description:
              "The commands to run, in order, each a full command line such as " +
              '"MAIL example.com" or "DIG example.com MX". Three to five is usually right: ' +
              "enough to be conclusive, few enough to read.",
          },
        },
        required: ["question", "steps"],
      },
      annotations: { readOnlyHint: true, untrustedContentHint: true },
      execute: async (input) => {
        const question = (input?.question ?? "").trim()
        // A model may hand targets back as an array, or fall back to the
        // single-target field. Both are obviously meant.
        const rawTargets = input?.targets
        const listed = (Array.isArray(rawTargets) ? rawTargets : [])
          .map((target) => String(target).trim())
          .filter(Boolean)
        const target = (input?.target ?? "").trim()
        const targets = listed.length > 0 ? listed : target ? [target] : []

        // A model may hand steps back as an array or, less often, as a
        // newline or comma separated string. Both are obviously meant.
        const raw = input?.steps
        const rawSteps = (Array.isArray(raw) ? raw : String(raw ?? "").split(/[\n,]/))
          .map((step) => String(step).trim())
          .filter(Boolean)

        if (!question || targets.length === 0 || rawSteps.length === 0) {
          return {
            content: [
              {
                type: "text",
                text: "investigate needs a question, at least one target, and at least one command to run",
              },
            ],
            isError: true,
          }
        }

        const steps = expandSteps(rawSteps, targets)

        const found = await investigateLatest(question, targets, steps)
        if (found.length === 0) {
          return {
            content: [
              { type: "text", text: `none of those commands could run against ${targets.join(", ")}` },
            ],
            isError: true,
          }
        }

        // Say which steps did not run rather than quietly returning fewer
        // answers than were asked for. A plan that half worked is a thing the
        // model needs to know about, and the person can already see the gap.
        const ran = new Set(found.map((step) => step.command))
        const skipped = steps.filter((step) => !ran.has(step))

        // The verdicts in order, so the model can reason about the case without
        // walking every block, and the payloads underneath so it can if it
        // wants to. The person already has the screens.
        const summary = [
          `${question}: ${targets.join(", ")}`,
          ...found.map((step) => `\n${step.command}\n${describe(step.payload)}`),
          ...(skipped.length > 0 ? [`\nnot run: ${skipped.join(", ")}`] : []),
        ].join("\n")

        return {
          content: [{ type: "text", text: summary }],
          structuredContent: {
            question,
            targets,
            steps: found.map((step) => ({ command: step.command, payload: step.payload })),
            skipped,
          },
        }
      },
    })

    // One controller for the whole set. Aborting it on unmount removes every
    // tool, which is also what keeps a second mount from colliding: the old
    // registration is gone before the new one runs, rather than being cleared
    // name by name and racing.
    const controller = new AbortController()

    const registrations = tools.map((tool) =>
      attempt(() => context.registerTool(tool, { signal: controller.signal }))
    )

    // Poll getTools rather than judging by the registration calls, for two
    // reasons that both leave a fully working page reporting no tools:
    // React's second mount pass aborts calls that already succeeded, and at
    // least one registerTool promise never settles in a production build, so
    // anything awaiting them all waits forever.
    void (async () => {
      const deadline = Date.now() + SETTLE_BUDGET_MS

      for (;;) {
        const registered = await present(context)

        // No getTools to check against. The calls are the only evidence left,
        // and since they may never settle they get the same budget and no more.
        if (registered === null) {
          const results = await Promise.race([
            Promise.all(registrations),
            wait(Math.max(0, deadline - Date.now())).then(() => null),
          ])
          if (!controller.signal.aborted && results !== null) {
            const ok = results.every(Boolean)
            settle(ok ? "registered" : "failed", ok ? tools.length : 0)
          }
          return
        }

        if (tools.every((tool) => registered.has(tool.name))) {
          if (!controller.signal.aborted) {
            settle("registered", tools.length)
          }
          return
        }
        if (Date.now() >= deadline) {
          // Aborted means unmounted, and the tools going away with it is not a
          // failure. Say nothing rather than report one that did not happen.
          if (!controller.signal.aborted) {
            settle("failed", 0)
          }
          return
        }
        await wait(SETTLE_STEP_MS)
      }
    })()

    return () => controller.abort()
  })

  return status
}
