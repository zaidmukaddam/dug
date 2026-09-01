// The planner: a symptom in, an investigation out.
//
// This is the one place dug calls a model itself, and it exists for a person
// without an agent. An agent in the page writes its own plan through
// dug_investigate and never touches this. Someone in an ordinary browser who
// types "why is mail from acme.com going to spam" would otherwise need to know
// that there is a topic called mail, which is the thing they do not know when
// they have the problem.
//
// Not under /api, where Vercel routes to the Go functions. Rate limited by the
// proxy like everything else that costs something.

import { openai } from "@ai-sdk/openai"
import { generateText, NoObjectGeneratedError, Output } from "ai"
import { z } from "zod"

import { COMMANDS } from "@/app/commands/grammar"
import { INVESTIGATIONS } from "@/lib/investigations"

// A public, keyless site. Nothing typed here should be stored anywhere it can
// be read back, which is also what /privacy promises.
const MODEL = "gpt-5.6-luna"

const Plan = z.object({
  question: z.string().min(1).max(200).describe("the question, in the words the person used"),
  target: z.string().min(1).max(253).describe("the bare domain or host, no scheme, no path"),
  steps: z
    .array(z.string().min(1).max(80))
    .min(1)
    .max(6)
    .describe("full command lines to run in order, three to five is usually right"),
})

const RUNNABLE = COMMANDS.filter((spec) => spec.endpoint)
const VERBS = new Set<string>(RUNNABLE.map((spec) => spec.name))

function system(): string {
  const commands = RUNNABLE.map(
    (spec) => `${spec.name} <${spec.argument}> — ${spec.summary}. e.g. ${spec.example}`
  ).join("\n")

  const examples = INVESTIGATIONS.map(
    (entry) => `"${entry.question}" -> ${JSON.stringify(entry.steps("example.com"))}`
  ).join("\n")

  return [
    "You plan a network diagnostic investigation for dug, a terminal for domain and network lookups.",
    "Given a symptom or question, choose the commands that would answer it, in the order someone who knew what they were doing would run them.",
    "",
    "Commands, one per line, with the argument each takes:",
    commands,
    "",
    "Rules:",
    "- target is a bare hostname such as example.com. Never a scheme, never a path, never a port.",
    "- every step is one full command line: the command name, then the target, then any second argument the command takes.",
    "- three to five steps. Do not pad. Do not repeat a command against the same target.",
    "- question is the person's question in their words, lowercase, no trailing question mark.",
    "- if the text names no domain or host at all, use example.com as the target.",
    "",
    "Worked examples:",
    examples,
  ].join("\n")
}

// The same three shapes ToName refuses on the Go side, cut away here so a
// model that pastes the url back does not produce a plan every step of which
// is rejected.
function bareHost(value: string): string {
  let host = value.trim().toLowerCase()
  const scheme = host.indexOf("://")
  if (scheme !== -1) host = host.slice(scheme + 3)
  host = host.split("/")[0].split(":")[0]
  return host.replace(/\.$/, "")
}

export async function POST(request: Request) {
  if (!process.env.OPENAI_API_KEY) {
    return Response.json(
      { error: "no_planner", message: "the planner is not configured on this deployment" },
      { status: 503 }
    )
  }

  const body = await request.json().catch(() => null)
  const ask = typeof body?.ask === "string" ? body.ask.trim() : ""
  if (!ask || ask.length > 500) {
    return Response.json(
      { error: "bad_ask", message: "send a question of up to 500 characters as {ask}" },
      { status: 400 }
    )
  }

  try {
    const { output } = await generateText({
      model: openai(MODEL),
      output: Output.object({ schema: Plan }),
      system: system(),
      prompt: ask,
      maxOutputTokens: 400,
      providerOptions: { openai: { store: false } },
    })

    const target = bareHost(output.target)
    // A step whose verb is not a command would only ever render as a failed
    // slot, so drop it here and say so, rather than show a plan that was never
    // going to run.
    const steps = output.steps
      .map((step) => step.trim())
      .filter((step) => VERBS.has(step.split(/\s+/)[0]?.toUpperCase() ?? ""))

    if (!target || steps.length === 0) {
      return Response.json(
        { error: "no_plan", message: "could not turn that into commands. try naming the domain" },
        { status: 422 }
      )
    }

    return Response.json({ question: output.question, target, steps })
  } catch (error) {
    if (NoObjectGeneratedError.isInstance(error)) {
      return Response.json(
        { error: "no_plan", message: "could not turn that into commands. try naming the domain" },
        { status: 422 }
      )
    }
    return Response.json(
      { error: "planner_failed", message: "the planner did not answer. try a command instead" },
      { status: 502 }
    )
  }
}
