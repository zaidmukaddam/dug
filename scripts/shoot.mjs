// Drive the terminal and screenshot each screen. Development only.
//   node scripts/shoot.mjs "TLS example.com" "MAIL github.com"
//
// Reduced motion is emulated because the graphs reveal on scroll via motion,
// and a screenshot taken before the reveal latches captures frames whose
// content sits at opacity zero. The library honours prefers-reduced-motion by
// rendering instantly, which is also exactly what a reduced-motion user gets,
// so the capture is a real rendering rather than a race with the animation.

import { chromium } from "playwright"

const out = process.env.SHOT_DIR ?? "/tmp"
const base = process.env.BASE ?? "http://localhost:3000"
const commands = process.argv.slice(2)

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1440, height: 1100 } })
await page.emulateMedia({ reducedMotion: "reduce" })
const errors = []
page.on(
  "console",
  (m) => m.type() === "error" && !m.text().includes("webpack-hmr") && errors.push(m.text())
)
page.on("pageerror", (e) => errors.push(String(e)))

async function fresh() {
  // Fresh page per command so each capture is one screen, the way a person
  // sees it, rather than a stack of every answer taken so far.
  await page.goto(base, { waitUntil: "networkidle" })
  if (process.env.THEME === "dark") {
    // The command input autofocuses and the theme hotkey rightly ignores keys
    // typed into it, so blur before pressing.
    await page.evaluate(() => (document.activeElement instanceof HTMLElement ? document.activeElement.blur() : null))
    await page.keyboard.press("d")
    await page.waitForTimeout(300)
  }
}

if (commands.length === 0) {
  await fresh()
  await page.screenshot({ path: `${out}/landing.png`, fullPage: true })
  console.log("landing.png")
} else {
  for (const command of commands) {
    await fresh()
    await page.fill('input[aria-label="command"]', command)
    await page.keyboard.press("Enter")
    await page
      .waitForFunction(() => document.querySelectorAll("figure").length > 1, { timeout: 120000 })
      .catch(() => {})
    await page.waitForTimeout(800)
    const slug =
      command.split(/\s+/)[0].toLowerCase() + (process.env.THEME === "dark" ? "-dark" : "")
    await page.screenshot({ path: `${out}/${slug}.png`, fullPage: true })
    console.log(`${slug}.png  figures=${await page.locator("figure").count()}`)
  }
}

if (errors.length) {
  console.log("CONSOLE ERRORS:")
  for (const error of errors.slice(0, 8)) console.log(`  ${error}`)
}

await browser.close()
