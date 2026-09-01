"use client"

import * as React from "react"
import { ThemeProvider as NextThemesProvider, useTheme } from "next-themes"

import { useMountEffect } from "@/hooks/use-mount-effect"

function ThemeProvider({
  children,
  ...props
}: React.ComponentProps<typeof NextThemesProvider>) {
  return (
    <NextThemesProvider
      attribute="class"
      defaultTheme="system"
      enableSystem
      disableTransitionOnChange
      {...props}
    >
      <ThemeHotkey />
      {children}
    </NextThemesProvider>
  )
}

function isTypingTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) {
    return false
  }

  return (
    target.isContentEditable ||
    target.tagName === "INPUT" ||
    target.tagName === "TEXTAREA" ||
    target.tagName === "SELECT"
  )
}

function ThemeHotkey() {
  const { resolvedTheme, setTheme } = useTheme()

  // The toggle reads the theme at keypress time rather than at subscribe time.
  // Without this the listener would name resolvedTheme as a dependency and tear
  // itself down and back up on every toggle, which is a lot of ceremony for a
  // window listener that should be attached exactly once.
  const toggle = React.useEffectEvent(() => {
    setTheme(resolvedTheme === "dark" ? "light" : "dark")
  })

  useMountEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.defaultPrevented || event.repeat) {
        return
      }

      if (event.metaKey || event.ctrlKey || event.altKey) {
        return
      }

      if (event.key.toLowerCase() !== "d") {
        return
      }

      if (isTypingTarget(event.target)) {
        return
      }

      toggle()
    }

    window.addEventListener("keydown", onKeyDown)

    return () => {
      window.removeEventListener("keydown", onKeyDown)
    }
  })

  return null
}

// The hotkey above is unreachable from a cold page: the command input carries
// autoFocus, so focus is already on an INPUT and isTypingTarget refuses. This
// is the same control with a surface you can actually hit.
//
// The label is the noun, not the state. Naming the state would mean reading
// resolvedTheme, which is undefined during the server render and would either
// mismatch on hydration or need a mounted flag to suppress it; neither is worth
// it for a word on a button that toggles either way.
function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme()

  return (
    <button
      type="button"
      title="press d for the same thing, when the prompt does not have focus"
      className="underline-offset-4 hover:text-foreground hover:underline"
      onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
    >
      theme
    </button>
  )
}

export { ThemeProvider, ThemeToggle }
