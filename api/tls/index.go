// TLS. Chain, validity spans, protocols, timing.
package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/zaidmukaddam/dug/pkg/certs"
	"github.com/zaidmukaddam/dug/pkg/dnsx"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

var weakProtocols = []string{"TLS 1.0", "TLS 1.1"}

func Handler(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	if target == "" {
		screen.Fail(w, r, "TLS", "", "no host given", "this command needs a hostname")
		return
	}

	name, err := dnsx.ToName(target)
	if err != nil {
		screen.Fail(w, r, "TLS", target, target+" is not a hostname", err.Error())
		return
	}

	result := screen.New("TLS", name)
	run(r, result, name)
	result.Write(w, r)
}

func run(r *http.Request, result *screen.Result, name string) {
	handshake := certs.Connect(r.Context(), name, 443, 8*time.Second)
	result.Spend(5)
	now := time.Now()

	if handshake.Err != "" {
		result.Degrade("tls", handshake.Err)
		result.SetVerdict("warn", name+" did not complete a tls handshake", handshake.Err)
		result.Add("GraphSpec", screen.SpecProps{Title: "handshake", Rows: []screen.SpecRow{
			{Label: "host", Value: name, Accent: true},
			{Label: "result", Value: "no handshake"},
			{Label: "reason", Value: handshake.Err},
		}}, 3)
		result.HoldTTL(0, "tls")
		return
	}

	var leaf *certs.Cert
	if len(handshake.Chain) > 0 {
		leaf = &handshake.Chain[0]
		result.HoldTTL(int(leaf.NotAfter.Sub(now).Seconds()), "tls")
	} else {
		result.HoldTTL(0, "tls")
	}

	var weak []string
	for _, label := range weakProtocols {
		if offered, ok := handshake.Protocols[label]; ok && offered != nil && *offered {
			weak = append(weak, label)
		}
	}

	switch {
	case leaf == nil:
		result.SetVerdict("warn", name+" served no certificate", "the handshake completed without a chain")
	case !handshake.Verified:
		result.SetVerdict("warn", name+" has a certificate that does not validate", firstNonEmpty(handshake.VerifyError, "the chain did not verify"))
	case leaf.DaysLeft(now) < 0:
		result.SetVerdict("warn", fmt.Sprintf("%s has an expired certificate", name),
			fmt.Sprintf("it lapsed %d days ago", -leaf.DaysLeft(now)))
	case leaf.DaysLeft(now) < 14:
		result.SetVerdict("warn", fmt.Sprintf("%s has a certificate expiring in %d days", name, leaf.DaysLeft(now)),
			"issued by "+leaf.Issuer)
	default:
		detail := handshake.Version + " to " + leaf.Issuer
		if len(weak) > 0 {
			detail += ", but " + joinAnd(weak) + " are still offered"
		}
		result.SetVerdict("ok", fmt.Sprintf("%s has a valid certificate for %d more days", name, leaf.DaysLeft(now)), detail)
	}

	spans, ticks := certs.GanttItems(handshake.Chain)
	items := make([]screen.GanttItem, 0, len(spans))
	for _, span := range spans {
		items = append(items, screen.GanttItem{Label: span.Label, Start: span.Start, End: span.End, Accent: span.Accent})
	}
	if len(items) == 0 {
		items = []screen.GanttItem{{Label: "no chain", Start: 0, End: 1}}
		ticks = []string{"", "", "", "", ""}
	}
	result.Add("GraphGantt", screen.GanttProps{
		Title:    "validity spans",
		Items:    items,
		Ticks:    ticks,
		Stage:    fmt.Sprintf("%d certificates", len(handshake.Chain)),
		Progress: certs.NowFraction(handshake.Chain, now),
	}, 3)

	verified := "yes"
	if !handshake.Verified {
		verified = "no, " + firstNonEmpty(handshake.VerifyError, "chain did not validate")
	}
	result.Add("GraphSpec", screen.SpecProps{Title: "handshake", Rows: []screen.SpecRow{
		{Label: "host", Value: name, Accent: true},
		{Label: "address", Value: handshake.IP},
		{Label: "version", Value: firstNonEmpty(handshake.Version, "unknown")},
		{Label: "cipher", Value: handshake.Cipher},
		{Label: "alpn", Value: firstNonEmpty(handshake.ALPN, "none offered")},
		{Label: "verified", Value: verified},
	}}, 1)

	if leaf != nil {
		cap := certs.MaxLifetimeDays(leaf.NotBefore)
		total := leaf.DaysTotal()
		maxTrack := cap
		if total > maxTrack {
			maxTrack = total
		}
		left := leaf.DaysLeft(now)
		if left < 0 {
			left = 0
		}
		result.Add("GraphBullet", screen.BulletProps{Title: "lifetime", Items: []screen.BulletItem{
			{Label: "issued for", Value: total, Target: cap, Max: maxTrack, Display: fmt.Sprintf("%dd of %dd", total, cap)},
			{Label: "remaining", Value: left, Target: 30, Max: total, Display: fmt.Sprintf("%dd", leaf.DaysLeft(now))},
		}}, 1)

		result.Add("GraphCountdown", screen.CountdownProps{
			Title:   "leaf expires",
			To:      leaf.NotAfter.Format(time.RFC3339),
			Done:    "expired",
			Caption: "issued by " + leaf.Issuer,
		}, 1)
	}

	protocolItems := make([]screen.CheckItem, 0, len(certs.Protocols))
	for _, entry := range certs.Protocols {
		offered := handshake.Protocols[entry.Label]
		note := "the client could not test this version"
		done := false
		if offered != nil {
			done = *offered
			if done {
				note = "offered"
			} else {
				note = "not offered"
			}
		}
		protocolItems = append(protocolItems, screen.CheckItem{Label: entry.Label, Done: done, Note: note})
	}
	result.Add("GraphCheck", screen.CheckProps{Title: "protocols", Items: protocolItems}, 1)

	tls13 := handshake.Protocols["TLS 1.3"]
	hygiene := []screen.CheckItem{
		{Label: "chain validates", Done: handshake.Verified, Note: firstNonEmpty(handshake.VerifyError, "against the system trust store")},
		{Label: "tls 1.3 offered", Done: tls13 != nil && *tls13, Note: "the current version"},
		{Label: "no deprecated versions", Done: len(weak) == 0, Note: weakNote(weak)},
	}
	if leaf != nil {
		hygiene = append(hygiene,
			screen.CheckItem{Label: "name matches", Done: certs.Covers(*leaf, name), Note: fmt.Sprintf("%d subject alternative names", len(leaf.SANs))},
			screen.CheckItem{Label: "over 30 days left", Done: leaf.DaysLeft(now) > 30, Note: fmt.Sprintf("%d days remaining", leaf.DaysLeft(now))},
		)
	}
	result.Add("GraphCheck", screen.CheckProps{Title: "hygiene", Items: hygiene}, 1)

	// Sheet, not Table: five columns of long subjects force a horizontal
	// scroll at any width. One section per certificate turns that axis
	// vertical, so every name renders whole.
	sections := make([]screen.SheetSection, 0, len(handshake.Chain))
	for _, cert := range handshake.Chain {
		sections = append(sections, screen.SheetSection{Title: cert.Label, Rows: [][]string{
			{"subject", cert.Subject},
			{"issuer", cert.Issuer},
			{"key", cert.Key},
			{"signature", cert.Signature},
			{"valid", cert.NotBefore.Format("2006-01-02") + " to " + cert.NotAfter.Format("2006-01-02")},
		}})
	}
	if len(sections) == 0 {
		sections = []screen.SheetSection{{Title: "chain", Rows: [][]string{{"result", "no chain returned"}}}}
	}
	result.Add("GraphSheet", screen.SheetProps{Title: "chain", Headers: []string{"field", "value"}, Sections: sections}, 3)

	now2 := time.Now().UnixMilli()
	result.Add("GraphTimer", screen.TimerProps{
		Title: "measured", Kind: "ago", At: &now2,
		Caption: strconv.Itoa(handshake.MS) + "ms to " + handshake.IP,
	}, 1)

	if leaf != nil {
		rows := make([][]string, 0, len(leaf.SANs))
		for _, name := range certs.SortedSANs(*leaf) {
			rows = append(rows, []string{name})
		}
		if len(rows) == 0 {
			rows = [][]string{{"none published"}}
		}
		result.Add("GraphTable", screen.TableProps{Title: "names covered", Headers: []string{"name"}, Rows: rows}, 1)
	}

	if leaf != nil && len(handshake.Chain) > 1 {
		earliest := handshake.Chain[0]
		for _, cert := range handshake.Chain {
			if cert.NotAfter.Before(earliest.NotAfter) {
				earliest = cert
			}
		}
		if earliest.Role != "leaf" {
			result.Note(fmt.Sprintf(
				"the %s certificate expires before the leaf, on %s. renewing the leaf alone won’t fix that.",
				earliest.Role, earliest.NotAfter.Format("2006-01-02")))
		}
	}
	if len(weak) > 0 {
		result.Note(joinAnd(weak) + " are still offered. both are deprecated and current clients won’t negotiate them, so this is a configuration leftover, not an active risk.")
	}
	result.Note("chain " + handshake.ChainSource + ". timing is one region to one endpoint and is not a global measurement.")
}

func weakNote(weak []string) string {
	if len(weak) == 0 {
		return "1.0 and 1.1 refused"
	}
	return joinAnd(weak) + " still offered"
}

func joinAnd(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	}
	out := ""
	for i, value := range values {
		switch {
		case i == 0:
			out = value
		case i == len(values)-1:
			out += " and " + value
		default:
			out += ", " + value
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
