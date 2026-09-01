package dnsx

import (
	"context"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/miekg/dns"

	"github.com/zaidmukaddam/dug/pkg/guard"
	"github.com/zaidmukaddam/dug/pkg/resolvers"
)

// maxHops bounds the walk. Real delegation is three or four hops deep, so
// anything past this is a server handing us labels, and every hop costs a
// sequential query at the full timeout.
const maxHops = 12

// walkBudget bounds the walk's wall clock. It is deliberately far below the
// function's own maxDuration: a walk that has spent this long is being led
// somewhere by a server that will not answer, and waiting longer buys nothing.
const walkBudget = 20 * time.Second

// Where the walk starts and what port it speaks to. Vars rather than constants
// only so a test can stand up a hostile authoritative server on an unprivileged
// port and prove the guard still fires on the addresses the walk is handed.
// Nothing in production assigns them.
var (
	walkRoot = resolvers.Roots[0]
	walkPort = "53"
)

// Delegation is one hop of the root to authoritative walk.
type Delegation struct {
	Zone          string
	Server        string
	ServerIP      string
	Nameservers   []string
	DS            []string
	MS            int
	Authoritative bool
	Err           string
}

// Walk follows the delegation from a root server down to the authoritative
// zone, iteratively, so the result is what the delegation says rather than
// what one recursive resolver decided to cache.
func Walk(ctx context.Context, name string) []Delegation {
	// maxHops bounds how many times round, not how long each time takes: a hop
	// can spend the timeout twice over on a truncated reply and again on a glue
	// lookup. Nothing upstream carries a deadline, so the walk brings its own.
	ctx, cancel := context.WithTimeout(ctx, walkBudget)
	defer cancel()

	var hops []Delegation
	serverName, serverIP := walkRoot.Name, walkRoot.IP

	for _, zone := range ZoneCuts(name)[1:] {
		if len(hops) >= maxHops || ctx.Err() != nil {
			break
		}

		hop := Delegation{Zone: zone, Server: serverName, ServerIP: serverIP}
		started := time.Now()

		msg := new(dns.Msg)
		msg.SetQuestion(dns.Fqdn(zone), dns.TypeNS)
		msg.SetEdns0(1232, true)
		msg.RecursionDesired = false

		client := &dns.Client{Timeout: Timeout, Net: "udp"}
		reply, _, err := client.ExchangeContext(ctx, msg, net.JoinHostPort(serverIP, walkPort))
		if err == nil && reply != nil && reply.Truncated {
			tcp := &dns.Client{Timeout: Timeout, Net: "tcp"}
			if full, _, tcpErr := tcp.ExchangeContext(ctx, msg, net.JoinHostPort(serverIP, walkPort)); tcpErr == nil {
				reply = full
			}
		}

		if err != nil {
			hop.Err = err.Error()
			hop.MS = int(time.Since(started).Milliseconds())
			hops = append(hops, hop)
			break
		}

		hop.Authoritative = reply.Authoritative
		for _, section := range [][]dns.RR{reply.Answer, reply.Ns} {
			for _, rr := range section {
				switch record := rr.(type) {
				case *dns.NS:
					hop.Nameservers = append(hop.Nameservers, strings.TrimSuffix(record.Ns, "."))
				case *dns.DS:
					hop.DS = append(hop.DS, recordText(record))
				}
			}
		}
		hop.Nameservers = unique(hop.Nameservers)

		nextIP, nextName, refused := nextServer(ctx, reply, hop.Nameservers)
		if refused != nil {
			hop.Err = "refused: " + refused.Error()
		} else if nextIP != "" {
			serverIP, serverName = nextIP, nextName
		}

		hop.MS = int(time.Since(started).Milliseconds())
		hops = append(hops, hop)

		if hop.Authoritative || hop.Err != "" {
			break
		}
	}

	// The loop ends on a referral, so the authoritative server has been named
	// but never asked. Ask it, otherwise the walk stops one hop short.
	if len(hops) > 0 && !hops[len(hops)-1].Authoritative && hops[len(hops)-1].Err == "" {
		final := Delegation{Zone: name, Server: serverName, ServerIP: serverIP}
		started := time.Now()

		msg := new(dns.Msg)
		msg.SetQuestion(dns.Fqdn(name), dns.TypeNS)
		msg.SetEdns0(1232, true)

		client := &dns.Client{Timeout: Timeout, Net: "udp"}
		if reply, _, err := client.ExchangeContext(ctx, msg, net.JoinHostPort(serverIP, walkPort)); err == nil {
			final.Authoritative = reply.Authoritative
			for _, rr := range reply.Answer {
				if ns, ok := rr.(*dns.NS); ok {
					final.Nameservers = append(final.Nameservers, strings.TrimSuffix(ns.Ns, "."))
				}
			}
			final.Nameservers = unique(final.Nameservers)
		} else {
			final.Err = err.Error()
		}
		final.MS = int(time.Since(started).Milliseconds())
		hops = append(hops, final)
	}

	return hops
}

