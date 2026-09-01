"use client"

// 404, rendered the way this tool renders every other answer: a verdict and the
// evidence for it. The evidence here is the request itself.
//
// A client component because the path is the one fact worth reporting, and only
// the browser knows which one was asked for.

import { usePathname } from "next/navigation"

import { Notice } from "@/app/screens/notice"

export default function NotFound() {
  const pathname = usePathname()

  return (
    <Notice
      headline={`${pathname} doesn’t resolve`}
      detail="the route set is closed, the same way the command set is. nothing here is generated from the url."
      title="request"
      rows={[
        { label: "path", value: pathname, accent: true },
        { label: "status", value: "404" },
        { label: "reason", value: "no route is registered for this path" },
      ]}
      links={[
        { href: "/", label: "/", note: "the terminal" },
        { href: "/llms.txt", label: "/llms.txt", note: "the grammar, for agents" },
        { href: "/openapi.json", label: "/openapi.json", note: "every command, typed" },
      ]}
    />
  )
}
