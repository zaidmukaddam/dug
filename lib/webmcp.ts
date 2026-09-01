"use client"

// WebMCP: the same commands, offered to an agent running inside the page.
//
// The remote MCP server at /api/mcp answers an agent that is somewhere else.
// This is for one already here, in the user's browser, and the difference is
// that a call lands on the page: every tool runs the ordinary command path, so
// the answer appears on screen where the person can see what was asked and
// read the evidence themselves.
//
// The API this targets has moved twice, and this file was written against the
// first shape. What is current, and what was here before:
//
//   entry point   document.modelContext, not navigator.modelContext, which is
//                 the deprecated alias as of Chrome 150
//   registration  registerTool(tool, { signal }), one at a time. The old
//                 provideContext({ tools }) declared the whole list at once and
//                 has been removed from the spec: on a page where two scripts
//                 register tools, replace-all is takeover
//   removal       abort the AbortSignal given at registration, not
//                 unregisterTool
//
// So the previous version of this file registered nothing anywhere, and said so
// nowhere. @mcp-b/global installs the API where the browser has not shipped it
// and wraps the native one where it has, which is what makes this work today
// rather than only on Chrome 149+ behind a flag.
//
// Native WebMCP additionally needs an origin-isolated document, which is why
// next.config sends Origin-Agent-Cluster: ?1.

import "@mcp-b/global"

import { useEffectEvent } from "react"

import { COMMANDS } from "@/app/commands/grammar"
import { useMountEffect } from "@/hooks/use-mount-effect"
import type { Payload } from "@/lib/cache"

type ToolDescriptor = {
  name: string
  description: string
  inputSchema: {
    type: "object"
    properties: Record<string, { type: string; description: string }>
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
  document.documentElement.dataset.webmcpServer = "/api/mcp"
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
    lines.push(`degraded: ${entry.source} — ${entry.reason}`)
  }
  return lines.join("\n")
}

export function useWebMcp(run: (text: string) => Promise<Payload | null>) {
  // Tools are registered once for the life of the page, so a tool invoked ten
  // minutes later must reach the current run rather than the one that happened
  // to be in scope at mount.
  const runLatest = useEffectEvent(run)

  useMountEffect(() => {
    const context = modelContext()
    if (!context) {
      // The ordinary path today, and not a failure. The remote server at
      // /api/mcp serves the same commands to an agent that is not in the page.
      mark("unsupported")
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
              content: [{ type: "text", text: `${spec.name} could not run. check the argument.` }],
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

    // provideContext replaces the whole set in one call, so it cannot collide
    // with a set this page already registered. registerTool can, and does: an
    // effect that runs twice, or a name left behind by a previous mount, throws
    // "Duplicate tool name". Prefer the atomic call and keep the per-tool loop
    // only for an implementation that lacks it.
    // One controller for the whole set. Aborting it on unmount removes every
    // tool, which is also what keeps a second mount from colliding: the old
    // registration is gone before the new one runs, rather than being cleared
    // name by name and racing.
    const controller = new AbortController()

    // The mark is taken from what is registered afterwards, not from whether
    // each call resolved.
    //
    // Those are different answers. React mounts an effect twice in development,
    // and abort races the registrations already in flight, so the second pass
    // gets "Tool already registered" for tools that are present and working.
    // Judging by the individual calls reported failure on a page whose tool set
    // was completely intact, which is the opposite of what this attribute is
    // for. What matters is whether an agent can find the tools.
    void Promise.all(
      tools.map((tool) => attempt(() => context.registerTool(tool, { signal: controller.signal })))
    ).then(async (results) => {
      if (controller.signal.aborted) {
        return
      }
      const registered = await present(context)
      if (registered === null) {
        // No getTools to check against, so fall back to the calls.
        mark(results.every(Boolean) ? "registered" : "failed")
        return
      }
      mark(tools.every((tool) => registered.has(tool.name)) ? "registered" : "failed")
    })

    return () => controller.abort()
  })
}
