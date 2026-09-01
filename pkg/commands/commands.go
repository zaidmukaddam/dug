// Package commands is the command grammar, in Go.
//
// app/commands/grammar.ts is the same list for the browser. This copy exists
// because llms.txt, the OpenAPI document and the MCP tool list are all served
// by Go functions and all describe the same grammar; internal/wiring fails if
// the two lists drift, exactly as it does for the resolver list.
package commands

type Param struct {
	Name     string
	Required bool
	About    string
	Example  string
}

type Spec struct {
	Name     string
	Family   string
	Endpoint string
	Argument string
	Summary  string
	Example  string
	// Path is the agent- and curl-facing route. The query form stays valid;
	// this is the one people type.
	Path   string
	Params []Param
}

// target is the first argument of nearly every command. Its description
// changes with the argument kind, which is what an agent needs to send a
// valid call.
var targetAbout = map[string]string{
	"domain":   "a domain name",
	"host":     "a hostname",
	"endpoint": "a hostname or an ip address",
	"address":  "an ip address",
	"asn":      "an as number, with or without the AS prefix",
	"cidr":     "a network in cidr form, a /24 or smaller",
	"pair":     "a domain name, compared against `other`",
}

// List is every command. HELP is deliberately absent: it renders the grammar
// from the browser's own copy and has no endpoint, and llms.txt is what an
// agent should read instead.
var List = []Spec{
	{Name: "DIG", Family: "resolution", Endpoint: "/api/resolve", Argument: "domain",
		Summary: "every record type, or a single one", Example: "DIG example.com MX",
		Path:   "/dig/{target}",
		Params: []Param{{Name: "type", About: "a single record type to ask for, such as MX", Example: "MX"}}},

	{Name: "PROP", Family: "resolution", Endpoint: "/api/propagate", Argument: "domain",
		Summary: "agreement across the fixed resolver list", Example: "PROP example.com",
		Path: "/prop/{target}"},

	{Name: "TTL", Family: "resolution", Endpoint: "/api/resolve", Argument: "domain",
		Summary: "remaining lifetime per record", Example: "TTL example.com",
		Path: "/ttl/{target}"},

	{Name: "NS", Family: "delegation", Endpoint: "/api/delegate", Argument: "domain",
		Summary: "root to tld to authoritative walk", Example: "NS example.com",
		Path: "/ns/{target}"},

	{Name: "DNSSEC", Family: "delegation", Endpoint: "/api/delegate", Argument: "domain",
		Summary: "chain of trust, ds and dnskey", Example: "DNSSEC cloudflare.com",
		Path: "/dnssec/{target}"},

	{Name: "RDAP", Family: "registration", Endpoint: "/api/rdap", Argument: "domain",
		Summary: "registration data with status codes decoded", Example: "RDAP example.com",
		Path: "/rdap/{target}"},

	{Name: "WATCH", Family: "registration", Endpoint: "/api/rdap", Argument: "domain",
		Summary: "domain and certificate expiry, computed now", Example: "WATCH example.com",
		Path: "/watch/{target}"},

	{Name: "TLS", Family: "transport", Endpoint: "/api/tls", Argument: "host",
		Summary: "chain, validity spans, protocols", Example: "TLS example.com",
		Path: "/tls/{target}"},

	{Name: "HTTP", Family: "transport", Endpoint: "/api/fetch", Argument: "host",
		Summary: "headers, redirect chain, security headers", Example: "HTTP example.com",
		Path: "/http/{target}"},

	{Name: "TRACE", Family: "transport", Endpoint: "/api/fetch", Argument: "host",
		Summary: "dns, tcp, tls and ttfb timing", Example: "TRACE example.com",
		Path: "/trace/{target}"},

	{Name: "MAIL", Family: "mail", Endpoint: "/api/mail", Argument: "domain",
		Summary: "mx, spf, dkim, dmarc and alignment policy", Example: "MAIL example.com",
		Path: "/mail/{target}"},

	{Name: "SPF", Family: "mail", Endpoint: "/api/mail", Argument: "domain",
		Summary: "include tree, against the ten lookup limit", Example: "SPF example.com",
		Path: "/spf/{target}"},

	{Name: "IP", Family: "addressing", Endpoint: "/api/addr", Argument: "address",
		Summary: "reverse dns, asn, prefix, neighbours", Example: "IP 8.8.8.8",
		Path: "/ip/{target}"},

	{Name: "ASN", Family: "addressing", Endpoint: "/api/addr", Argument: "asn",
		Summary: "prefixes and address space", Example: "ASN 13335",
		Path: "/asn/{target}"},

	{Name: "NET", Family: "addressing", Endpoint: "/api/addr", Argument: "cidr",
		Summary: "address space grid, a /24 or smaller", Example: "NET 8.8.8.0/24",
		// A cidr always carries a slash, so the prefix length is its own
		// segment rather than something the caller has to percent-encode.
		Path: "/net/{target}/{bits}"},

	{Name: "PING", Family: "reachability", Endpoint: "/api/probe", Argument: "endpoint",
		Summary: "icmp echo, round trip time and packet loss", Example: "PING 1.1.1.1",
		Path:   "/ping/{target}",
		Params: []Param{{Name: "count", About: "how many echoes to send, 1 to 10", Example: "8"}}},

	{Name: "ROUTE", Family: "reachability", Endpoint: "/api/probe", Argument: "endpoint",
		Summary: "the hops between here and there, with reverse dns", Example: "ROUTE example.com",
		Path: "/route/{target}"},

	{Name: "PORTS", Family: "reachability", Endpoint: "/api/probe", Argument: "endpoint",
		Summary: "which tcp ports are open, closed or filtered", Example: "PORTS scanme.nmap.org 22,80,443",
		Path:   "/ports/{target}",
		Params: []Param{{Name: "ports", About: "ports to try, comma separated, ranges allowed", Example: "22,80,443"}}},

	{Name: "VS", Family: "meta", Endpoint: "/api/vs", Argument: "pair",
		Summary: "two domains side by side", Example: "VS example.com github.com",
		Path:   "/vs/{target}/{other}",
		Params: []Param{{Name: "other", Required: true, About: "the second domain name", Example: "github.com"}}},

	{Name: "SRC", Family: "meta", Endpoint: "/api/src", Argument: "none",
		Summary: "resolver list, cache ceilings, upstream health", Example: "SRC",
		Path: "/src"},

	{Name: "ME", Family: "addressing", Endpoint: "/api/me", Argument: "none",
		Summary: "the address this request came from", Example: "ME",
		Path: "/me"},

	{Name: "SEO", Family: "readability", Endpoint: "/api/seo", Argument: "domain",
		Summary: "what a crawler reads: title, canonical, robots, structured data",
		Example: "SEO example.com", Path: "/seo/{target}"},

	{Name: "AEO", Family: "readability", Endpoint: "/api/aeo", Argument: "domain",
		Summary: "what an answer engine reads: llms.txt, markdown, content without js",
		Example: "AEO example.com", Path: "/aeo/{target}"},

	{Name: "OG", Family: "readability", Endpoint: "/api/og", Argument: "domain",
		Summary: "the share card, with the image fetched and measured",
		Example: "OG example.com", Path: "/og/{target}"},

	{Name: "WEBMCP", Family: "readability", Endpoint: "/api/webmcp", Argument: "domain",
		Summary: "tools for an agent in the page, and the mcp surface for one that isn’t",
		Example: "WEBMCP example.com", Path: "/webmcp/{target}"},
}

// NotHere mirrors NOT_HERE in the grammar. An agent that knows what this tool
// refuses to do will not waste a call finding out.
var NotHere = [][2]string{
	{"monitoring and alerts", "nothing is stored between queries"},
	{"registrant lookup", "redacted at source, and there’s an official channel"},
	{"reaching private space", "every destination is validated, on every command"},
}

func ByName(name string) (Spec, bool) {
	for _, spec := range List {
		if spec.Name == name {
			return spec, true
		}
	}
	return Spec{}, false
}

// TargetAbout describes what this command's first argument must be.
func (s Spec) TargetAbout() string {
	if s.Argument == "none" {
		return ""
	}
	return targetAbout[s.Argument]
}

// Families in the order they first appear, so every rendering groups the same
// way the landing page does.
func Families() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, spec := range List {
		if !seen[spec.Family] {
			seen[spec.Family] = true
			out = append(out, spec.Family)
		}
	}
	return out
}
