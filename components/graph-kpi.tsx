"use client"

import { motion, useReducedMotion } from "motion/react"

import {
  Graph,
  GraphBody,
  GraphTick,
  GraphTrack,
} from "@/components/ui/graph-frame"
import {
  DIM_OPACITY,
  fadeUp,
  fillDelay,
  graphTransition,
  isMonoPalette,
  resolveGlyphs,
  toneClass,
  type Glyphs,
  type GraphPalette,
} from "@/lib/graph-motion"
import { cn } from "@/lib/utils"

const SPARK_DEFAULT = ["▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"]

type GraphKpiProps = {
  title: string
  value: string
  label: string
  hint?: string
  data: number[]
  glyphs?: Glyphs
  palette?: GraphPalette
  corner?: string
  className?: string
}

function GraphKpi({
  title,
  value,
  label,
  hint,
  data,
  glyphs,
  palette,
  corner,
  className,
}: GraphKpiProps) {
  const reduce = useReducedMotion()
  const enter = fadeUp(reduce)
  const max = Math.max(...data, 1)
  const last = data.length - 1
  const set = glyphs == null ? SPARK_DEFAULT : resolveGlyphs(glyphs)
  const points = data.map((entry) => {
    const index = Math.round((entry / max) * (set.length - 1))
    return set[index] ?? set[0] ?? "▁"
  })

  return (
    <Graph title={title} className={className} corner={corner}>
      <GraphBody className="flex flex-col gap-4">
        <motion.div
          className="flex flex-col gap-2"
          initial={reduce ? false : "hidden"}
          variants={enter}
          viewport={{ once: true, amount: 0.5 }}
          whileInView="show"
        >
          <p
            className={cn(
              "text-3xl tracking-tight tabular-nums sm:text-4xl",
              toneClass(palette, "primary")
            )}
          >
            {value}
          </p>
          {/* Wraps rather than sharing one row. Side by side, two long strings
              each got half of an already narrow column and both broke across
              several lines; given the full width they usually need one each.
              They still sit on one line whenever both are short enough. */}
          <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
            <p className="min-w-0 text-graph-muted">{label}</p>
            {hint ? (
              <p className="min-w-0 text-graph-muted tabular-nums">
                {/* Inside the hint rather than between the two, so it travels
                    with it when the pair wraps instead of being left stranded
                    at the end of the line above. Without it an as name running
                    straight into "prefix ..." reads as one string. */}
                {label ? (
                  <span aria-hidden="true" className="pr-2 text-graph-faint">
                    ·
                  </span>
                ) : null}
                {hint}
              </p>
            ) : null}
          </div>
        </motion.div>
        {points.length > 0 ? (
          <GraphTrack className="justify-start gap-0.5">
            {points.map((glyph, index) => {
              const live = index === last

              return (
                <GraphTick className="flex-none" key={`${glyph}-${index}`}>
                  <motion.span
                    className={cn(
                      live
                        ? toneClass(palette, "primary")
                        : toneClass(palette, "secondary")
                    )}
                    initial={reduce ? false : { opacity: 0 }}
                    transition={graphTransition(reduce, {
                      delay: fillDelay(reduce, index),
                    })}
                    viewport={{ once: true }}
                    whileInView={{
                      opacity:
                        live || !isMonoPalette(palette) ? 1 : DIM_OPACITY,
                    }}
                  >
                    {glyph}
                  </motion.span>
                </GraphTick>
              )
            })}
          </GraphTrack>
        ) : null}
        <span className="sr-only">
          {value} {label}
          {hint ? `. ${hint}` : ""}
        </span>
      </GraphBody>
    </Graph>
  )
}

export { GraphKpi }
export type { GraphKpiProps }
