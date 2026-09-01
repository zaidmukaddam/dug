// Markdown content negotiation for the html pages, per acceptmarkdown.com.
//
// The command routes already negotiate three representations in Go. The pages
// did not negotiate at all: they answered text/html to everything, and Next's
// own Vary names only its router headers, so a shared cache in front of this
// would hand a markdown request whichever variant landed in the cache first.
//
// Two things happen here and nothing else. A request that asks for markdown
// gets markdown, and every page response gains Accept in its Vary, which is the
// half that makes the negotiation safe to cache.
//
// proxy.ts, not middleware.ts: the middleware convention is deprecated in Next
// 16 and the export is renamed with it. Proxy runs on the nodejs runtime and
// cannot be switched to edge, which costs this nothing — the imports below are
// plain typescript over two constant lists.
//
// The markdown responses below set their own Vary, because they are returned
// from here and never reach the page renderer.
//
// The html variant cannot carry one. Next owns Vary on a page response and
// replaces any configured value with its own router list. That was measured at
// all three layers available — this proxy, next.config headers(), and
// vercel.json edge headers — each time with a probe header alongside it: the
// probe landed every time and the Vary never did.
//
// The negotiation is still safe, because this runs ahead of the cache. A
// request asking for markdown is answered here and never reaches the cached
// html at all, so the variant mix-up that Vary exists to prevent cannot occur.
// What is lost is the declaration, not the behaviour.

import { NextResponse, type NextRequest } from "next/server"

import { COMMANDS, FAMILIES } from "@/app/commands/grammar"
import { RESOLVERS } from "@/lib/resolvers"

// The pretty path prefixes the Go functions answer on, derived rather than
// listed so a new command cannot be missing from it.
const COMMAND_PREFIXES = new Set(COMMANDS.filter((spec) => spec.endpoint).map((spec) => spec.name.toLowerCase()))

// Every path under /api that a Go function actually answers on. Derived from
// the command list plus the three that are not commands, so a new command
// cannot be missing from it.
const API_ENDPOINTS = new Set([
  ...COMMANDS.map((spec) => spec.endpoint).filter(Boolean),
  "/api/llms",
  "/api/openapi",
  "/api/mcp",
  "/api/guard",
  "/api/catalog",
])

// The contract this surface answers under. Mirrors screen.APIVersion in Go;
// pkg/wiring fails if the two drift.
const API_VERSION = "1"

// The same envelope a Go refusal returns, so a caller that already parses one
// error shape does not need a second for the errors raised out here.
function apiError(
  status: number,
  code: string,
  message: string,
  hint: string,
  pathname: string
) {
  const body = {
    command: "",
    target: pathname,
    verdict: { state: "warn", headline: message, detail: hint },
    ts: Date.now(),
    ttl: 0,
    elapsed_ms: 0,
    upstream_queries: 0,
    notes: [],
    degraded: [],
    blocks: [],
    error: { code, message, hint },
  }

  return new NextResponse(JSON.stringify(body), {
    status,
    headers: {
      "content-type": "application/json",
      "cache-control": "no-store",
      "x-api-version": API_VERSION,
      // Points a caller that guessed wrong at the documents that would not
      // have let it guess. RFC 8631.
      link:
        '</openapi.json>; rel="service-desc"; type="application/json", ' +
        '</developers>; rel="service-doc"; type="text/html"',
    },
  })
}

// Served by a rewrite or by a file convention, not by a page. Markdown is not
// theirs to offer: llms.txt is already the markdown-shaped document, and the
// rest are their own media types.
const PASS_THROUGH = new Set([
  "/llms.txt",
  "/openapi.json",
  "/robots.txt",
  "/sitemap.xml",
  "/opengraph-image",
  "/favicon.ico",
])

function wantsMarkdown(request: NextRequest): boolean {
  const accept = request.headers.get("accept")?.toLowerCase() ?? ""
  if (!accept.includes("text/markdown")) {
    return false
  }
  // An Accept naming both is a browser listing everything it can take; only a
  // client that asked for markdown ahead of html actually wants it.
  const markdown = accept.indexOf("text/markdown")
  const html = accept.indexOf("text/html")
  return html === -1 || markdown < html
}

function markdown(body: string, status = 200) {
  return new NextResponse(body, {
    status,
    headers: {
      "content-type": "text/markdown; charset=utf-8",
      // The half acceptmarkdown.com calls for and the audit found missing.
      // Without it a cache can serve this body to a browser.
      vary: "Accept, Accept-Encoding",
      "cache-control": status === 200 ? "public, s-maxage=3600" : "no-store",
    },
  })
}

const ORIGIN = "https://dug.sh"

function commandTable(): string {
  return FAMILIES.map((family) => {
    const rows = COMMANDS.filter((spec) => spec.family === family)
      .map((spec) => `- \`${spec.name.toLowerCase()}\` — ${spec.summary}. Example: \`${spec.example}\``)
      .join("\n")
    return `### ${family}\n\n${rows}`
  }).join("\n\n")
}

function homeMarkdown(): string {
  return `# dug

Live domain and network diagnostics. Every answer is a fresh lookup made when
you asked for it: nothing is precomputed, nothing is stored between requests,
and each answer is labelled with how old it is.

Every endpoint is a GET, needs no key and no signup.

    curl ${ORIGIN}/tls/github.com
    curl ${ORIGIN}/dig/example.com/MX
    curl -H 'Accept: application/json' ${ORIGIN}/mail/github.com

## Commands

${commandTable()}

## Resolvers

A fixed list that a query cannot point elsewhere:

${RESOLVERS.map((resolver) => `- ${resolver.name} — \`${resolver.ip}\``).join("\n")}

