// Parse input, dispatch to a screen.
//
// Every command is deterministic and every route is a constant. There is no
// model on the command path, for the same reason as the companion project: a
// bad routing day should degrade the app, not break it.
//
// Nothing here builds a URL from user input beyond a query parameter. The
// endpoint set is closed, so an unrecognised verb fails locally rather than
// being forwarded anywhere.

export type CommandName =
  | "DIG"
  | "PROP"
  | "TTL"
  | "NS"
  | "DNSSEC"
  | "RDAP"
  | "WATCH"
  | "TLS"
  | "HTTP"
  | "TRACE"
  | "MAIL"
  | "SPF"
  | "IP"
  | "ASN"
  | "NET"
  | "PING"
  | "ROUTE"
  | "PORTS"
  | "VS"
  | "SRC"
  | "ME"
  | "SEO"
  | "AEO"
  | "OG"
  | "WEBMCP"
  | "HELP"

// `endpoint` is a host or a bare address, which is what the reachability
// commands take: pinging 1.1.1.1 is as ordinary as pinging a name.
export type ArgumentKind =
  | "domain"
  | "host"
  | "endpoint"
  | "address"
  | "asn"
  | "cidr"
  | "pair"
  | "none"

export type CommandSpec = {
  name: CommandName
  family: string
  endpoint: string
  argument: ArgumentKind
  summary: string
  example: string
}

// An `api/http` package would shadow the standard library http import inside
// the handler, so the HTTP and TRACE handler is served from /api/fetch. The
// command names are unaffected.
export const COMMANDS: CommandSpec[] = [
  {
    name: "DIG",
    family: "resolution",
    endpoint: "/api/resolve",
    argument: "domain",
    summary: "every record type, or a single one",
    example: "DIG example.com MX",
  },
  {
    name: "PROP",
    family: "resolution",
    endpoint: "/api/propagate",
    argument: "domain",
    summary: "agreement across the fixed resolver list",
    example: "PROP example.com",
  },
  {
    name: "TTL",
    family: "resolution",
    endpoint: "/api/resolve",
    argument: "domain",
    summary: "remaining lifetime per record",
    example: "TTL example.com",
  },
  {
    name: "NS",
    family: "delegation",
    endpoint: "/api/delegate",
    argument: "domain",
    summary: "root to tld to authoritative walk",
    example: "NS example.com",
  },
  {
    name: "DNSSEC",
    family: "delegation",
    endpoint: "/api/delegate",
    argument: "domain",
    summary: "chain of trust, ds and dnskey",
    example: "DNSSEC cloudflare.com",
  },
  {
    name: "RDAP",
    family: "registration",
    endpoint: "/api/rdap",
    argument: "domain",
    summary: "registration data with status codes decoded",
    example: "RDAP example.com",
  },
  {
    name: "WATCH",
    family: "registration",
    endpoint: "/api/rdap",
    argument: "domain",
    summary: "domain and certificate expiry, computed now",
    example: "WATCH example.com",
  },
  {
    name: "TLS",
    family: "transport",
    endpoint: "/api/tls",
    argument: "host",
    summary: "chain, validity spans, protocols",
    example: "TLS example.com",
  },
  {
    name: "HTTP",
    family: "transport",
    endpoint: "/api/fetch",
    argument: "host",
    summary: "headers, redirect chain, security headers",
    example: "HTTP example.com",
  },
  {
    name: "TRACE",
    family: "transport",
    endpoint: "/api/fetch",
    argument: "host",
    summary: "dns, tcp, tls and ttfb timing",
    example: "TRACE example.com",
  },
  {
    name: "MAIL",
    family: "mail",
    endpoint: "/api/mail",
    argument: "domain",
    summary: "mx, spf, dkim, dmarc and alignment policy",
    example: "MAIL example.com",
  },
  {
    name: "SPF",
    family: "mail",
    endpoint: "/api/mail",
    argument: "domain",
    summary: "include tree, against the ten lookup limit",
    example: "SPF example.com",
  },
  {
    name: "IP",
    family: "addressing",
    endpoint: "/api/addr",
    argument: "address",
    summary: "reverse dns, asn, prefix, neighbours",
    example: "IP 8.8.8.8",
  },
  {
    name: "ASN",
    family: "addressing",
    endpoint: "/api/addr",
    argument: "asn",
    summary: "prefixes and address space",
    example: "ASN 13335",
  },
  {
    name: "NET",
    family: "addressing",
    endpoint: "/api/addr",
    argument: "cidr",
    summary: "address space grid, a /24 or smaller",
    example: "NET 8.8.8.0/24",
  },
  {
    name: "PING",
    family: "reachability",
    endpoint: "/api/probe",
    argument: "endpoint",
    summary: "icmp echo, round trip time and packet loss",
    example: "PING 1.1.1.1",
  },
  {
    name: "ROUTE",
    family: "reachability",
    endpoint: "/api/probe",
    argument: "endpoint",
    summary: "the hops between here and there, with reverse dns",
    example: "ROUTE example.com",
  },
  {
    name: "PORTS",
    family: "reachability",
    endpoint: "/api/probe",
    argument: "endpoint",
    summary: "which tcp ports are open, closed or filtered",
    example: "PORTS scanme.nmap.org 22,80,443",
  },
  {
    name: "VS",
    family: "meta",
    endpoint: "/api/vs",
    argument: "pair",
    summary: "two domains side by side",
    example: "VS example.com github.com",
  },
  {
    name: "SRC",
    family: "meta",
    endpoint: "/api/src",
    argument: "none",
    summary: "resolver list, cache ceilings, upstream health",
    example: "SRC",
  },
  {
    // Grouped with addressing but listed here, because pkg/wiring compares the
    // two grammars by position and the Go list appends rather than inserts.
    name: "ME",
    family: "addressing",
    endpoint: "/api/me",
    argument: "none",
    summary: "the address this request came from",
    example: "ME",
  },
  {
    name: "SEO",
    family: "readability",
    endpoint: "/api/seo",
    argument: "domain",
    summary: "what a crawler reads: title, canonical, robots, structured data",
    example: "SEO example.com",
  },
  {
    name: "AEO",
    family: "readability",
    endpoint: "/api/aeo",
    argument: "domain",
    summary: "what an answer engine reads: llms.txt, markdown, content without js",
    example: "AEO example.com",
  },
  {
    name: "OG",
    family: "readability",
    endpoint: "/api/og",
    argument: "domain",
    summary: "the share card, with the image fetched and measured",
    example: "OG example.com",
  },
  {
    name: "WEBMCP",
    family: "readability",
    endpoint: "/api/webmcp",
    argument: "domain",
    summary: "tools for an agent in the page, and the mcp surface for one that isn’t",
    example: "WEBMCP example.com",
  },
  {
    name: "HELP",
    family: "meta",
    endpoint: "",
    argument: "none",
    summary: "this list",
    example: "HELP",
  },
]

