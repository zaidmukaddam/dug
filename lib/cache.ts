// Cache keys and lifetimes, derived from the answer rather than guessed.
//
// Section 8: DNS carries its own cache policy, so the record's TTL is the cache
// duration, floored at 30 seconds and capped by kind on the server. The handler
// has already done that arithmetic and returns the result as `ttl`, so this
// module only has to key, store, and age.
//
// Nothing is persisted. A reload is a cold cache, which is the honest behaviour
// for a tool that keeps no history by design.

export type Block = {
  component: string
  props: Record<string, unknown>
  span?: number
}

export type Degraded = {
  source: string
  reason: string
}

export type Verdict = {
  state: "ok" | "warn" | "none"
  headline: string
  detail: string
}

// Present only on a 4xx, mirroring ErrorInfo in pkg/screen. Its presence is
// what tells the client an envelope is a refusal rather than an answer.
export type PayloadError = {
  code: string
  message: string
  hint?: string
}

export type Payload = {
  command: string
  target: string
  verdict: Verdict
  ts: number
  ttl: number
  elapsed_ms: number
  upstream_queries: number
  notes: string[]
  degraded: Degraded[]
  blocks: Block[]
  error?: PayloadError
}

export type CacheState = {
  cached: boolean
  ageMs: number
  ttlMs: number
  remaining: number
}

const store = new Map<string, Payload>()

export function cacheKey(command: string, target: string, extra?: string): string {
  return [command, target, extra ?? ""].join(" ").toLowerCase().trim()
}

export function readCache(key: string): Payload | null {
  const hit = store.get(key)
  if (!hit) {
    return null
  }

  if (Date.now() - hit.ts > hit.ttl * 1000) {
    store.delete(key)
    return null
  }

  return hit
}

export function writeCache(key: string, payload: Payload): void {
  store.set(key, payload)
}

export function clearCache(): number {
  const size = store.size
  store.clear()
  return size
}

export function cacheSize(): number {
  return store.size
}

// A stale answer presented as fresh is the failure mode this tool has instead
// of the other project's fabricated number, so every frame carries this.
export function cacheState(payload: Payload, now: number): CacheState {
  const ageMs = Math.max(0, now - payload.ts)
  const ttlMs = payload.ttl * 1000

  return {
    cached: ageMs > 1500,
    ageMs,
    ttlMs,
    remaining: ttlMs > 0 ? Math.max(0, Math.min(1, 1 - ageMs / ttlMs)) : 0,
  }
}

export function formatAge(ms: number): string {
  const seconds = Math.floor(ms / 1000)

  if (seconds < 60) {
    return `${seconds}s old`
  }

  if (seconds < 3600) {
    return `${Math.floor(seconds / 60)}m old`
  }

  return `${Math.floor(seconds / 3600)}h old`
}
