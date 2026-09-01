// Package dnsx runs DNS queries against the fixed resolver list.
//
// Query only ever talks to a constant from internal/resolvers, so it needs no
// address guard. Walk is the exception: it learns the next nameserver address
// from the reply it was just handed, so every candidate it follows goes
// through the guard first.
package dnsx

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/sync/errgroup"

	"github.com/zaidmukaddam/dug/pkg/resolvers"
)

const Timeout = 4 * time.Second

// RFC 1035, the presentation form without the trailing dot.
const maxNameLength = 253

var RecordTypes = []string{"A", "AAAA", "CNAME", "MX", "TXT", "NS", "SOA", "CAA", "DS", "DNSKEY"}

// Answer is one resolver's reply for one name and type.
type Answer struct {
	Name          string
	Type          string
	Resolver      string
	Records       []string
	TTL           int
	MS            int
	Rcode         string
	Err           string
	Authenticated bool
}

func (a Answer) OK() bool    { return a.Err == "" && a.Rcode == "NOERROR" }
func (a Answer) Empty() bool { return len(a.Records) == 0 }

// Fingerprint is a stable string for comparing two resolvers' answers. Sorted,
// because record order inside an rrset is not meaningful and several resolvers
// rotate it deliberately.
func (a Answer) Fingerprint() string {
	if a.Err != "" {
		return "!" + a.Err
	}
	if a.Rcode != "NOERROR" {
		return "!" + a.Rcode
	}
	sorted := append([]string(nil), a.Records...)
	sort.Strings(sorted)
	return strings.Join(sorted, "\n")
}

