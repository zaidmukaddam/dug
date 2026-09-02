// The pretty path back into the line a person would have typed, for the
// shared-link redirect in proxy.ts. Kept free of next/* imports so it can be
// tested with node --test.

// decodeURIComponent throws on a malformed escape, and a thrown error in the
// proxy is a platform 500 for the one request. The raw segment goes through
// instead: the line only ever reaches the command parser, which refuses what
// it cannot read.
function decodeSegment(segment: string): string {
  try {
    return decodeURIComponent(segment)
  } catch {
    return segment
  }
}

// NET is the one command whose argument contains a slash, so its two segments
// rejoin.
export function commandLine(pathname: string): string {
  const [verb, ...rest] = pathname.split("/").filter(Boolean).map(decodeSegment)
  const args = verb === "net" ? [rest.join("/")] : rest
  return [(verb ?? "").toUpperCase(), ...args].join(" ")
}
