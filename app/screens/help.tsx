// HELP builds a payload locally and hands it to the same renderer every other
// command uses. It is the one screen with no upstream, so it is also the proof
// that the block envelope is the only contract between a command and a screen.

import { COMMANDS, FAMILIES, NOT_HERE } from "@/app/commands/grammar"
import { RESOLVERS } from "@/lib/resolvers"
import type { Payload } from "@/lib/cache"

export function helpPayload(): Payload {
  return {
    command: "HELP",
    target: "grammar",
    verdict: {
      state: "ok",
      headline: `${COMMANDS.length} commands, each answering one question about a domain`,
      detail:
        "type one with a domain after it. every screen leads with the answer in a sentence and puts the graphs underneath as the evidence.",
    },
    ts: Date.now(),
    ttl: 3600,
    elapsed_ms: 0,
    upstream_queries: 0,
    degraded: [],
    blocks: [
      {
        component: "GraphSheet",
        span: 2,
        props: {
          title: "commands",
          headers: ["command", "does"],
          sections: FAMILIES.map((family) => ({
            title: family,
            rows: COMMANDS.filter((spec) => spec.family === family).map((spec) => [
              spec.example,
              spec.summary,
            ]),
          })),
        },
      },
      {
        component: "GraphSpec",
        props: {
          title: "reading a screen",
          rows: [
            { label: "[*] live", value: "answered just now", accent: true },
            { label: "[~] cached", value: "held under its own ttl, age shown" },
            { label: "[x] and [ ]", value: "passed and did not, glyph never colour alone" },
            { label: "none", value: "checked and absent, not skipped" },
            { label: "degraded", value: "an upstream failed, the rest still answered" },
          ],
        },
      },
      {
        component: "GraphTable",
        span: 2,
        props: {
          title: "resolver list",
          headers: ["name", "address"],
          rows: RESOLVERS.map((resolver) => [resolver.name, resolver.ip]),
        },
      },
      {
        component: "GraphCheck",
        props: {
          title: "deliberately not here",
          items: NOT_HERE.map((item) => ({
            label: item.label,
            done: false,
            note: item.reason,
          })),
        },
      },
    ],
    notes: [
      "every command is deterministic and there is no model on the command path. " +
        "a bad routing day should degrade this app, not break it.",
      "arrow keys walk the history. the resolver list is a constant in the codebase, " +
        "not something a query can point somewhere else.",
    ],
  }
}