// What the tool refuses to do, and why. Shown on the landing page and in HELP
// from this one list, because non-goals that only appear after someone asks
// read as missing features rather than decisions.
export const NOT_HERE: { label: string; reason: string }[] = [
  {
    label: "monitoring and alerts",
    reason: "nothing is stored between queries",
  },
  {
    label: "registrant lookup",
    reason: "redacted at source, and there’s an official channel",
  },
  {
    label: "reaching private space",
    reason: "every destination is validated, on every command",
  },
]

const BY_NAME = new Map(COMMANDS.map((spec) => [spec.name, spec]))

export type ParsedCommand = {
  spec: CommandSpec
  target: string
  other?: string
  type?: string
  url: string
  cacheExtra: string
  label: string
}

export type ParseFailure = {
  input: string
  message: string
  hint?: string
}

export type ParseResult =
  | { ok: true; command: ParsedCommand }
  | { ok: false; failure: ParseFailure }

const DOMAIN = /^(?=.{1,253}$)([a-z0-9¡-￿](?:[a-z0-9¡-￿-]{0,61}[a-z0-9¡-￿])?\.)+[a-z¡-￿]{2,}$/i
const IPV4 = /^(\d{1,3}\.){3}\d{1,3}$/
const CIDR = /^\S+\/\d{1,3}$/

function looksLikeAddress(value: string): boolean {
  return IPV4.test(value) || value.includes(":")
}

