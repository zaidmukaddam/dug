// The dashed frame, for pages that are not command screens.
//
// components/ui/graph-frame.tsx is the vendored original and stays the one the
// graph components use. This is the same visual object rebuilt for the handful
// of pages that need a frame without a payload behind it, so those pages do not
// have to reach into a registry component and drag its props along.

import { cn } from "@/lib/utils"

export function Frame({
  title,
  className,
  children,
}: {
  title: string
  className?: string
  children: React.ReactNode
}) {
  const corner =
    "pointer-events-none absolute z-10 flex size-4 items-center justify-center bg-background font-mono text-sm leading-none text-graph-frame select-none"

  return (
    <figure className={cn("graph-frame relative min-w-0 font-mono text-sm", className)}>
      <figcaption className="absolute top-0 left-1/2 z-10 -translate-x-1/2 -translate-y-1/2 bg-background px-2.5 tracking-wide whitespace-nowrap uppercase">
        <span className="text-graph-muted">[ {title} ]</span>
      </figcaption>

      <span aria-hidden="true" className={cn(corner, "top-0 left-0 -translate-x-1/2 -translate-y-1/2")}>
        +
      </span>
      <span aria-hidden="true" className={cn(corner, "top-0 right-0 translate-x-1/2 -translate-y-1/2")}>
        +
      </span>
      <span aria-hidden="true" className={cn(corner, "bottom-0 left-0 -translate-x-1/2 translate-y-1/2")}>
        +
      </span>
      <span aria-hidden="true" className={cn(corner, "right-0 bottom-0 translate-x-1/2 translate-y-1/2")}>
        +
      </span>

      {/* h-full so that a frame stretched by its grid row fills its own box.
          Without it the figure grows and the padded content area does not, and
          anything trying to sit at the bottom of a frame sits at the bottom of
          nothing. Resolves to auto wherever the row is not stretching. */}
      <div className="h-full px-5 py-7 sm:px-8 sm:py-8">{children}</div>
    </figure>
  )
}

// label on the left, value on the right, the way every spec block in this tool
// is read.
export function FrameRows({
  rows,
}: {
  rows: { label: string; value: string; accent?: boolean }[]
}) {
  return (
    <dl className="flex flex-col gap-2">
      {rows.map((row) => (
        <div key={row.label} className="grid grid-cols-[7rem_minmax(0,1fr)] gap-x-4">
          <dt className="text-graph-muted">{row.label}</dt>
          <dd className={cn("break-words", row.accent ? "text-graph-accent" : "text-foreground")}>
            {row.value}
          </dd>
        </div>
      ))}
    </dl>
  )
}
