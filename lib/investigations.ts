// Investigations: one question, several lookups, one case file.
//
// Every command answers one question about one target, which is the right shape
// for someone who already knows which question to ask. Nobody debugging mail
// knows to ask four. They know the symptom — "our invoices are going to spam" —
// and the expertise being sold by every runbook on the subject is only the
// knowledge of which lookups to run, in what order, and what it means when one
// of them disagrees with the others.
//
// That knowledge is what a model has and a lookup table does not. So the plan
// is not stored here: an agent forms it, the page runs it and keeps every
// screen under the question that produced it. The list below is the landing's
// worked examples, for a person who has arrived without an agent — not the set
// of investigations that are possible, which is every sequence of commands
// somebody can think of a reason to run.
//
// Deliberately not a server endpoint. /mcp could run the same commands and hand
// back the same payloads, and an agent there can already do that by calling
// them itself. What it cannot do is leave the evidence somewhere a person is
// looking, which is the only thing an investigation adds.

export type Investigation = {
  id: string
  // What a person would say, and what the case file is titled with.
  question: string
  // The commands, in the order someone who knew what they were doing would run
  // them. Order matters on screen: a case reads top to bottom.
  steps: (target: string) => string[]
}

// Worked examples, shown on the landing. An agent is not limited to these and
// is not offered them; it writes its own plan out of the same grammar every
// other tool uses.
export const INVESTIGATIONS: Investigation[] = [
  {
    id: "mail",
    question: "why is mail from this domain going to spam",
    steps: (target) => [`MAIL ${target}`, `SPF ${target}`, `DIG ${target} MX`, `RDAP ${target}`],
  },
  {
    id: "dns",
    question: "has my dns change actually taken effect",
    steps: (target) => [`PROP ${target}`, `TTL ${target}`, `NS ${target}`, `DNSSEC ${target}`],
  },
  {
    id: "tls",
    question: "is this site's certificate and transport healthy",
    steps: (target) => [`TLS ${target}`, `WATCH ${target}`, `HTTP ${target}`, `TRACE ${target}`],
  },
  {
    id: "reachability",
    question: "why is this host slow or unreachable",
    steps: (target) => [`PING ${target}`, `ROUTE ${target}`, `TRACE ${target}`],
  },
  {
    id: "agents",
    question: "can an agent read and understand this site",
    steps: (target) => [`SEO ${target}`, `AEO ${target}`, `WEBMCP ${target}`, `OG ${target}`],
  },
]
