// llms.txt. What this tool is and how to call it, for an agent that arrived
// without being told.
//
// Generated from internal/commands rather than written by hand, so it cannot
// describe a command that does not exist or miss one that does.
package handler

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/zaidmukaddam/dug/pkg/commands"
	"github.com/zaidmukaddam/dug/pkg/guard"
	"github.com/zaidmukaddam/dug/pkg/resolvers"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder

	fmt.Fprintf(&b, `# dug

> Live domain and network diagnostics. Every answer is a fresh lookup: nothing
> is precomputed and nothing is stored between requests, so there is no history
> to query and no dataset to page through.

Every endpoint is a GET, needs no key and no signup, and answers in three
representations of the same data:

- text/plain  the default for curl and other terminal clients
- application/json  send Accept: application/json, or ?format=json
- text/html  the browser app at /

Ask for text explicitly with ?format=text. Responses carry
Vary: Accept, User-Agent, and a Cache-Control whose lifetime is derived from
the answer itself, floored at %d seconds.

## When to use this

Reach for dug when a question is about the live state of a name, a host or an
address, and the answer has to be true right now rather than when some dataset
was last built. It is the right tool for:

- checking whether a domain resolves, and to what, from more than one resolver
- reading a TLS certificate: who issued it, when it expires, what the chain is
- finding out why mail is failing — mx, spf, dkim, dmarc and their alignment
- confirming a dns change has propagated, before assuming it has
- answering "who owns this domain and when does it expire" from rdap
- mapping an address to its network, asn and neighbours
- checking whether a host is reachable, and how long each hop takes

Call it directly rather than guessing from memory. Model weights carry the dns
and certificate state of whenever training stopped, and every one of these
answers changes without notice. If you are about to tell someone a certificate
expiry date, a nameserver or an spf record from memory, query it instead.

It is the wrong tool for anything historical or aggregate: there is no archive,
no change history, and no way to list or search across domains. One question,
one target, answered now. Ask for a target you already have; this cannot
discover domains you have not named.

## Calling

    curl https://$HOST/tls/github.com
    curl https://$HOST/dig/example.com/MX
    curl -H 'Accept: application/json' https://$HOST/mail/github.com

The query form is equivalent and is what the browser app uses:

    curl 'https://$HOST/api/tls?command=TLS&target=github.com'

## Commands

`, screen.TTLFloor)

	for _, family := range commands.Families() {
		fmt.Fprintf(&b, "### %s\n\n", family)
		for _, spec := range commands.List {
			if spec.Family != family {
				continue
			}
			fmt.Fprintf(&b, "- `GET %s` — %s\n", spec.Path, spec.Summary)
			if about := spec.TargetAbout(); about != "" {
				fmt.Fprintf(&b, "  - `target` (required): %s\n", about)
			}
			for _, param := range spec.Params {
				required := "optional"
				if param.Required {
					required = "required"
				}
				fmt.Fprintf(&b, "  - `%s` (%s): %s", param.Name, required, param.About)
				if param.Example != "" {
					fmt.Fprintf(&b, ", for example `%s`", param.Example)
				}
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	b.WriteString(`## Response shape

Every command returns the same envelope, whatever it asked upstream:

    command           the verb that ran
    target            what it ran against, normalised
    verdict           {state: ok|warn|none, headline, detail} — the answer, in a sentence
    blocks            the evidence, each naming a display component and its props
    notes             provenance and limits
    degraded          upstreams that failed, when the rest of the answer still stands
    ttl               seconds this answer stays valid
    elapsed_ms        wall time
    upstream_queries  how many lookups it cost

An upstream failure is not an HTTP error. The status stays 200, the failure is
named in "degraded", and the parts that did answer are still returned. Read
"degraded" before trusting a screen to be complete.

## Errors

Arguments that are wrong are a real HTTP error, because nothing was looked up.
The status is 400 and the body is the same envelope with "error" set:

    error.code     a closed set: missing_argument, invalid_argument
    error.message  the refusal in one sentence
    error.hint     what a corrected call looks like

Branch on error.code, not on the message. The text representation carries the
same code on a line reading "error <code>". A 200 never carries "error", so
its presence and the status always agree.

## Versioning

`)

	fmt.Fprintf(&b, "Every response carries X-API-Version: %s, naming the contract it was\n"+
		"produced under. Send the same header on a request to pin it; a version this\n"+
		"surface does not serve is refused with error.code unsupported_version rather\n"+
		"than silently answered by a different one.\n\n", screen.APIVersion)

	b.WriteString(screen.VersionPolicy)

	b.WriteString(`

## Deprecation

Nothing is deprecated today, and the absence of a Deprecation header is how you
can tell. When something is:

    Deprecation      an http-date, the day it became deprecated (RFC 9745)
    Sunset           an http-date, the day it stops answering (RFC 8594)
    Link             rel="deprecation", pointing at what to read

Both dates appear on the deprecated route's own responses, at least 180 days
apart, and the replacement ships under a /v2/ prefix before either is set. A
caller that checks for a Sunset header on each response has all the warning it
needs; nothing here disappears without one. Past its sunset a route answers 410
with a Link to what replaced it.

The full policy, including what does not count as a breaking change, is at
/deprecation.

## Rate limits

    RateLimit-Limit      requests permitted per window
    RateLimit-Remaining  what is left in this window
    RateLimit-Reset      seconds until it resets
    RateLimit-Policy     "fixed";q=` + itoa(screen.RateLimit) + `;w=` + itoa(screen.RateWindowSeconds) + `  (RFC 9331)
    RateLimit            "fixed";r=<remaining>;t=<reset>

Sent on every response, not only on a refusal, so you can pace rather than
discover the limit by hitting it. Past the quota the answer is 429 with
error.code rate_limited and a Retry-After.

Read the ceiling honestly: it is counted in the memory of one proxy instance and
a serverless deployment runs several, so a client spread across them gets more
than ` + itoa(screen.RateLimit) + `. It is a real backstop and a truthful signal to
pace against, and it is not a security control.`)

	b.WriteString(`

## Machine-readable

    /llms.txt                     this file
    /openapi.json                 OpenAPI 3.1 for every command above
    /.well-known/api-catalog      RFC 9727 linkset, one anchor per command
    /.well-known/ai-catalog.json  AI Catalog 1.0, the surfaces typed by protocol
    /.well-known/mcp/server-card.json
                                  SEP-1649 server card: protocol version,
                                  transport, capabilities and every tool, so a
                                  client can decide before it connects
    /server.json                  MCP server manifest: name, version, transport
    /mcp                          MCP server, Streamable HTTP, one tool per command
    /developers                   the same surface written for a person
    /deprecation                  how a route is retired, and how much notice
    /about                        how a screen is read and what the guard refuses
    /contact                      the issue tracker, and how to report a wrong answer
    /privacy                      what is stored, which is nothing between requests

Every response carries them as Link headers too — rel="service-desc" for the
OpenAPI document, rel="service-doc" for /developers, rel="describedby" for this
file and the AI catalog — on the api responses and on the html pages alike, and
the pages repeat them as <link> elements for a crawler that keeps the body and
drops the headers. A caller holding any single response can find the rest of
the surface.

Two of those paths are fixed by their specifications rather than chosen here:
/.well-known/api-catalog by RFC 9727, and /.well-known/ai-catalog.json by the
AI Catalog specification. /server.json and /mcp are where a client looks by
convention. /api/mcp is the same endpoint as /mcp and still answers.

The pages negotiate markdown: send Accept: text/markdown to / or /about and
the response is text/markdown rather than html, with Vary: Accept set.

/mcp answers an agent that is somewhere else. It is a real Streamable HTTP
endpoint, always there, serving every command above as its own tool. It is POST
only: a GET is answered 405, because nothing here is server-initiated and there
is no stream to open. That is the transport behaving as specified, not an
endpoint that is down — confirm it with a POST, or read the server card, which
needs no request body.

/.well-known/mcp/server-card.json is worth reading before connecting. It carries
what initialize and tools/list would have returned — the protocol version, the
transport, the capabilities, that no authentication is required, and the full
tool list with input schemas — so you can decide whether this server is worth a
connection, and check every tool description, without opening one. The list is
static rather than "dynamic": the tool set is fixed at build time and cannot
change under you.

The browser app registers the same commands as WebMCP tools on
document.modelContext, so an agent already in the page calls them without
leaving it, and the answer renders on screen where the person can read the
evidence rather than only the agent seeing it. Where the browser has not shipped
WebMCP a polyfill installs it, so the tools are there either way.

It registers one tool that is not a command and has no counterpart here:
dug_investigate. It takes a question, a target and a list of commands, runs them
in order, and leaves every screen on the page under the question that produced
them. The plan is not a preset and there is no menu of investigations: the
calling model writes the sequence out of the same grammar every other tool uses,
which is the part a model is good at and a lookup table is not.

Reach for it when someone has described a symptom rather than named a lookup —
mail going to spam, a dns change that has not landed, a host that is slow —
because the answer is which four lookups to run and what it means when one
disagrees with the others.

It is deliberately not served here. This endpoint could run the same commands
and return the same payloads, and an agent here can already do that by calling
them itself. What it cannot do is leave the evidence somewhere a person is
looking, which is the only thing the investigation adds.

    const tools = await document.modelContext.getTools()
    await document.modelContext.executeTool(tool, '{"target":"github.com"}')

document.modelContext is canonical; navigator.modelContext is a deprecated alias
and may be absent. The root element carries data-webmcp, which is "registered"
once the tools are up, and data-webmcp-server pointing at /mcp. Read that
attribute before concluding a page has no tools.

## Limits

`)

	fmt.Fprintf(&b, "- at most %d upstream queries per request\n", screen.MaxUpstream)
	fmt.Fprintf(&b, "- resolvers are a fixed list of %d and cannot be pointed elsewhere: ", len(resolvers.List))
	names := make([]string, 0, len(resolvers.List))
	for _, resolver := range resolvers.List {
		names = append(names, resolver.Name+" ("+resolver.IP+")")
	}
	b.WriteString(strings.Join(names, ", "))
	b.WriteString("\n")
	fmt.Fprintf(&b, "- outbound ports are an allowlist: %s. PORTS waives it, and only it\n", portList())
	b.WriteString("- every destination is validated immediately before connect, so private, loopback, link-local and reserved space is unreachable through this tool\n")
	b.WriteString("- PORTS completes a TCP handshake, so it appears in the target's logs as a connection from this deployment\n")

	b.WriteString("\n## Deliberately not here\n\n")
	for _, entry := range commands.NotHere {
		fmt.Fprintf(&b, "- %s — %s\n", entry[0], entry[1])
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// Read by agents running in a page on some other origin as often as by
	// anything server-side.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, s-maxage=86400, stale-while-revalidate=172800")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, b.String())
}

func portList() string {
	// AllowedPorts walks a map, so the order is not stable. This document is
	// cached for a day and diffed by people; sort it.
	ports := guard.AllowedPorts()
	sort.Ints(ports)
	out := make([]string, 0, len(ports))
	for _, port := range ports {
		out = append(out, itoa(port))
	}
	return strings.Join(out, ", ")
}

func itoa(n int) string { return fmt.Sprint(n) }
