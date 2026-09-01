import { chromium } from "playwright"
const out = process.env.SHOT_DIR
const b = await chromium.launch()
const ctx = await b.newContext({ viewport: { width: 1360, height: 900 }, deviceScaleFactor: 1 })
const p = await ctx.newPage()
await p.emulateMedia({ reducedMotion: "reduce" })
await p.goto("http://localhost:3111", { waitUntil: "networkidle" })
await p.fill('input[aria-label="command"]', process.argv[2])
await p.keyboard.press("Enter")
await p.waitForFunction(() => document.querySelectorAll("figure").length > 1, { timeout: 90000 })
await p.waitForTimeout(1200)
for (const title of process.argv.slice(3)) {
  const idx = await p.evaluate((t) => {
    const figs = [...document.querySelectorAll("figure")]
    return figs.findIndex(f => (f.querySelector("figcaption")?.innerText ?? "").includes(t))
  }, title)
  if (idx < 0) { console.log(title, "not found"); continue }
  const fig = p.locator("figure").nth(idx)
  let box = await fig.boundingBox()
  await p.evaluate((y) => window.scrollTo(0, y), Math.max(0, box.y - 50))
  await p.waitForTimeout(400)
  box = await fig.boundingBox()
  await p.screenshot({
    path: `${out}/crop-${title.toLowerCase().replace(/\W+/g,"-")}.png`,
    clip: { x: Math.max(0, box.x-8), y: Math.max(0, box.y-8), width: Math.min(box.width+16, 1352), height: Math.min(box.height+16, 870) },
  })
  console.log(`${title} h=${Math.round(box.height)}`)
}
await b.close()