// What a name may contain once IDNA has done its work: letters, digits, hyphen,
// dot, and underscore. The underscore is not legal in a hostname and is exactly
// what the records this tool exists to look up need — _dmarc, _acme-challenge,
// selector._domainkey — which is also why STD3 rules are off in idna.go.
//
// That leniency is why this check has to exist. With STD3 off, idna passes a
// slash, a colon and a space straight through, so "http://example.com" became a
// name, was queried as one, and came back "the name does not exist". Refusing
// the input is the honest answer; claiming a real domain is missing is not.
var validName = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$`)

func beforeSlash(value string) string {
	host, _, _ := strings.Cut(value, "/")
	return host
}

// ToName normalises user input to a DNS name, converting unicode through IDNA
// so the punycode form is what goes on the wire.
//
// Errors name the fix rather than the fault, because every one of these is
// something a person can correct in a keystroke once they know which keystroke.
func ToName(value string) (string, error) {
	text := strings.TrimSuffix(strings.TrimSpace(value), ".")
	if text == "" {
		return "", fmt.Errorf("empty name")
	}

	// The three shapes people actually paste, each answered with what to type
	// instead. Checked before IDNA so the suggestion is in their own spelling,
	// and only offered when the thing being suggested is itself a name —
	// otherwise "../etc" earns the advice "try ..".
	if _, rest, found := strings.Cut(text, "://"); found {
		if host := beforeSlash(rest); validName.MatchString(host) {
			return "", fmt.Errorf("that is a url, and this takes a name. try %s", host)
		}
	}
	if host, _, found := strings.Cut(text, "/"); found && validName.MatchString(host) {
		return "", fmt.Errorf("a name has no path. try %s", host)
	}
	if host, port, found := strings.Cut(text, ":"); found && port != "" && validName.MatchString(host) {
		return "", fmt.Errorf("a name has no port. try %s", host)
	}

	ascii, err := idnaToASCII(text)
	if err != nil {
		return "", err
	}
	if !validName.MatchString(ascii) {
		return "", fmt.Errorf("a name is letters, digits, hyphens and dots, and this has something else in it")
	}
	// Punycode expands, so the limit is only meaningful on the wire form. An
	// over-long name is refused here rather than turned into one query per
	// label further down.
	if len(ascii) > maxNameLength {
		return "", fmt.Errorf("name too long: %d bytes, limit %d", len(ascii), maxNameLength)
	}
	return ascii, nil
}

// DisplayForms returns the unicode and punycode forms, equal when ASCII. Both
// are shown whenever they differ, so a name that renders as one thing and
// resolves as another is visible rather than hidden.
func DisplayForms(value string) (unicode, ascii string) {
	ascii, err := ToName(value)
	if err != nil {
		return value, value
	}
	unicode, err = idnaToUnicode(ascii)
	if err != nil {
		return ascii, ascii
	}
	return unicode, ascii
}

func Query(ctx context.Context, name, rtype, resolverIP string) Answer {
	answer := Answer{Name: name, Type: rtype, Resolver: resolverIP, Rcode: "NOERROR"}

	qtype, ok := dns.StringToType[rtype]
	if !ok {
		answer.Err = "unknown record type " + rtype
		return answer
	}

	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(name), qtype)
	msg.SetEdns0(1232, true) // DO bit, so DNSSEC-aware answers come back
	msg.RecursionDesired = true

	server := net.JoinHostPort(resolverIP, "53")
	client := &dns.Client{Timeout: Timeout, Net: "udp"}
	started := time.Now()
	reply, _, err := client.ExchangeContext(ctx, msg, server)

	// Retry over TCP when the answer did not fit. A domain with many TXT
	// records overflows even an EDNS0 buffer, and taking the truncated reply
	// at face value silently reports no records at all.
	if err == nil && reply != nil && reply.Truncated {
		tcp := &dns.Client{Timeout: Timeout, Net: "tcp"}
		if full, _, tcpErr := tcp.ExchangeContext(ctx, msg, server); tcpErr == nil {
			reply = full
		}
	}
	answer.MS = int(time.Since(started).Milliseconds())

	if err != nil {
		if strings.Contains(err.Error(), "timeout") || ctx.Err() != nil {
			answer.Err = "timeout"
		} else {
			answer.Err = err.Error()
		}
		return answer
	}

	answer.Rcode = dns.RcodeToString[reply.Rcode]
	answer.Authenticated = reply.AuthenticatedData

	for _, rr := range reply.Answer {
		if rr.Header().Rrtype != qtype {
			continue // skip CNAMEs in the chain to the asked-for type
		}
		answer.Records = append(answer.Records, recordText(rr))
		if answer.TTL == 0 {
			answer.TTL = int(rr.Header().Ttl)
		}
	}
	sort.Strings(answer.Records)
	return answer
}

// recordText renders the rdata without the owner name, class and TTL prefix.
func recordText(rr dns.RR) string {
	text := rr.String()
	header := rr.Header().String()
	return strings.TrimPrefix(text, header)
}

type Job struct {
	Name     string
	Type     string
	Resolver string
}

// QueryMany runs jobs concurrently. The fan-out is bounded by the caller, which
// always builds jobs from the fixed resolver and record-type lists.
func QueryMany(ctx context.Context, jobs []Job, workers int) []Answer {
	answers := make([]Answer, len(jobs))
	if len(jobs) == 0 {
		return answers
	}
	if workers <= 0 || workers > len(jobs) {
		workers = len(jobs)
	}

	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(workers)
	for i, job := range jobs {
		i, job := i, job
		group.Go(func() error {
			answers[i] = Query(ctx, job.Name, job.Type, job.Resolver)
			return nil
		})
	}
	_ = group.Wait()
	return answers
}

// TXTStrings rejoins the chunked strings inside TXT records.
func TXTStrings(answer Answer) []string {
	out := make([]string, 0, len(answer.Records))
	for _, record := range answer.Records {
		parts := strings.Split(record, `" "`)
		for i := range parts {
			parts[i] = strings.Trim(parts[i], `"`)
		}
		out = append(out, strings.ReplaceAll(strings.Join(parts, ""), `\"`, `"`))
	}
	return out
}

func ReverseName(ip string) (string, error) {
	name, err := dns.ReverseAddr(ip)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(name, "."), nil
}

func MinTTL(answers []Answer) int {
	lowest := 0
	for _, answer := range answers {
		if answer.TTL > 0 && (lowest == 0 || answer.TTL < lowest) {
			lowest = answer.TTL
		}
	}
	return lowest
}

// ZoneCuts returns every zone from the root down to the name, root first.
func ZoneCuts(name string) []string {
	labels := strings.Split(strings.TrimSuffix(name, "."), ".")
	cuts := []string{"."}
	for i := len(labels) - 1; i >= 0; i-- {
		cuts = append(cuts, strings.Join(labels[i:], "."))
	}
	return cuts
}

func DefaultResolverIP() string { return resolvers.Default.IP }
