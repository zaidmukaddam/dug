import assert from "node:assert/strict"
import { test } from "node:test"

import { hostFrom, parse } from "./grammar.ts"

test("hostFrom strips a port from a hostname", () => {
  assert.equal(hostFrom("example.com:8443", "host"), "example.com")
})

test("hostFrom strips scheme, userinfo, path, query and fragment", () => {
  assert.equal(hostFrom("https://user@example.com/a?b#c", "host"), "example.com")
})

test("hostFrom keeps a cidr whole", () => {
  assert.equal(hostFrom("8.8.8.0/24", "cidr"), "8.8.8.0/24")
})

test("hostFrom keeps an ipv6 address whole", () => {
  assert.equal(hostFrom("2001:4860:4860::8888", "address"), "2001:4860:4860::8888")
  assert.equal(hostFrom("::1", "address"), "::1")
  assert.equal(hostFrom("2606:4700:4700::1111", "endpoint"), "2606:4700:4700::1111")
})

test("hostFrom unwraps a bracketed ipv6 address and drops its port", () => {
  assert.equal(hostFrom("[2001:db8::1]:443", "endpoint"), "2001:db8::1")
  assert.equal(hostFrom("https://[2001:db8::1]:8443/path", "host"), "2001:db8::1")
})

test("hostFrom returns the word unchanged when it strips to nothing", () => {
  assert.equal(hostFrom("/a/b", "host"), "/a/b")
})

test("parse accepts an ipv6 target for IP", () => {
  const result = parse("IP 2001:4860:4860::8888")
  assert.equal(result.ok, true)
  if (result.ok) {
    assert.equal(result.command.target, "2001:4860:4860::8888")
  }
})
