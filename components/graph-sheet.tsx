"use client"

import type { ReactNode } from "react"

import { motion, useReducedMotion } from "motion/react"

import {
  Graph,
  GraphBody,
  GraphRule,
} from "@/components/ui/graph-frame"
import {
  fadeUp,
  staggerList,
} from "@/lib/graph-motion"
import { cn } from "@/lib/utils"

type GraphAlign = "left" | "right"

type SheetSection = {
  title: string
  rows: ReactNode[][]
}

type GraphSheetProps = {
  title: string
  headers: string[]
  sections: SheetSection[]
  footer?: ReactNode[]
  align?: GraphAlign[]
  corner?: string
  className?: string
}

function side(align: GraphAlign[] | undefined, index: number): GraphAlign {
  return align?.[index] ?? (index === 0 ? "left" : "right")
}

function RuleY() {
  return (
    <span
      aria-hidden="true"
      className="pointer-events-none absolute inset-y-0 left-0 graph-rule-y @max-md:hidden"
    />
  )
}

function GraphSheet({
  title,
  headers,
  sections,
  footer,
  align,
  corner,
  className,
}: GraphSheetProps) {
  const reduce = useReducedMotion()
  const item = fadeUp(reduce)
  const list = staggerList(reduce, 0.04)
  const columns = headers.length

  // Two layouts, chosen by the sheet's own width. From @md the table is a
  // table: label beside value, values right-aligned so numbers line up. Below
  // it every cell is a block, label above value, and the value has the whole
  // width. Measured on a 375px phone: the label column sat at its content
  // width and still left 22 characters for a 24-character date, which
  // wrapped as "2021-03-22 to 2036-03-" / "21" and read as two values.
  function cellClass(index: number, extra?: string) {
    return cn(
      "relative px-3 py-2.5 text-left @max-md:block @max-md:px-0 @max-md:py-0.5",
      index === 0 && "@max-md:pt-3 @max-md:text-graph-muted",
      side(align, index) === "right" && "tabular-nums @md:text-right",
      extra
    )
  }

  return (
    <Graph title={title} className={className} corner={corner}>
      <GraphBody className="px-3 py-6 sm:px-6 sm:py-8">
        <div className="@container graph-scroll-x">
          <table className="w-full min-w-lg border-separate border-spacing-0">
            <thead>
              {/* Column headers name columns, and stacked cells have none. */}
              <tr className="@max-md:hidden">
                {headers.map((header, index) => (
                  <th
                    key={index}
                    className={cn(
                      "relative px-3 pb-3 text-left font-normal whitespace-nowrap text-foreground",
                      side(align, index) === "right" && "@md:text-right"
                    )}
                  >
                    {index > 0 ? <RuleY /> : null}
                    {header}
                  </th>
                ))}
              </tr>
              <tr>
                <th colSpan={columns} className="p-0">
                  <GraphRule />
                </th>
              </tr>
            </thead>
            {sections.map((section, sectionIndex) => (
              <motion.tbody
                initial={reduce ? false : "hidden"}
                // Keyed by position: section titles are data and repeat, as a
                // TLS chain with two intermediates does.
                key={sectionIndex}
                variants={list}
                viewport={{ once: true, amount: "some" }}
                whileInView="show"
              >
                {sectionIndex > 0 ? (
                  <tr>
                    <td colSpan={columns} className="pt-4 pb-1">
                      <GraphRule />
                    </td>
                  </tr>
                ) : null}
                <tr>
                  <td
                    className="px-3 pt-3 pb-1 text-graph-muted"
                    colSpan={columns}
                  >
                    {section.title}
                  </td>
                </tr>
                {section.rows.map((row, rowIndex) => (
                  <motion.tr key={rowIndex} variants={item}>
                    {row.map((cell, cellIndex) => (
                      <td
                        className={cellClass(cellIndex, "whitespace-nowrap")}
                        key={cellIndex}
                      >
                        {cellIndex > 0 ? <RuleY /> : null}
                        {cell}
                      </td>
                    ))}
                  </motion.tr>
                ))}
              </motion.tbody>
            ))}
            {footer ? (
              <tfoot>
                <tr>
                  <td colSpan={columns} className="pt-3 pb-3">
                    <GraphRule />
                  </td>
                </tr>
                <tr>
                  {footer.map((cell, cellIndex) => (
                    <td
                      className={cellClass(cellIndex, "whitespace-nowrap")}
                      key={cellIndex}
                    >
                      {cellIndex > 0 ? <RuleY /> : null}
                      {cell}
                    </td>
                  ))}
                </tr>
              </tfoot>
            ) : null}
          </table>
        </div>
      </GraphBody>
    </Graph>
  )
}

export { GraphSheet }
export type { GraphAlign, GraphSheetProps, SheetSection }
