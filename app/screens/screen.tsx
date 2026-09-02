"use client"

// One layout for every command. Handlers name the component and the span; this
// is a registry lookup and a grid.

import { useState, type ComponentType } from "react"

import { GraphBars } from "@/components/graph-bars"
import { GraphBullet } from "@/components/graph-bullet"
import { GraphCells } from "@/components/graph-cells"
import { GraphCheck } from "@/components/graph-check"
import { GraphCompare } from "@/components/graph-compare"
import { GraphCountdown } from "@/components/graph-countdown"
import { GraphDiff } from "@/components/graph-diff"
import { GraphFlow } from "@/components/graph-flow"
import { GraphFunnel } from "@/components/graph-funnel"
import { GraphGantt } from "@/components/graph-gantt"
import { GraphHeatmap } from "@/components/graph-heatmap"
import { GraphKpi } from "@/components/graph-kpi"
import { GraphMatrix } from "@/components/graph-matrix"
import { GraphMeter } from "@/components/graph-meter"
import { GraphPlot } from "@/components/graph-plot"
import { GraphRank } from "@/components/graph-rank"
import { GraphSheet } from "@/components/graph-sheet"
import { GraphSlope } from "@/components/graph-slope"
import { GraphSpark } from "@/components/graph-spark"
import { GraphSpec } from "@/components/graph-spec"
import { GraphStack } from "@/components/graph-stack"
import { GraphStat } from "@/components/graph-stat"
import { GraphTable } from "@/components/graph-table"
import { GraphTimeline } from "@/components/graph-timeline"
import { GraphTimer } from "@/components/graph-timer"
import { GraphTree } from "@/components/graph-tree"
import { GraphUptime } from "@/components/graph-uptime"
import { GraphWaffle } from "@/components/graph-waffle"
import { GraphWaterfall } from "@/components/graph-waterfall"
import { cacheState, formatAge, type Block, type Payload } from "@/lib/cache"
import { useGraphNow } from "@/lib/graph-clock"
import { cn } from "@/lib/utils"

// Activity, Calendar and Invoice are absent by design: two need history, one
// needs a billing relationship.
const REGISTRY = {
  GraphBars,
  GraphBullet,
  GraphCells,
  GraphCheck,
  GraphCompare,
  GraphCountdown,
  GraphDiff,
  GraphFlow,
  GraphFunnel,
  GraphGantt,
  GraphHeatmap,
  GraphKpi,
  GraphMatrix,
  GraphMeter,
  GraphPlot,
  GraphRank,
  GraphSheet,
  GraphSlope,
  GraphSpark,
  GraphSpec,
  GraphStack,
  GraphStat,
  GraphTable,
  GraphTimeline,
  GraphTimer,
  GraphTree,
  GraphUptime,
  GraphWaffle,
  GraphWaterfall,
} as unknown as Record<string, ComponentType<Record<string, unknown>>>

const SPAN_CLASS: Record<number, string> = {
  1: "lg:col-span-1",
  2: "lg:col-span-2",
  3: "lg:col-span-3",
}

// Minimum columns each component needs before its content overflows the frame.
//
// The tables sat at 3 to avoid a horizontal scrollbar. They wrap now, so two
// columns is enough and the grid packs far denser vertically: a tall table can
// share a row with a stack of short frames instead of forcing its own.
// The track components still need ~440px for their fixed label and display
// columns plus a 20 to 24 tick glyph track.
const MIN_SPAN: Record<string, number> = {
  GraphTable: 2,
  GraphSheet: 2,
  GraphMatrix: 2,
  GraphCompare: 2,
  GraphInvoice: 2,
  GraphBullet: 2,
  GraphRank: 2,
  GraphFunnel: 2,
  GraphWaterfall: 2,
  GraphBars: 2,
}

function spanFor(block: Block): number {
  return Math.max(block.span ?? 1, MIN_SPAN[block.component] ?? 1)
}


function BlockFrame({ block }: { block: Block }) {
  const Component = REGISTRY[block.component]

  if (!Component) {
    return (
      <GraphSpec
        title="unrenderable"
        rows={[
          { label: "component", value: block.component, accent: true },
          { label: "reason", value: "not in the screen registry" },
        ]}
      />
    )
  }

  return <Component {...block.props} />
}

function withInstant(block: Block, ts: number): Block {
  if (block.props.at === null || block.props.at === undefined) {
    if (block.component === "GraphTimer") {
      return { ...block, props: { ...block.props, at: ts } }
    }
  }
  return block
}

