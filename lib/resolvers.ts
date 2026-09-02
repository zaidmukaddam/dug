// Mirrors pkg/resolvers. The Go list is what actually gets queried; this
// copy exists so HELP and SRC can render the list without a round trip.
// pkg/wiring fails if the two ever drift apart.

export type Resolver = {
  id: string
  name: string
  short: string
  ip: string
}

export const RESOLVERS: Resolver[] = [
  { id: "cloudflare", name: "Cloudflare", short: "cf", ip: "1.1.1.1" },
  { id: "google", name: "Google", short: "goog", ip: "8.8.8.8" },
  { id: "quad9", name: "Quad9", short: "quad9", ip: "9.9.9.9" },
  { id: "opendns", name: "OpenDNS", short: "odns", ip: "208.67.222.222" },
  { id: "adguard", name: "AdGuard", short: "adg", ip: "94.140.14.14" },
  { id: "controld", name: "Control D", short: "ctrld", ip: "76.76.2.0" },
]

export const DEFAULT_RESOLVER = RESOLVERS[0]
