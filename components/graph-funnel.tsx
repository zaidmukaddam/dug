"use client"

import { motion, useReducedMotion } from "motion/react"

import {
  Graph,
  GraphBody,
  GraphTick,
  GraphTrack,
} from "@/components/ui/graph-frame"
import {
  fadeUp,
  isMonoPalette,
  seriesClass,
  seriesDim,
  staggerList,
  trackMarks,
  type Glyphs,
  type GraphPalette,
} from "@/lib/graph-motion"

type FunnelStep = {
  label: string
  value: number
  display?: string
}

type GraphFunnelProps = {
  title: string
  steps: FunnelStep[]
  ticks?: number
  stage?: string
  glyphs?: Glyphs
  palette?: GraphPalette
  corner?: string
  className?: string
}

function GraphFunnel({
  title,
  steps,
  ticks = 20,
  stage,
  glyphs,
  palette,
  corner,
  className,
}: GraphFunnelProps) {
  const reduce = useReducedMotion()
  const item = fadeUp(reduce)
  const list = staggerList(reduce, 0.05)
  const max = Math.max(...steps.map((step) => step.value), 1)
  const head = steps[0]?.value || 1
  const marks = trackMarks(glyphs)

  return (
    <Graph title={title} className={className} corner={corner}>
      <GraphBody>
        <motion.ol
          className="flex flex-col gap-3"
          initial={reduce ? false : "hidden"}
          role="list"
          variants={list}
          viewport={{ once: true, amount: "some" }}
          whileInView="show"
        >
          {steps.map((step, index) => {
            const width = Math.max(1, Math.round((step.value / max) * ticks))
            const percent = Math.round((step.value / head) * 100)
            const focused = Boolean(stage) && step.label === stage
            const dim = Boolean(stage) && !focused

            return (
              <motion.li
                // Same phone layout as the bullet: label on its own row, the
                // two value columns sized to their content.
                className="grid grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-x-4 sm:grid-cols-[7rem_minmax(0,1fr)_8ch_4ch]"
                key={index}
                style={seriesDim(palette, !dim)}
                variants={item}
              >
                <span className="col-span-3 truncate text-foreground sm:col-span-1">
                  {step.label}
                </span>
                <GraphTrack>
                  {Array.from({ length: ticks }, (_, cell) => {
                    const filled = cell < width

                    return (
                      <GraphTick
                        className={
                          filled
                            ? isMonoPalette(palette)
                              ? "text-graph-accent"
                              : seriesClass(palette, index)
                            : "text-graph-frame"
                        }
                        key={cell}
                      >
                        {filled ? marks.fill : marks.empty}
                      </GraphTick>
                    )
                  })}
                </GraphTrack>
                <span className="text-right text-foreground tabular-nums">
                  {step.display ?? step.value.toLocaleString()}
                </span>
                <span className="text-right text-graph-muted tabular-nums">
                  {index === 0 ? "" : `${percent}%`}
                </span>
              </motion.li>
            )
          })}
        </motion.ol>
      </GraphBody>
    </Graph>
  )
}

export { GraphFunnel }
export type { FunnelStep, GraphFunnelProps }
