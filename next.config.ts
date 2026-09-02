import type { NextConfig } from "next"

import { COMMANDS } from "./app/commands/grammar"

// In production Vercel routes /api/* to the Go functions in api/ directly.
// Locally nothing does, so `pnpm dev` points those paths at cmd/devapi.
// The rewrite only exists when DEV_API_ORIGIN is set, so a production build
// never carries it.
const devApi = process.env.DEV_API_ORIGIN

// A rewrite destination is not itself rewritten, so in development the pretty
// paths have to name the Go server directly rather than route through the
// /api/* rewrite below.
const api = (path: string) => (devApi ? `${devApi}${path}` : path)

// The html pages, which is every route that renders a document rather than
// answering a lookup. pkg/wiring checks this against app/sitemap.ts, so a new
// page cannot be added to one and forgotten in the other.
const PAGES = ["/", "/about", "/developers", "/contact", "/privacy", "/deprecation"]

// The same three relations pkg/screen sets on every api response. service-desc
// is the machine description, service-doc the human one, describedby the prose
// an agent reads first.
const SERVICE_LINKS = [
  `</openapi.json>; rel="service-desc"; type="application/vnd.oai.openapi+json"`,
  `</developers>; rel="service-doc"; type="text/html"`,
  `</llms.txt>; rel="describedby"; type="text/plain"`,
  `</.well-known/ai-catalog.json>; rel="describedby"; type="application/ai-catalog+json"`,
].join(", ")

// The curl and agent surface: `curl /tls/github.com` rather than
// `curl '/api/tls?command=TLS&target=github.com'`. Derived from the same
// COMMANDS the browser parses, and pkg/wiring checks these against the Go
// list that llms.txt, OpenAPI and MCP are generated from.
function prettyPaths() {
  return COMMANDS.filter((spec) => spec.endpoint).flatMap((spec) => {
    const verb = spec.name.toLowerCase()
    const query = `command=${spec.name}`

    if (spec.argument === "none") {
      return [{ source: `/${verb}`, destination: api(`${spec.endpoint}?${query}`) }]
    }

    if (spec.argument === "pair") {
      return [
        {
          source: `/${verb}/:target/:other`,
          destination: api(`${spec.endpoint}?${query}&target=:target&other=:other`),
        },
      ]
    }

    const one = {
      source: `/${verb}/:target`,
      destination: api(`${spec.endpoint}?${query}&target=:target`),
    }

    // A cidr is the one argument that always carries a slash, so it needs two
    // segments and the destination has to put them back together. The
    // one-segment rule stays for the %2F-encoded spelling.
    if (spec.argument === "cidr") {
      return [
        {
          source: `/${verb}/:target/:bits`,
          destination: api(`${spec.endpoint}?${query}&target=:target/:bits`),
        },
        one,
      ]
    }

    // A second segment where the command takes one: DIG a record type, PORTS a
    // port list, PING a count. Same names the query form uses.
    const second: Record<string, string> = { DIG: "type", PORTS: "ports", PING: "count" }
    const extra = second[spec.name]
    if (!extra) {
      return [one]
    }

    return [
      {
        source: `/${verb}/:target/:${extra}`,
        destination: api(`${spec.endpoint}?${query}&target=:target&${extra}=:${extra}`),
      },
      one,
    ]
  })
}

const nextConfig: NextConfig = {
  // Next 16 blocks dev-only resources requested from an origin it does not
  // recognise, and the block is quiet: the page still renders its server HTML,
  // so it looks fine while the HMR socket is refused and the app never
  // hydrates. Opening the same server on 127.0.0.1 instead of localhost is
  // enough to trigger it, and the symptom reads as a broken component rather
  // than a blocked request.
  allowedDevOrigins: ["127.0.0.1", "localhost"],

  // WebMCP is only available in an origin-isolated document: with
  // document.domain reachable, the origin can change under a registered tool,
  // so Chromium disables the API entirely rather than let that happen. This is
  // the header that opts in, and without it the native implementation refuses
  // no matter what the page registers.
  async headers() {
    return [
      { source: "/:path*", headers: [{ key: "Origin-Agent-Cluster", value: "?1" }] },

      // RFC 8631 discovery on the html pages. The Go handlers already set this
      // on every api response, but an agent that arrived at the site rather
      // than at an endpoint was landing on a page whose only Link relation was
      // the canonical url — so the first document it could reach told it
      // nothing about the api underneath. Listed page by page rather than
      // /:path* on purpose: a wildcard also matches the api routes, and they
      // would then carry this header twice.
      ...PAGES.map((source) => ({
        source,
        headers: [{ key: "Link", value: SERVICE_LINKS }],
      })),
    ]
  },

  async rewrites() {
    const discovery = [
      { source: "/llms.txt", destination: api("/api/llms") },
      { source: "/openapi.json", destination: api("/api/openapi") },
      // RFC 9727 fixes the path, so this is the one route whose location is
      // not ours to choose.
      { source: "/.well-known/api-catalog", destination: api("/api/catalog") },
      // The MCP registry manifest, at the path the registry and every client
      // that reads one expect.
      { source: "/server.json", destination: api("/api/server") },
      // The AI Catalog specification fixes this path the same way RFC 9727
      // fixes the one above.
      { source: "/.well-known/ai-catalog.json", destination: api("/api/aicatalog") },
      // SEP-1649 fixes this one. The card is what a client would have got from
      // initialize and tools/list, without the handshake.
      { source: "/.well-known/mcp/server-card.json", destination: api("/api/servercard") },
      // /mcp is where a client looks for a remote MCP server, and it was a 404
      // here while the server sat at /api/mcp. Same handler, both spellings, so
      // nothing that already hardcoded the long path breaks.
      { source: "/mcp", destination: api("/api/mcp") },
    ]

    const routes = [...discovery, ...prettyPaths()]

    if (!devApi) {
      return routes
    }

    return [...routes, { source: "/api/:path*", destination: `${devApi}/api/:path*` }]
  },
}

export default nextConfig
