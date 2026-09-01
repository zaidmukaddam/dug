// VS. Two domains side by side.
package handler

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/zaidmukaddam/dug/pkg/certs"
	"github.com/zaidmukaddam/dug/pkg/dnsx"
	"github.com/zaidmukaddam/dug/pkg/mailx"
	"github.com/zaidmukaddam/dug/pkg/resolvers"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

type profile struct {
	name      string
	answers   map[string]dnsx.Answer
	spf       string
	dmarc     string
	handshake certs.Handshake
	lookupMS  int
	minTTL    int
}

func (p profile) has(rtype string) bool {
	answer, ok := p.answers[rtype]
	return ok && len(answer.Records) > 0
}

func Handler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	left := query.Get("target")
	right := query.Get("other")
	if left == "" || right == "" {
		screen.Fail(w, r, "VS", left, "vs needs two domains", "try VS example.com github.com")
		return
	}

	leftName, err := dnsx.ToName(left)
	if err != nil {
		screen.Fail(w, r, "VS", left, left+" is not a domain name", err.Error())
		return
	}
	rightName, err := dnsx.ToName(right)
	if err != nil {
		screen.Fail(w, r, "VS", right, right+" is not a domain name", err.Error())
		return
	}

	result := screen.New("VS", leftName+" "+rightName)
	run(r, result, leftName, rightName)
	result.Write(w, r)
}

func run(r *http.Request, result *screen.Result, leftName, rightName string) {
	ctx := r.Context()

	var left, right profile
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { left = build(groupCtx, leftName); return nil })
	group.Go(func() error { right = build(groupCtx, rightName); return nil })
	_ = group.Wait()

	result.Spend(28)
	ttl := left.minTTL
	if right.minTTL > 0 && (ttl == 0 || right.minTTL < ttl) {
		ttl = right.minTTL
	}
	result.HoldTTL(ttl, "dns")

	rows := []screen.CompareRow{
		{Label: "a record", Values: []any{left.has("A"), right.has("A")}},
		{Label: "ipv6", Values: []any{left.has("AAAA"), right.has("AAAA")}},
		{Label: "mail exchangers", Values: []any{left.has("MX"), right.has("MX")}},
		{Label: "spf", Values: []any{left.spf != "", right.spf != ""}},
		{Label: "dmarc", Values: []any{left.dmarc != "", right.dmarc != ""}},
		{Label: "dnssec", Values: []any{left.has("DS"), right.has("DS")}},
		{Label: "caa", Values: []any{left.has("CAA"), right.has("CAA")}},
		{Label: "tls handshake", Values: []any{left.handshake.Err == "", right.handshake.Err == ""}},
		{Label: "certificate valid", Values: []any{left.handshake.Verified, right.handshake.Verified}},
		{Label: "tls 1.3", Values: []any{offers(left, "TLS 1.3"), offers(right, "TLS 1.3")}},
		{Label: "tls 1.0 refused", Values: []any{!offers(left, "TLS 1.0"), !offers(right, "TLS 1.0")}},
		{Label: "nameservers", Values: []any{nsCount(left), nsCount(right)}},
		{Label: "cert issuer", Values: []any{issuer(left), issuer(right)}},
	}

	differences := 0
	for _, row := range rows {
		if fmt.Sprint(row.Values[0]) != fmt.Sprint(row.Values[1]) {
			differences++
		}
	}

	if differences == 0 {
		result.SetVerdict("ok",
			fmt.Sprintf("%s and %s match on all %d checks", leftName, rightName, len(rows)),
			"a dash is an absence, not a fault")
	} else {
		result.SetVerdict("none",
			fmt.Sprintf("%s and %s differ on %d of %d checks", leftName, rightName, differences, len(rows)),
			"a dash is an absence, not a fault")
	}

	result.Add("GraphCompare", screen.CompareProps{
		Title: "side by side", Columns: []string{leftName, rightName}, Rows: rows,
	}, 3)

	var diffRows []screen.DiffRow
	for _, rtype := range []string{"A", "NS", "MX"} {
		ours := map[string]bool{}
		theirs := map[string]bool{}
		if left.has(rtype) {
			for _, record := range left.answers[rtype].Records {
				ours[record] = true
			}
		}
		if right.has(rtype) {
			for _, record := range right.answers[rtype].Records {
				theirs[record] = true
			}
		}
		all := make([]string, 0, len(ours)+len(theirs))
		for record := range ours {
			all = append(all, record)
		}
		for record := range theirs {
			if !ours[record] {
				all = append(all, record)
			}
		}
		sort.Strings(all)
		for _, record := range all {
			sign := "add"
			switch {
			case ours[record] && theirs[record]:
				sign = "keep"
			case ours[record]:
				sign = "remove"
			}
			diffRows = append(diffRows, screen.DiffRow{Label: lower(rtype), Value: record, Sign: sign})
		}
	}
	if len(diffRows) > 16 {
		diffRows = diffRows[:16]
	}
	if len(diffRows) == 0 {
		diffRows = []screen.DiffRow{{Label: "-", Value: "neither domain returned records", Sign: "keep"}}
	}
	result.Add("GraphDiff", screen.DiffProps{
		Title: "records", Rows: diffRows,
		Footer: &screen.DiffRow{
			Label: "removed is left only, added is right only",
			Value: leftName + " vs " + rightName,
		},
	}, 2)

	slopeItems := []screen.SlopeItem{
		{Label: "mean lookup ms", From: left.lookupMS, To: right.lookupMS},
		{Label: "handshake ms", From: left.handshake.MS, To: right.handshake.MS},
		{Label: "shortest ttl", From: left.minTTL, To: right.minTTL},
	}
	if len(left.handshake.Chain) > 0 && len(right.handshake.Chain) > 0 {
		now := time.Now()
		slopeItems = append(slopeItems, screen.SlopeItem{
			Label: "cert days left",
			From:  left.handshake.Chain[0].DaysLeft(now),
			To:    right.handshake.Chain[0].DaysLeft(now),
		})
	}
	result.Add("GraphSlope", screen.SlopeProps{
		Title: "numbers", FromLabel: leftName, ToLabel: rightName, Items: slopeItems,
	}, 1)

	for _, side := range []profile{left, right} {
		if side.handshake.Err != "" {
			result.Degrade("tls "+side.name, side.handshake.Err)
		}
	}

	result.Note("a dash means the record or capability was not found, which is not the same as one domain being worse configured than the other. both columns were queried the same way at the same moment.")
}

