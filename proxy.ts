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
import { commandLine } from "@/lib/command-line"
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
  "/api/aicatalog",
  "/api/server",
  "/api/servercard",
])

// The contract this surface answers under. Mirrors screen.APIVersion in Go;
// pkg/wiring fails if the two drift.
const API_VERSION = "1"

// The published quota, and it is real: requests past it are refused rather than
// counted and forgiven. Set high enough that a person driving the browser app,
// which spends one request per command, will never see it.
//
// Honest about what it is not: this counts in the memory of one proxy instance,
// and a serverless deployment runs several. The effective ceiling is therefore
// this number times however many instances are warm, and a client spread across
// them gets more than 60. It is a real backstop against one caller hammering one
// instance, and a truthful signal for a well-behaved agent to pace itself
// against. It is not a security control and llms.txt says so.
const RATE_LIMIT = 60
const RATE_WINDOW_SECONDS = 60

type Bucket = { count: number; resetAt: number }
const buckets = new Map<string, Bucket>()

// Swept opportunistically rather than on a timer: a proxy instance has no
// lifecycle hook to clean up on, and an unbounded map is the one way this could
// hurt a deployment that is otherwise stateless.
function sweep(now: number) {
  if (buckets.size < 5000) {
    return
  }
  for (const [key, bucket] of buckets) {
    if (bucket.resetAt <= now) {
      buckets.delete(key)
    }
  }
}

function clientKey(request: NextRequest): string {
  const forwarded = request.headers.get("x-forwarded-for")
  if (forwarded) {
    return forwarded.split(",")[0]?.trim() || "unknown"
  }
  return request.headers.get("x-real-ip")?.trim() || "unknown"
}

type Quota = { allowed: boolean; remaining: number; resetSeconds: number }

function take(request: NextRequest): Quota {
  const now = Date.now()
  sweep(now)

  const key = clientKey(request)
  const existing = buckets.get(key)

  if (!existing || existing.resetAt <= now) {
    buckets.set(key, { count: 1, resetAt: now + RATE_WINDOW_SECONDS * 1000 })
    return { allowed: true, remaining: RATE_LIMIT - 1, resetSeconds: RATE_WINDOW_SECONDS }
  }

  existing.count += 1
  const resetSeconds = Math.max(1, Math.ceil((existing.resetAt - now) / 1000))
  return {
    allowed: existing.count <= RATE_LIMIT,
    remaining: Math.max(0, RATE_LIMIT - existing.count),
    resetSeconds,
  }
}

// Both spellings. RFC 9331 is the structured-field form; the three separate
// fields are what most clients and every older library actually read, and
// sending both costs a few bytes.
function rateHeaders(quota: Quota): Record<string, string> {
  return {
    "RateLimit-Limit": String(RATE_LIMIT),
    "RateLimit-Remaining": String(quota.remaining),
    "RateLimit-Reset": String(quota.resetSeconds),
    "RateLimit-Policy": `"fixed";q=${RATE_LIMIT};w=${RATE_WINDOW_SECONDS}`,
    RateLimit: `"fixed";r=${quota.remaining};t=${quota.resetSeconds}`,
  }
}

// The same envelope a Go refusal returns, so a caller that already parses one
// error shape does not need a second for the errors raised out here.
function apiError(
  status: number,
  code: string,
  message: string,
  hint: string,
  pathname: string,
  extra: Record<string, string> = {}
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
      ...extra,
    },
  })
}