export function Screen({ payload }: { payload: Payload }) {
  // Null until mounted, so a cached answer never reads as live on first paint.
  const now = useGraphNow(1000)

  const state = cacheState(payload, now ?? payload.ts)
  const settled = now !== null

  return (
    <section className="flex flex-col gap-8">
      <Verdict payload={payload} state={state} settled={settled} />

      {/* Dense so a later one-column block backfills the gap a wide block
          leaves. DOM order, and so reading order, is unchanged.
          `items-start` because grid items otherwise stretch to the tallest in
          their row, which left short frames like the countdown carrying two
          hundred pixels of empty space. */}
      <div className="grid grid-cols-1 items-start gap-6 lg:grid-flow-dense lg:grid-cols-3">
        {payload.blocks.map((block, index) => (
          <div
            key={`${block.component}-${index}`}
            className={cn("min-w-0", SPAN_CLASS[spanFor(block)] ?? SPAN_CLASS[1])}
          >
            <BlockFrame block={withInstant(block, payload.ts)} />
          </div>
        ))}

        {/* Live answers read 100% here, so the badge covers those instead. */}
        {state.cached ? (
          <div className="min-w-0 lg:col-span-1">
            <GraphMeter
              title="cache"
              value={state.remaining}
              caption={`${formatAge(state.ageMs)}, ${Math.round(state.remaining * payload.ttl)}s of ${payload.ttl}s left`}
            />
          </div>
        ) : null}
      </div>

      {/* Provenance and limits, folded. Every screen ends with two or three of
          these and on a phone they were the tallest thing after the verdict.
          A native details keeps them one tap away with no state to manage,
          and the text representation still prints them in full. */}
      {payload.notes.length > 0 ? (
        <footer className="pt-4 text-xs leading-relaxed text-muted-foreground">
          <details className="group">
            <summary className="cursor-pointer list-none select-none hover:text-foreground [&::-webkit-details-marker]:hidden">
              <span className="font-mono">
                <span className="group-open:hidden">[+]</span>
                <span className="hidden group-open:inline">[-]</span>
              </span>{" "}
              {payload.notes.length} {payload.notes.length === 1 ? "note" : "notes"} on how this
              was measured
            </summary>
            <div className="flex flex-col gap-2 pt-3">
              {payload.notes.map((note, index) => (
                <p key={index} className="max-w-3xl text-pretty">
                  {note}
                </p>
              ))}
            </div>
          </details>
        </footer>
      ) : null}
    </section>
  )
}

// Glyph and word both carry the state; colour only reinforces.
const VERDICT_GLYPH: Record<string, string> = { ok: "[x]", warn: "[!]", none: "[ ]" }

function Verdict({
  payload,
  state,
  settled,
}: {
  payload: Payload
  state: ReturnType<typeof cacheState>
  settled: boolean
}) {
  const verdict = payload.verdict
  const glyph = VERDICT_GLYPH[verdict?.state ?? "none"] ?? "[ ]"

  return (
    <header className="flex flex-col gap-4">
      {/* The command line reads as a label above the answer rather than as a
          row of statistics competing with it. */}
      <div className="flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1 text-xs text-muted-foreground">
        <div className="flex flex-wrap items-baseline gap-x-2">
          <span className="font-mono text-foreground">
            {payload.command} {payload.target}
          </span>
          <span aria-hidden="true">·</span>
          <CacheBadge
            cached={state.cached}
            age={state.ageMs}
            ttl={payload.ttl}
            settled={settled}
          />
          <span aria-hidden="true">·</span>
          <span className="tabular-nums">{payload.elapsed_ms}ms</span>
          <span aria-hidden="true">·</span>
          <span>
            <span className="tabular-nums">{payload.upstream_queries}</span> lookups
          </span>
        </div>
        <CopyButton payload={payload} />
      </div>

      {/* The verdict leads, but it is a sentence and not a masthead. At display
          scale it read as the page and pushed the evidence below the fold,
          which inverts what the screen is for. */}
      <div className="flex flex-col gap-1.5">
        <p className="flex items-baseline gap-3 text-xl leading-tight text-balance">
          <span
            aria-hidden="true"
            className={cn(
              "shrink-0 font-mono text-sm",
              verdict?.state === "ok" ? "text-graph-accent" : "text-muted-foreground"
            )}
          >
            {glyph}
          </span>
          <span>{verdict?.headline ?? "answered"}</span>
        </p>

        {verdict?.detail ? (
          <p className="max-w-3xl pl-9 text-sm text-pretty text-muted-foreground">
            {verdict.detail}
          </p>
        ) : null}
      </div>
    </header>
  )
}

function CacheBadge({
  cached,
  age,
  ttl,
  settled,
}: {
  cached: boolean
  age: number
  ttl: number
  settled: boolean
}) {
  // Section 11: pass and fail never rely on colour alone, and neither does
  // this. The glyph carries the state and the accent only reinforces it.
  if (!settled) {
    return <span className="text-muted-foreground">[ ] reading clock</span>
  }

  return (
    <span className={cn(cached ? "text-muted-foreground" : "text-graph-accent")}>
      {cached ? `[~] cached ${formatAge(age)}` : `[*] live, held ${ttl}s`}
    </span>
  )
}

function CopyButton({ payload }: { payload: Payload }) {
  const [copied, setCopied] = useState(false)

  return (
    <button
      type="button"
      className="text-xs text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
      onClick={() => {
        void navigator.clipboard
          .writeText(JSON.stringify(payload, null, 2))
          .then(() => {
            setCopied(true)
            window.setTimeout(() => setCopied(false), 1600)
          })
          .catch(() => setCopied(false))
      }}
    >
      {copied ? "[x] copied" : "[ ] copy json"}
    </button>
  )
}
