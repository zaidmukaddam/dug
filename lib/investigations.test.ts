import assert from "node:assert/strict"
import { test } from "node:test"

import { expandSteps, matchInvestigation } from "./investigations.ts"

test("a step written with {target} runs once per target, in target order", () => {
  assert.deepEqual(expandSteps(["MAIL {target}", "SPF {target}"], ["a.com", "b.com"]), [
    "MAIL a.com",
    "SPF a.com",
    "MAIL b.com",
    "SPF b.com",
  ])
})

test("a step without the placeholder passes through unchanged", () => {
  assert.deepEqual(expandSteps(["SRC"], ["a.com", "b.com"]), ["SRC"])
})

test("WHY with a comma separated list matches several targets", () => {
  const match = matchInvestigation("WHY mail a.com, b.com")
  assert.equal(match?.ok, true)
  if (match?.ok) {
    assert.deepEqual(match.targets, ["a.com", "b.com"])
  }
})
