<!-- BEGIN:nextjs-agent-rules -->

# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` (resolved from this file's directory; in monorepos the `next` package may not be visible from the repo root) before writing any code. Heed deprecation notices.

This block is written and re-added by `next dev` — verify at `node_modules/next/dist/server/lib/generate-agent-files.js`. Removing it from a diff only re-creates the uncommitted change; committing it with your work keeps the tree clean.

<!-- END:nextjs-agent-rules -->

## Commands

`pnpm dev` runs the Go API and Next together for local work; `pnpm verify` runs everything CI runs.

Touched `app/`, `components/`, `lib/`, or `hooks/`? Run `pnpm typecheck`, `pnpm lint`, and `pnpm test:web`.

Touched `pkg/`, `api/`, or `cmd/`? Run `pnpm vet` and `pnpm test:api`.

Touched a mirrored pair (see below) or a route? Run `pnpm build` too, since that is what catches a route Next cannot resolve.

## Vercel Go constraints

Go packages shared across functions live in `pkg/`, not `internal/`. Vercel compiles each `api/<route>/index.go` inside a synthetic module named `handler`, so an import of `internal/` is a cross-module import and Go refuses it.

Each `api/<route>/` directory exports exactly one `Handler`, because that is all Go allows per package.

The HTTP handler lives at `api/fetch`, not `api/http`, because `api/http` would shadow the standard library package of the same name.

## Mirrors that must move together

`pkg/commands` and `app/commands/grammar.ts` are the same grammar in two languages; a drift between them is pinned by a test in `pkg/wiring`.

`pkg/resolvers` and `lib/resolvers.ts` are the same resolver list; a drift between them is pinned by a test in `pkg/wiring`.

`next.config.ts`'s Link headers and `pkg/screen.ServiceLinks` name the same relations; a drift between them is pinned by a test in `pkg/wiring`.

The API version and rate-limit constants live once, in `pkg/screen`; a drift from what a client is told is pinned by a test in `pkg/wiring`.

## Copy rules

Verdicts and row values render lowercase; a verdict comes first in its sentence.

No em dashes; use a comma, a colon, or a period instead. Apostrophes are curly.

An absent value renders as `none`, not a blank cell or a plain dash.

## Decided tradeoffs

Biome lints, Prettier formats; Prettier's Tailwind plugin sorts the class lists, so do not reorder them by hand.

`useEffect` is banned outside `hooks/use-mount-effect.ts`; reach for that wrapper instead of writing a new one.

The vendored `components/graph-*` files are not patched; a fix there is upstreamed or worked around, not diffed in place.

There is no browser end to end suite; `pnpm test:web` and `pnpm test:api` are what CI runs.

Vercel deploys from `main`.
