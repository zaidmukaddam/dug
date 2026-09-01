// One question, several lookups, one case file.
//
// Every command answers one question about one target, which suits someone who
// already knows which question to ask. Nobody debugging mail knows to ask four.
// They know the symptom, and the expertise is knowing which lookups follow from
// it — which is why the plan is not stored here. An agent writes it and the page
// runs it; the list below is only the landing's worked examples, for someone who
// arrived without one.

export type Investigation = {
  id: string
  question: string
  // In the order someone who knew what they were doing would run them. A case
  // reads top to bottom.
  steps: (target: string) => string[]
}

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