// nextServer picks the address to ask for the next hop: glue from the
// additional section when the referral carries it, otherwise an A lookup for
// the first delegated nameserver.
//
// Both are published by the zone being walked, so this is the one destination
// in this package that is not a constant, and the guard has to approve it
// before anything dials it. A refusal is returned rather than skipped, because
// a walk that quietly ends at the last good hop looks like a delegation that
// simply stops.
func nextServer(ctx context.Context, reply *dns.Msg, nameservers []string) (ip, name string, err error) {
	for _, rr := range reply.Extra {
		if a, ok := rr.(*dns.A); ok {
			ip, name = a.A.String(), strings.TrimSuffix(a.Hdr.Name, ".")
			break
		}
	}
	if ip == "" && len(nameservers) > 0 {
		glue := Query(ctx, nameservers[0], "A", resolvers.Default.IP)
		if len(glue.Records) > 0 {
			ip, name = glue.Records[0], nameservers[0]
		}
	}
	if ip == "" {
		return "", "", nil
	}
	if _, err := guard.CheckString(ip); err != nil {
		return "", "", err
	}
	return ip, name, nil
}

func unique(values []string) []string {
	seen := map[string]bool{}
	out := values[:0]
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

// DNSKEY algorithm numbers, IANA registry.
var DNSKEYAlgorithms = map[int]string{
	5: "RSA/SHA-1", 7: "RSASHA1-NSEC3-SHA1", 8: "RSA/SHA-256", 10: "RSA/SHA-512",
	13: "ECDSA P-256/SHA-256", 14: "ECDSA P-384/SHA-384", 15: "Ed25519", 16: "Ed448",
}

var DSDigests = map[int]string{1: "SHA-1", 2: "SHA-256", 3: "GOST", 4: "SHA-384"}

// Key describes one DNSKEY, with the tag miekg computes for us.
type Key struct {
	Flags     int
	Algorithm int
	Role      string
	Tag       int
}

// ParseDNSKEYs re-parses the rendered records so the key tag can be computed.
// miekg's KeyTag implements RFC 4034 appendix B, which is worth not writing.
func ParseDNSKEYs(name string, records []string) []Key {
	keys := make([]Key, 0, len(records))
	for _, record := range records {
		rr, err := dns.NewRR(dns.Fqdn(name) + " IN DNSKEY " + record)
		if err != nil {
			continue
		}
		key, ok := rr.(*dns.DNSKEY)
		if !ok {
			continue
		}
		role := "zone signing"
		if key.Flags == 257 {
			role = "key signing"
		}
		keys = append(keys, Key{
			Flags:     int(key.Flags),
			Algorithm: int(key.Algorithm),
			Role:      role,
			Tag:       int(key.KeyTag()),
		})
	}
	return keys
}
