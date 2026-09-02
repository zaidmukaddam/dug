import assert from "node:assert/strict"
import { beforeEach, test } from "node:test"

import { cacheSize, clearCache, readCache, writeCache, type Payload } from "./cache.ts"

function payload(overrides: Partial<Payload> = {}): Payload {
  return {
    command: "DIG",
    target: "example.com",
    verdict: { state: "ok", headline: "", detail: "" },
    ts: Date.now(),
    ttl: 60,
    elapsed_ms: 0,
    upstream_queries: 1,
    notes: [],
    degraded: [],
    blocks: [],
    ...overrides,
  }
}

// The store is module-level, so tests share it unless cleared between runs.
beforeEach(() => {
  clearCache()
})

test("writing past capacity evicts the oldest key", () => {
  for (let i = 0; i < 101; i++) {
    writeCache(`key-${i}`, payload())
  }
  assert.equal(cacheSize(), 100)
  assert.equal(readCache("key-0"), null)
  assert.notEqual(readCache("key-100"), null)
})

test("an expired entry is swept on any write", () => {
  writeCache("stale", payload({ ts: Date.now() - 5000, ttl: 1 }))
  assert.equal(cacheSize(), 1)
  writeCache("unrelated", payload())
  // Checked via size, not readCache, so the assertion exercises the sweep in
  // writeCache rather than readCache's own expiry check on the way out.
  assert.equal(cacheSize(), 1)
})

test("readCache returns a fresh entry unchanged", () => {
  const written = payload({ target: "fresh.example" })
  writeCache("fresh", written)
  assert.deepEqual(readCache("fresh"), written)
})