// Served by a rewrite or by a file convention, not by a page. Markdown is not
// theirs to offer: llms.txt is already the markdown-shaped document, and the
// rest are their own media types.
const PASS_THROUGH = new Set([
  "/llms.txt",
  "/openapi.json",
  "/server.json",
  "/.well-known/api-catalog",
  "/.well-known/ai-catalog.json",
  "/.well-known/mcp/server-card.json",
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

// A command url opened in a browser is a shared link, not an api call. The Go
// handler behind it knows text and json and nothing else, so before this a
// person tapping dug.sh/tls/github.com on a phone got raw json. Only a
// navigation is redirected: a fetch() from a page sends */* and a script that
// asked for html on purpose can still say ?format=json.
function wantsApp(request: NextRequest): boolean {
  if (request.method !== "GET" || request.nextUrl.searchParams.has("format")) {
    return false
  }
  const mode = request.headers.get("sec-fetch-mode")
  if (mode && mode !== "navigate") {
    return false
  }
  return (request.headers.get("accept")?.toLowerCase() ?? "").includes("text/html")
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
- [/mcp](${ORIGIN}/mcp) — MCP server, Streamable HTTP, one tool per command
- [/server.json](${ORIGIN}/server.json) — the MCP server manifest, if you are adding it to a client
- [/developers](${ORIGIN}/developers) — calling it, the error model, versioning
- [/deprecation](${ORIGIN}/deprecation) — how a route is retired, and how much notice
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
- [/mcp](${ORIGIN}/mcp)
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
- [/mcp](${ORIGIN}/mcp) — MCP server, one tool per command
- [/.well-known/ai-catalog.json](${ORIGIN}/.well-known/ai-catalog.json) — both surfaces, typed by protocol
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

// Every path that renders a page, whether or not it has a markdown form above.
//
// The two are not the same set, and conflating them was a real bug: a request
// for text/markdown fell through to the markdown 404, so /developers — the one
// page an agent looking for an api is most likely to ask for — answered 404 to
// any client that preferred markdown, while the same url in a browser was fine.
// A page with no markdown generator serves its html; only a path that is not a
// page at all is a 404.
const PAGE_PATHS = new Set([...Object.keys(PAGES), "/developers", "/deprecation", "/contact", "/privacy"])

export function proxy(request: NextRequest) {
  const pathname = request.nextUrl.pathname
  const head = pathname.split("/")[1] ?? ""
  const isCommand = COMMAND_PREFIXES.has(head)

  // Both spellings of the same api. /tls/github.com is the one llms.txt tells
  // an agent to call and it rewrites to /api/tls, so limiting only the /api
  // form would leave the documented surface uncounted — which it did, until
  // this was measured.
  //
  // /mcp is the same trap in a second place: it rewrites to /api/mcp, and every
  // tool call an agent makes runs a real command through it. Counting only the
  // long spelling would make the quota opt-in. The other short paths added
  // alongside it — /server.json, the two well-known documents — are static,
  // cached for a day and cost nothing upstream, so they stay uncounted like
  // /llms.txt and /openapi.json already are.
  // A Server Action is a POST to the page's own url carrying a Next-Action
  // header, so it cannot be matched by path. The planner is one, and it calls
  // a model, which is the most expensive thing on the site. Counting every
  // action rather than the one that exists today means the next one is
  // covered before anyone remembers to add it.
  const isAction = request.method === "POST" && request.headers.has("next-action")

  // Ahead of the quota: a redirect costs nothing upstream, and the command it
  // leads to is counted when the app runs it.
  if (isCommand && wantsApp(request)) {
    const url = request.nextUrl.clone()
    url.pathname = "/"
    url.search = ""
    url.searchParams.set("run", commandLine(pathname))
    const response = NextResponse.redirect(url, 302)
    // The same url answers json to curl, so no cache may keep this for it.
    response.headers.set("vary", "Accept, User-Agent")
    response.headers.set("cache-control", "no-store")
    return response
  }

  if (pathname.startsWith("/api/") || isCommand || pathname === "/mcp" || isAction) {
    // Counted before anything else, so a refused request is cheap and a caller
    // that is over its quota is told so rather than served.
    const quota = take(request)
    if (!quota.allowed) {
      return apiError(
        429,
        "rate_limited",
        `too many requests, more than ${RATE_LIMIT} in ${RATE_WINDOW_SECONDS} seconds`,
        `wait ${quota.resetSeconds} seconds. RateLimit-Reset says when, Retry-After says the same.`,
        pathname,
        { ...rateHeaders(quota), "Retry-After": String(quota.resetSeconds) }
      )
    }

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
        pathname,
        rateHeaders(quota)
      )
    }

    // A path under /api that no function serves used to fall through to the
    // html 404, so a caller probing the api surface got a react page where it
    // asked for an endpoint. Anything under /api answers in json, including
    // when the answer is that there is nothing here.
    // Only for the /api spelling: a pretty path is matched by prefix and its
    // exact shape is the rewrite's business, not this one's.
    if (pathname.startsWith("/api/") && !API_ENDPOINTS.has(pathname)) {
      return apiError(
        404,
        "unknown_endpoint",
        `${pathname} is not an endpoint`,
        "every path this api serves is listed in /openapi.json and /llms.txt",
        pathname,
        rateHeaders(quota)
      )
    }

    // The quota is reported on the answer too, not only on the refusal: a
    // caller that only learns its budget by exhausting it cannot pace itself,
    // which is the whole point of the headers.
    //
    // Forwarded as request headers rather than set on the response, because a
    // response header set here does not survive to the client for these routes.
    // That was measured: the limiter refused correctly while every 200 reached
    // curl with no RateLimit fields at all. Go reads these two and writes the
    // real headers in screen.writePayload, which is the one exit every command
    // response goes through.
    const forwarded = new Headers(request.headers)
    forwarded.set("x-dug-rate-remaining", String(quota.remaining))
    forwarded.set("x-dug-rate-reset", String(quota.resetSeconds))
    return NextResponse.next({ request: { headers: forwarded } })
  }

  if (PASS_THROUGH.has(pathname)) {
    return NextResponse.next()
  }

  if (wantsMarkdown(request)) {
    const page = PAGES[pathname]
    if (page) {
      return markdown(page())
    }
    // A command path is answered by Go in text or json, and a page with no
    // markdown form answers in html. Neither is faked here; only a path that is
    // neither gets the markdown 404, which exists to hand an agent the command
    // list rather than an html error page it cannot read.
    if (!isCommand && !PAGE_PATHS.has(pathname)) {
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