func build(ctx context.Context, name string) profile {
	types := []string{"A", "AAAA", "MX", "NS", "TXT", "DS", "CAA", "SOA"}
	jobs := make([]dnsx.Job, 0, len(types)+1)
	for _, rtype := range types {
		jobs = append(jobs, dnsx.Job{Name: name, Type: rtype, Resolver: resolvers.Default.IP})
	}
	jobs = append(jobs, dnsx.Job{Name: "_dmarc." + name, Type: "TXT", Resolver: resolvers.Default.IP})

	answers := dnsx.QueryMany(ctx, jobs, 9)
	out := profile{name: name, answers: map[string]dnsx.Answer{}}

	total := 0
	for i, answer := range answers {
		total += answer.MS
		if i < len(types) {
			out.answers[types[i]] = answer
		}
	}
	out.lookupMS = total / len(answers)
	out.minTTL = dnsx.MinTTL(answers)

	if txt, ok := out.answers["TXT"]; ok {
		out.spf = mailx.ParseSPF(dnsx.TXTStrings(txt))
	}
	for _, record := range dnsx.TXTStrings(answers[len(answers)-1]) {
		if len(record) > 8 && lower(record[:8]) == "v=dmarc1" {
			out.dmarc = record
			break
		}
	}

	out.handshake = certs.Connect(ctx, name, 443, 8*time.Second)
	return out
}

func offers(p profile, label string) bool {
	value, ok := p.handshake.Protocols[label]
	return ok && value != nil && *value
}

func nsCount(p profile) string {
	if !p.has("NS") {
		return "none"
	}
	return strconv.Itoa(len(p.answers["NS"].Records))
}

func issuer(p profile) string {
	if len(p.handshake.Chain) == 0 {
		return "none"
	}
	return p.handshake.Chain[0].Issuer
}

func lower(s string) string {
	out := []rune(s)
	for i, ch := range out {
		if ch >= 'A' && ch <= 'Z' {
			out[i] = ch + 32
		}
	}
	return string(out)
}
