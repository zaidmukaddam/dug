import assert from "node:assert/strict"
import { test } from "node:test"

import { commandLine } from "./command-line.ts"

test("a command path becomes the typed line", () => {
  assert.equal(commandLine("/tls/github.com"), "TLS github.com")
  assert.equal(commandLine("/dig/example.com/MX"), "DIG example.com MX")
  assert.equal(commandLine("/src"), "SRC")
})

test("net rejoins its prefix length", () => {
  assert.equal(commandLine("/net/8.8.8.0/24"), "NET 8.8.8.0/24")
})

test("a percent-encoded segment is decoded", () => {
  assert.equal(commandLine("/ports/example.com/22%2C80"), "PORTS example.com 22,80")
})

test("a malformed escape falls back to the raw segment instead of throwing", () => {
  assert.doesNotThrow(() => commandLine("/tls/100%"))
  assert.equal(commandLine("/tls/100%"), "TLS 100%")
  assert.equal(commandLine("/dig/%zz"), "DIG %zz")
})