function validate(spec: CommandSpec, words: string[]): ParseFailure | null {
  const [first, second] = words

  if (spec.argument === "none") {
    return null
  }

  if (!first) {
    return { input: spec.name, message: `${spec.name.toLowerCase()} needs an argument`, hint: spec.example }
  }

  if (spec.argument === "pair" && !second) {
    return { input: first, message: "vs needs two domains", hint: spec.example }
  }

  if ((spec.argument === "domain" || spec.argument === "host" || spec.argument === "pair") && !DOMAIN.test(first)) {
    return {
      input: first,
      message: `${first} isn’t a domain name`,
      hint: spec.example,
    }
  }

  if (spec.argument === "pair" && second && !DOMAIN.test(second)) {
    return { input: second, message: `${second} isn’t a domain name`, hint: spec.example }
  }

  if (spec.argument === "address" && !looksLikeAddress(first)) {
    return { input: first, message: `${first} isn’t an ip address`, hint: spec.example }
  }

  if (spec.argument === "endpoint" && !looksLikeAddress(first) && !DOMAIN.test(first)) {
    return {
      input: first,
      message: `${first} isn’t a host or an ip address`,
      hint: spec.example,
    }
  }

  if (spec.argument === "asn" && !/^(as)?\d+$/i.test(first)) {
    return { input: first, message: `${first} isn’t an as number`, hint: spec.example }
  }

  if (spec.argument === "cidr" && !CIDR.test(first)) {
    return {
      input: first,
      message: `${first} isn’t a network in cidr form`,
      hint: spec.example,
    }
  }

  return null
}

const RECORD_TYPES = ["A", "AAAA", "CNAME", "MX", "TXT", "NS", "SOA", "CAA", "DS", "DNSKEY"]

// Reduce a pasted url to the host the command actually wants: scheme, userinfo,
// path, query, fragment and port all go, because all of them come along when
// someone copies out of a browser bar. Not for cidr, where the slash is the
// prefix length and cutting at it would turn `8.8.8.0/24` into a bare address
// the handler then refuses.
//
// A word that strips to nothing is returned unchanged, so `HTTP /a/b` still
// fails with the word it was given rather than with "needs an argument".
function hostFrom(word: string, kind: ArgumentKind): string {
  const withoutScheme = word.replace(/^[a-z][a-z0-9+.-]*:\/\//i, "")

  if (kind === "cidr") {
    return withoutScheme || word
  }

  const host = withoutScheme
    .replace(/[/?#].*$/, "")
    .replace(/^.*@/, "")
    .replace(/:\d*$/, "")

  return host || word
}

export function parse(input: string): ParseResult {
  const words = input.trim().split(/\s+/).filter(Boolean)

  if (words.length === 0) {
    return { ok: false, failure: { input, message: "type a command", hint: "HELP" } }
  }

  const verb = words[0].toUpperCase() as CommandName
  const spec = BY_NAME.get(verb)

  if (!spec) {
    return {
      ok: false,
      failure: {
        input: words[0],
        message: `${words[0].toLowerCase()} isn’t a command`,
        hint: "HELP lists all of them",
      },
    }
  }

  const rest = words.slice(1)

  // Both slots, not just the first: `VS a.com https://b.com` is as ordinary a
  // paste as the other way round, and stripping only one made the two
  // arguments behave differently.
  const target = hostFrom(rest[0] ?? "", spec.argument)
  const second = spec.argument === "pair" ? hostFrom(rest[1] ?? "", spec.argument) : rest[1]

  const checked = [target]
  if (second !== undefined) {
    checked.push(second)
  }

  const failure = validate(spec, checked)
  if (failure) {
    return { ok: false, failure }
  }

  const other = spec.argument === "pair" ? second : undefined
  const type =
    spec.name === "DIG" && rest[1] && RECORD_TYPES.includes(rest[1].toUpperCase())
      ? rest[1].toUpperCase()
      : undefined
  // PORTS takes an optional list after the host: 22,80,8000-8010.
  const ports = spec.name === "PORTS" && rest[1] ? rest[1] : undefined
  const count = spec.name === "PING" && /^\d+$/.test(rest[1] ?? "") ? rest[1] : undefined

  const query = new URLSearchParams({ command: spec.name })
  if (target) {
    query.set("target", target)
  }
  if (other) {
    query.set("other", other)
  }
  if (type) {
    query.set("type", type)
  }
  if (ports) {
    query.set("ports", ports)
  }
  if (count) {
    query.set("count", count)
  }

  return {
    ok: true,
    command: {
      spec,
      target,
      other,
      type,
      url: spec.endpoint ? `${spec.endpoint}?${query.toString()}` : "",
      cacheExtra: [other, type, ports, count].filter(Boolean).join(" "),
      label: [spec.name, target, other, type, ports, count].filter(Boolean).join(" "),
    },
  }
}

export const FAMILIES = Array.from(new Set(COMMANDS.map((spec) => spec.family)))
