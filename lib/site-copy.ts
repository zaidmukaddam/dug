// Copy shared between the html pages and their markdown form in proxy.ts, so
// the two cannot say different things about the same tool. If the landing's
// prose ever needs to appear in markdown too, extend this module rather than
// hand-writing it in the proxy.

export type Row = { label: string; value: string; accent?: boolean }

export const READING_A_SCREEN: Row[] = [
  { label: "the verdict", value: "the answer in one sentence", accent: true },
  { label: "the blocks", value: "the evidence for it" },
  { label: "[*] live", value: "answered a moment ago" },
  { label: "[~] cached", value: "held under its own ttl, age shown" },
  { label: "degraded", value: "an upstream failed, the rest still answered" },
  { label: "none", value: "checked and absent, not skipped" },
]

export const THE_GUARD: Row[] = [
  { label: "checked", value: "in the dialer, after the name resolves and before connect", accent: true },
  { label: "covers", value: "every candidate address, not only the first" },
  { label: "refuses", value: "private, loopback, link-local, carrier-grade NAT and reserved space" },
  { label: "ipv4-in-ipv6", value: "judged by the address inside, not the wrapper" },
  { label: "ports", value: "an allowlist; PORTS waives it and nothing else" },
]

export const MACHINE_READABLE: { href: string; note: string }[] = [
  { href: "/llms.txt", note: "the grammar and the limits" },
  { href: "/openapi.json", note: "one operation per command" },
  { href: "/mcp", note: "mcp, one tool per command" },
]