## More

- [/llms.txt](${ORIGIN}/llms.txt) — the full grammar, the limits, and when to use this
- [/openapi.json](${ORIGIN}/openapi.json) — OpenAPI 3.1, one operation per command
- [/api/mcp](${ORIGIN}/api/mcp) — MCP server, Streamable HTTP, one tool per command
- [/developers](${ORIGIN}/developers) — calling it, the error model, versioning
- [/about](${ORIGIN}/about) — how a screen is read and what the guard refuses
`
}

function aboutMarkdown(): string {
  return `# About dug

Live domain and network diagnostics. Every screen is a lookup made when you
asked for it: nothing is precomputed, nothing is stored between requests, and
each answer is labelled with how old it is.

## Reading a screen

- **the verdict** — the answer in one sentence, read it first
- **the blocks** — the evidence for it
- \`[*] live\` — answered just now
- \`[~] cached\` — held under its own ttl, age shown
- **degraded** — an upstream failed, the rest of the answer still stands
- **none** — checked and absent, not skipped

## The address guard

Every destination is validated in the dialer, after the name resolves and
immediately before connect. It covers every candidate address rather than just
the first, refuses private, loopback, link-local, cgnat and reserved space,
judges ipv4-in-ipv6 by the address inside rather than the wrapper, and holds
outbound ports to an allowlist that only PORTS waives.

## Resolvers

${RESOLVERS.map((resolver) => `- ${resolver.name} — \`${resolver.ip}\``).join("\n")}

## Machine readable

- [/llms.txt](${ORIGIN}/llms.txt)
- [/openapi.json](${ORIGIN}/openapi.json)
- [/api/mcp](${ORIGIN}/api/mcp)
- [/developers](${ORIGIN}/developers)
`
}

function notFoundMarkdown(pathname: string): string {
  return `# 404 — \`${pathname}\` does not resolve

The route set is closed, the same way the command set is. Nothing here is
generated from the url, so a path that is not listed below does not exist.

## Where to look next

- [/](${ORIGIN}/) — the terminal
- [/llms.txt](${ORIGIN}/llms.txt) — every command, its arguments and its limits
- [/openapi.json](${ORIGIN}/openapi.json) — OpenAPI 3.1 for the same surface
- [/api/mcp](${ORIGIN}/api/mcp) — MCP server, one tool per command
- [/developers](${ORIGIN}/developers) — calling it, the error model, versioning
- [/sitemap.xml](${ORIGIN}/sitemap.xml) — every indexable page

## Commands

Each is \`GET /<command>/<target>\`, for example \`${ORIGIN}/tls/github.com\`.

${COMMANDS.filter((spec) => spec.endpoint)
  .map((spec) => `- \`/${spec.name.toLowerCase()}/{target}\` — ${spec.summary}`)
  .join("\n")}
`
}

const PAGES: Record<string, () => string> = {
  "/": homeMarkdown,
  "/about": aboutMarkdown,
}

export function proxy(request: NextRequest) {
  const pathname = request.nextUrl.pathname

  if (pathname.startsWith("/api/")) {
    // Checked here rather than in each handler: it is one rule about the
    // request, it has to run before any lookup does, and there are twenty
    // handlers that would otherwise each need to remember it.
    const pinned = request.headers.get("x-api-version")
    if (pinned !== null && pinned.trim() !== "" && pinned.trim() !== API_VERSION) {
      return apiError(
        400,
        "unsupported_version",
        `this api does not serve version ${pinned.trim()}`,
        `only version ${API_VERSION} exists. omit X-API-Version to take the current one.`,
        pathname
      )
    }

    // A path under /api that no function serves used to fall through to the
    // html 404, so a caller probing the api surface got a react page where it
    // asked for an endpoint. Anything under /api answers in json, including
    // when the answer is that there is nothing here.
    if (!API_ENDPOINTS.has(pathname)) {
      return apiError(
        404,
        "unknown_endpoint",
        `${pathname} is not an endpoint`,
        "every path this api serves is listed in /openapi.json and /llms.txt",
        pathname
      )
    }
    return NextResponse.next()
  }

  if (PASS_THROUGH.has(pathname)) {
    return NextResponse.next()
  }

  const head = pathname.split("/")[1] ?? ""
  const isCommand = COMMAND_PREFIXES.has(head)

  if (wantsMarkdown(request)) {
    const page = PAGES[pathname]
    if (page) {
      return markdown(page())
    }
    // A command path is answered by Go in text or json; markdown is not one of
    // its representations, so it passes through rather than being faked here.
    if (!isCommand) {
      return markdown(notFoundMarkdown(pathname), 404)
    }
    return NextResponse.next()
  }

  // Every page response, whatever it ended up serving, has to admit that the
  // body depends on Accept. Next sets its own router Vary and appending keeps
  // both rather than trading one correctness problem for another.
  const response = NextResponse.next()
  if (!isCommand) {
    const existing = response.headers.get("vary")
    response.headers.set("vary", existing ? `${existing}, Accept` : "Accept")
  }
  return response
}

export const config = {
  // Static assets and image optimisation never negotiate, and keeping them out
  // means this does not run on the majority of requests.
  matcher: ["/((?!_next/static|_next/image).*)"],
}
