// Package mailx evaluates SPF, DKIM and DMARC configuration.
//
// Alignment here is the published policy, not an observed result: without a
// message in hand there is no such thing as observed alignment.
package mailx

import (
	"context"
	"strings"

	"github.com/zaidmukaddam/dug/pkg/dnsx"
)

// RFC 7208 4.6.4: ten DNS-querying mechanisms across the whole evaluation.
const LookupLimit = 10

// QueryCeiling is a hard stop on the queries one walk may issue, far above
// what the lookup limit can reach, so a hostile record cannot run away even if
// the lookup accounting is ever wrong.
const QueryCeiling = 64

var Querying = map[string]bool{
	"include": true, "a": true, "mx": true, "ptr": true, "exists": true, "redirect": true,
}

// Selectors cannot be enumerated from DNS, so this is a fixed list of the ones
// large senders use. Bounded fan-out.
var Selectors = []string{
	"default", "google", "selector1", "selector2", "k1", "k2", "k3",
	"mail", "dkim", "s1", "s2", "smtp", "mandrill", "zoho",
	"sendgrid", "mailjet", "protonmail", "fm1", "everlytickey1", "pic",
}

var AlignmentModes = map[string]string{
	"r": "relaxed, a subdomain still aligns",
	"s": "strict, the domain must match exactly",
}

type Term struct {
	Qualifier string
	Mechanism string
	Value     string
}

func ParseSPF(records []string) string {
	for _, record := range records {
		if strings.HasPrefix(strings.ToLower(record), "v=spf1") {
			return record
		}
	}
	return ""
}

func Terms(record string) []Term {
	fields := strings.Fields(record)
	if len(fields) == 0 {
		return nil
	}
	out := make([]Term, 0, len(fields))
	for _, raw := range fields[1:] {
		if raw == "" {
			continue
		}
		qualifier := "+"
		text := raw
		if strings.ContainsAny(text[:1], "+-~?") {
			qualifier, text = text[:1], text[1:]
		}
		name, value, found := strings.Cut(text, ":")
		if !found {
			if key, rest, eq := strings.Cut(text, "="); eq {
				name, value = key, rest
			}
		}
		out = append(out, Term{qualifier, strings.ToLower(name), value})
	}
	return out
}

// Terminal reads the qualifier on the final `all`, from the end.
func Terminal(record string) string {
	terms := Terms(record)
	for i := len(terms) - 1; i >= 0; i-- {
		if terms[i].Mechanism == "all" {
			return terms[i].Qualifier
		}
	}
	return "?"
}

func AllMeaning(qualifier string) string {
	switch qualifier {
	case "-":
		return "hard fail, everything else is rejected"
	case "~":
		return "soft fail, everything else is marked but delivered"
	case "?":
		return "neutral, no assertion, which is the same as no record"
	case "+":
		return "pass everything, which defeats the point of spf"
	}
	return "unknown qualifier"
}

type Node struct {
	Label    string
	Meta     string
	Accent   bool
	Children []Node
}

// Walk expands include and redirect, counting lookups against the limit.
type Walk struct {
	ResolverIP string
	Lookups    int
	Void       int
	Queries    int
	seen       map[string]bool
}

func NewWalk(resolverIP string) *Walk {
	return &Walk{ResolverIP: resolverIP, seen: map[string]bool{}}
}

func (w *Walk) Domains() int { return len(w.seen) }

func (w *Walk) Expand(ctx context.Context, domain string, depth int) Node {
	node := Node{Label: domain}

	// The limit has to bound the walk, not just annotate it. A wildcard TXT
	// record answering every subdomain with ten fresh includes fans out
	// exponentially otherwise. The caller keeps counting past the limit, so
	// how far over a domain is stays reportable.
	if w.Lookups > LookupLimit {
		node.Meta = "not expanded, past the limit of " + itoa(LookupLimit)
		return node
	}
	if w.Queries >= QueryCeiling {
		node.Meta = "not expanded, too many queries"
		return node
	}

	if w.seen[domain] {
		node.Meta = "already expanded, skipped to avoid a loop"
		return node
	}
	w.seen[domain] = true

	if depth > 6 {
		node.Meta = "nesting deeper than six, stopped"
		return node
	}

	answer := dnsx.Query(ctx, domain, "TXT", w.ResolverIP)
	w.Queries++
	record := ParseSPF(dnsx.TXTStrings(answer))
	if record == "" {
		w.Void++
		node.Meta = "no spf record"
		return node
	}

	counted := 0
	for _, term := range Terms(record) {
		if term.Mechanism == "all" {
			node.Children = append(node.Children, Node{
				Label: term.Qualifier + "all",
				Meta:  AllMeaning(term.Qualifier),
			})
			continue
		}

		if !Querying[term.Mechanism] {
			label := term.Mechanism
			if term.Value != "" {
				label += ":" + term.Value
			}
			node.Children = append(node.Children, Node{Label: label, Meta: "no lookup"})
			continue
		}

		counted++
		w.Lookups++
		over := w.Lookups > LookupLimit

		if (term.Mechanism == "include" || term.Mechanism == "redirect") && term.Value != "" {
			child := w.Expand(ctx, term.Value, depth+1)
			child.Label = term.Mechanism + ":" + term.Value
			child.Accent = over
			switch {
			case over:
				child.Meta = "lookup " + itoa(w.Lookups) + ", past the limit of " + itoa(LookupLimit)
			case child.Meta == "":
				child.Meta = "lookup " + itoa(w.Lookups)
			}
			node.Children = append(node.Children, child)
			continue
		}

		label := term.Mechanism
		if term.Value != "" {
			label += ":" + term.Value
		}
		meta := "lookup " + itoa(w.Lookups)
		if over {
			meta += ", past the limit"
		}
		node.Children = append(node.Children, Node{Label: label, Meta: meta, Accent: over})
	}

	node.Meta = itoa(counted) + " querying mechanisms"
	return node
}

func ParseDMARC(records []string) map[string]string {
	for _, record := range records {
		if !strings.HasPrefix(strings.ToLower(record), "v=dmarc1") {
			continue
		}
		tags := map[string]string{}
		for _, part := range strings.Split(record, ";") {
			key, value, found := strings.Cut(strings.TrimSpace(part), "=")
			if found || key != "" {
				tags[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
			}
		}
		return tags
	}
	return map[string]string{}
}

type Selector struct {
	Name   string
	Record string
}

// ProbeDKIM tries the fixed selector list: twenty lookups, bounded.
func ProbeDKIM(ctx context.Context, domain, resolverIP string) []Selector {
	jobs := make([]dnsx.Job, 0, len(Selectors))
	for _, selector := range Selectors {
		jobs = append(jobs, dnsx.Job{Name: selector + "._domainkey." + domain, Type: "TXT", Resolver: resolverIP})
	}
	answers := dnsx.QueryMany(ctx, jobs, 10)

	found := make([]Selector, 0, 4)
	for i, answer := range answers {
		for _, text := range dnsx.TXTStrings(answer) {
			if strings.Contains(text, "p=") {
				found = append(found, Selector{Name: Selectors[i], Record: text})
				break
			}
		}
	}
	return found
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
