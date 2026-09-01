"use client"

// The one sanctioned useEffect in the codebase.
//
// Everything else that reaches for an effect is doing something else in
// disguise: deriving state that could be computed inline, fetching, or relaying
// an event. What is left is genuine synchronisation with something outside
// React — a browser API, a subscription, a third-party registry — and that is
// always the same shape: set up on mount, tear down on unmount. Naming that
// shape makes the rare real case obvious and the common wrong case impossible
// to write by accident.
//
// If the effect body needs a value that changes between renders, do not add a
// dependency: wrap the part that reads it in useEffectEvent, so the setup still
// happens once and the callback still sees the latest render.

// biome-ignore lint/style/noRestrictedImports: this hook is the sanctioned wrapper the ban points everything else at
import { useEffect, type EffectCallback } from "react"

export function useMountEffect(effect: EffectCallback) {
  // biome-ignore lint/correctness/useExhaustiveDependencies: mount only is the entire point
  useEffect(effect, [])
}
