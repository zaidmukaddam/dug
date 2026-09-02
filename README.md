# dug

[dug.sh](https://dug.sh), a monospace, command-driven terminal for domain and network diagnostics. Every screen leads with the answer in a sentence; the graphs below it are the evidence. Nothing is precomputed or stored between queries.

The same twenty-five commands are served four ways from one implementation: as a browser app, as plain text to `curl`, as an MCP server, and as WebMCP tools on the page itself. An agent in the tab calls a tool and the answer renders on screen, so the person watching reads the same evidence the agent got, not a transcript of what it claims to have found.

```bash
curl dug.sh/tls/github.com          # a terminal gets text
curl dug.sh/aeo/example.com         # …the same url, the same answer
```

```js
// an agent in the page gets tools, and its answers land on screen
const tools = await document.modelContext.getTools()   // 27
await document.modelContext.executeTool(tool, '{"target":"github.com"}')
```

## Commands

| Family | Commands |
| --- | --- |
| resolution | `DIG` `PROP` `TTL` |
| delegation | `NS` `DNSSEC` |
| registration | `RDAP` `WATCH` |
| transport | `TLS` `HTTP` `TRACE` |
| mail | `MAIL` `SPF` |
| addressing | `IP` `ASN` `NET` `ME` |
| reachability | `PING` `ROUTE` `PORTS` |
| readability | `SEO` `AEO` `OG` |
| meta | `VS` `SRC` `HELP` |

```
DIG example.com MX          PORTS scanme.nmap.org 22,80,443
PROP cloudflare.com         PING 1.1.1.1 8
NET 8.8.8.0/24              VS example.com github.com
```

Tab completes, arrow keys walk history.

`PING` and `ROUTE` are real ICMP over an unprivileged datagram socket, which needs no capability; where a sandbox refuses it, both render the refusal. `PORTS` is a TCP connect scan and completes the handshake, so it appears in the target’s logs as a connection from this deployment’s egress address.

## Agents and curl

Every command is a GET with no key and no signup, and answers in whichever of three representations the caller asked for. Terminal clients get text, browsers get the app, everything else gets JSON.

```bash
curl dug.sh/tls/github.com
curl dug.sh/dig/example.com/MX
curl -H 'Accept: application/json' dug.sh/mail/github.com
```

The query form the browser uses stays valid, so `/api/tls?command=TLS&target=…` answers the same. Force a representation with `?format=text` or `?format=json`. Responses carry `Vary: Accept, User-Agent`. Without it, a shared cache serves one client’s representation to another.

| Route | Is |
| --- | --- |
| `/llms.txt` | The grammar, the envelope, and the limits |
| `/openapi.json` | OpenAPI 3.1, one operation per command |
| `/.well-known/api-catalog` | RFC 9727 linkset, one anchor per command |
| `/.well-known/ai-catalog.json` | AI Catalog 1.0, both surfaces typed by protocol |
| `/.well-known/mcp/server-card.json` | SEP-1649 server card: every tool, without connecting |
| `/server.json` | MCP server manifest, the registry’s own shape |
| `/mcp` | MCP server, Streamable HTTP, one tool per command |
| `/deprecation` | How a route is retired, and how much notice you get |

Every response names the first three as `Link` relations (`service-desc`, `service-doc`, `describedby`) on the API and on the HTML pages alike, so a caller holding any single response can find the rest. `/api/mcp` is the same endpoint as `/mcp` and still answers.

The MCP server is stateless and issues no session: nothing is stored between queries anywhere else here either. Its tools dispatch to the same handlers the HTTP routes use, so there’s no second implementation to drift.

The browser app registers one tool that has no counterpart on the server: `dug_investigate`. An agent calls it with a question, a target and a list of commands, its own plan, not a preset, and each screen is left on the page, in order, under the question that produced them. Someone debugging mail knows the symptom, not that the answer takes four lookups; that’s the part a model supplies and a page is good at displaying. The remote server could run the same commands and return the same payloads, and can’t leave the evidence anywhere a person is looking, which is the only thing an investigation adds.

WebMCP has moved twice since this began, which is why it registered nothing for a while: the entry point is `document.modelContext`, not `navigator.modelContext`, and `registerTool(tool, { signal })` with an `AbortController` for removal replaced `provideContext({ tools })`. `@mcp-b/global` installs the API where a browser hasn’t shipped it and wraps the native one where it has. Native WebMCP also requires an origin-isolated document, which is what the `Origin-Agent-Cluster: ?1` header in `next.config.ts` is for.

The root element carries `data-webmcp="registered" | "unsupported" | "failed"` and `data-webmcp-server`. The state comes from `getTools()` after registration, not from whether each call resolved, because React mounts the effect twice in development and the second pass gets "already registered" for tools that are present and working.

`pkg/commands` is the grammar in Go and `app/commands/grammar.ts` is the grammar for the browser; `pnpm test:api` fails if they drift, the same way it does for the resolver list.

## Running

```bash
pnpm install
pnpm dev
```

| Script | Does |
| --- | --- |
| `pnpm dev` | Go API on `:8787` and Next together |
| `pnpm test:api` | Guard vectors and wiring checks, offline |
| `pnpm test:live` | Network-touching tests: registry spread, TLS chains, ICMP |
| `pnpm lint` | Biome, with its next and react domains |
| `pnpm typecheck` | `tsc --noEmit` |
| `pnpm vet` | `go vet`, and `gofmt` that fails on an unformatted file |
| `pnpm verify` | Everything CI runs: typecheck, lint, vet, offline tests, build |
| `node scripts/shoot.mjs "TLS github.com"` | Screenshot a screen, `THEME=dark` for dark |

Biome lints and Prettier formats; its Tailwind plugin sorts the class lists.

## Layout

```
app/              terminal viewport, command grammar, screen renderer
app/not-found.tsx 404, rendered as a failed lookup, not a dead end
app/error.tsx     the route error boundary, in the same visual language
api/<route>/      Vercel Go functions, one exported Handler each
api/llms          llms.txt, generated from pkg/commands
api/openapi       OpenAPI 3.1, generated from pkg/commands
api/catalog       RFC 9727 api-catalog, one anchor per command
api/aicatalog     AI Catalog, the mcp and rest surfaces typed by protocol
api/server        server.json, the MCP registry manifest
api/servercard    the SEP-1649 server card, generated from pkg/mcpx
api/mcp           MCP server, dispatching to the other handlers
pkg/mcpx          the server identity and tool list the card and mcp share
pkg/guard         address validation, used by every dialer
pkg/commands      the grammar, mirrored by app/commands/grammar.ts
pkg/screen        the block envelope, and its json and text renderings
pkg/              dnsx, certs, httpx, rdap, mailx, icmpx, epp
components/       markdown-graphs, copied in via shadcn registry
lib/webmcp.ts     the same commands, for an agent inside the page
```

The shared Go packages are `pkg/` and not `internal/`. Vercel compiles each `api/<route>/index.go` inside a synthetic module named `handler`, so an import of `internal/` is a cross-module import and Go refuses it: `use of internal package ... not allowed`. The name is the whole fix.

Handlers marshal typed structs into blocks naming a component and its props; the frontend maps blocks to components and doesn’t transform data.

Handlers live one per directory because Go allows a single `Handler` per package. The URLs are unaffected.

## The guard

Every destination is checked in `net.Dialer.Control`, which runs after resolution and immediately before connect. There’s no window between the check and the connection for a second DNS answer, and it fires for each candidate address during Happy Eyeballs. `Unmap()` runs before every predicate, and addresses carrying an embedded IPv4 (NAT64, 6to4) are judged by that inner address.

Only `PORTS` waives the port allowlist, and it waives nothing else.

## License

MIT. See [LICENSE](LICENSE). The OpenAPI document at `/openapi.json` declares the same, so the served description and the repository agree.
