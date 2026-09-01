// RDAP and WATCH. Registration data, with WHOIS as the ccTLD fallback.
package handler

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/zaidmukaddam/dug/pkg/certs"
	"github.com/zaidmukaddam/dug/pkg/dnsx"
	"github.com/zaidmukaddam/dug/pkg/epp"
	"github.com/zaidmukaddam/dug/pkg/rdap"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	target := query.Get("target")
	command := query.Get("command")
	if command == "" {
		command = "RDAP"
	}
	if target == "" {
		screen.Fail(w, r, command, "", "no domain given", "this command needs a domain name")
		return
	}
	name, err := dnsx.ToName(target)
	if err != nil {
		screen.Fail(w, r, command, target, target+" is not a domain name", err.Error())
		return
	}

	result := screen.New(command, name)
	if command == "WATCH" {
		runWatch(r, result, name)
	} else {
		runRDAP(r, result, name)
	}
	result.Write(w, r)
}

func runRDAP(r *http.Request, result *screen.Result, name string) {
	record, problems := rdap.Load(r.Context(), name)
	result.Spend(3)
	result.HoldTTL(0, "rdap")
	for _, problem := range problems {
		result.Degrade(record.Protocol, problem)
	}

	now := time.Now().UTC()
	expires, hasExpiry := record.Events["expires"]
	locked := epp.LockedCount(record.Statuses)

	switch {
	case record.Protocol == "none":
		result.SetVerdict("warn", "no registration record for "+name+" could be read",
			"neither rdap nor whois returned anything usable")
	case hasExpiry:
		days := int(expires.Sub(now).Hours() / 24)
		state := "ok"
		if days < 30 {
			state = "warn"
		}
		result.SetVerdict(state,
			fmt.Sprintf("%s is registered until %s, %d days away", name, expires.Format("2006-01-02"), days),
			fmt.Sprintf("%s, %d of %d status codes bar a change",
				orNot(record.Registrar, "registrar not published"), locked, len(record.Statuses)))
	default:
		result.SetVerdict("none", name+" is registered, with no expiry published",
			"answered over "+record.Protocol)
	}

	registryType := "whois only"
	switch {
	case record.Thin:
		registryType = "thin, referral followed"
	case record.Protocol == "rdap":
		registryType = "thick"
	}

	dateRows := make([][]string, 0, len(record.Events))
	for _, entry := range sortedEvents(record.Events) {
		dateRows = append(dateRows, []string{entry.label, entry.stamp.Format("2006-01-02 15:04 utc")})
	}
	if len(dateRows) == 0 {
		dateRows = [][]string{{"dates", "none published"}}
	}

	nsRows := make([][]string, 0, len(record.Nameservers))
	for i, ns := range record.Nameservers {
		nsRows = append(nsRows, []string{"ns " + strconv.Itoa(i+1), ns})
	}
	if len(nsRows) == 0 {
		nsRows = [][]string{{"nameservers", "none published"}}
	}

	signed := "no"
	if record.DNSSEC {
		signed = "yes"
	}

	result.Add("GraphSheet", screen.SheetProps{
		Title:   "registration",
		Headers: []string{"field", "value"},
		Sections: []screen.SheetSection{
			{Title: "registry", Rows: [][]string{
				{"name", orNot(record.Name, name)},
				{"handle", orNot(record.Handle, "not published")},
				{"registry type", registryType},
			}},
			{Title: "registrar", Rows: [][]string{
				{"registrar", orNot(record.Registrar, "not published")},
				{"abuse contact", orNot(record.Abuse, "not published")},
				{"registrant", orNot(record.Registrant, "redacted")},
			}},
			{Title: "dates", Rows: dateRows},
			{Title: "nameservers", Rows: nsRows},
			{Title: "dnssec", Rows: [][]string{{"delegation signed", signed}}},
		},
	}, 2)

	items := epp.CheckItems(record.Statuses)
	checkItems := make([]screen.CheckItem, 0, len(items))
	for _, item := range items {
		checkItems = append(checkItems, screen.CheckItem{Label: item.Label, Done: item.Done, Note: item.Note})
	}
	result.Add("GraphCheck", screen.CheckProps{Title: "status codes", Items: checkItems}, 1)

	events := make([]screen.TimelineEvent, 0, len(record.Events)+1)
	inserted := false
	for _, entry := range sortedEvents(record.Events) {
		state := "done"
		if entry.stamp.After(now) {
			state = "next"
			if !inserted {
				events = append(events, screen.TimelineEvent{Date: now.Format("2006-01-02"), Label: "today", State: "now"})
				inserted = true
			}
		}
		events = append(events, screen.TimelineEvent{
			Date: entry.stamp.Format("2006-01-02"), Label: entry.label, State: state,
		})
	}
	switch {
	case len(events) == 0:
		events = []screen.TimelineEvent{{Date: now.Format("2006-01-02"), Label: "no dates published", State: "now"}}
	case !inserted:
		events = append(events, screen.TimelineEvent{Date: now.Format("2006-01-02"), Label: "today", State: "now"})
	}
	result.Add("GraphTimeline", screen.TimelineProps{Title: "lifecycle", Events: events}, 2)

	if hasExpiry {
		result.Add("GraphCountdown", screen.CountdownProps{
			Title: "domain expiry", To: expires.Format(time.RFC3339), Done: "expired",
			Caption: "computed from the record, not monitored",
		}, 1)
	} else {
		result.Add("GraphSpec", screen.SpecProps{Title: "domain expiry", Rows: []screen.SpecRow{
			{Label: "expiry", Value: "not published"},
			{Label: "reason", Value: "this registry does not publish the field"},
		}}, 1)
	}

	result.Add("GraphSpec", screen.SpecProps{Title: "source", Rows: []screen.SpecRow{
		{Label: "protocol", Value: record.Protocol, Accent: true},
		{Label: "endpoint", Value: orNot(record.Endpoint, "none")},
		{Label: "status codes", Value: countOrNone(len(record.Statuses))},
		{Label: "nameservers", Value: countOrNone(len(record.Nameservers))},
	}}, 1)

	labels := splitTLD(name)
	switch record.Protocol {
	case "whois":
		result.Note("answered over whois on port 43 because ." + labels +
			" publishes no rdap service. icann contracts bind gtlds only, so a cctld may never publish one.")
	case "none":
		result.Note("neither rdap nor whois returned a usable record for ." + labels +
			". nothing partial is being shown as complete.")
	default:
		if record.Thin {
			result.Note("thin registry: the registry answer and the registrar's own record were merged, because the full detail only exists at the second hop.")
		}
	}
	result.Note("registrant contact fields are redacted by design. rdap returns a minimal data set and this tool does not try to work around it.")
}

// runWatch shows two countdowns to dates that are genuinely known. Nothing is
// observed between queries, and the note says so.
func runWatch(r *http.Request, result *screen.Result, name string) {
	record, problems := rdap.Load(r.Context(), name)
	result.Spend(3)
	for _, problem := range problems {
		result.Degrade(record.Protocol, problem)
	}

	handshake := certs.Connect(r.Context(), name, 443, 8*time.Second)
	result.Spend(5)
	result.HoldTTL(0, "rdap")

	now := time.Now().UTC()
	domainExpiry, hasDomain := record.Events["expires"]
	var leaf *certs.Cert
	if len(handshake.Chain) > 0 {
		leaf = &handshake.Chain[0]
	}
	if handshake.Err != "" {
		result.Degrade("tls", handshake.Err)
	}

	type horizon struct {
		label string
		when  time.Time
	}
	var known []horizon
	if hasDomain {
		known = append(known, horizon{"domain", domainExpiry})
	}
	if leaf != nil {
		known = append(known, horizon{"certificate", leaf.NotAfter})
	}
	sort.Slice(known, func(i, j int) bool { return known[i].when.Before(known[j].when) })

	if len(known) == 0 {
		result.SetVerdict("warn", "nothing datable was found for "+name,
			"neither an expiry field nor a certificate")
	} else {
		days := int(known[0].when.Sub(now).Hours() / 24)
		state := "ok"
		if days < 30 {
			state = "warn"
		}
		result.SetVerdict(state,
			fmt.Sprintf("the %s for %s lapses first, in %d days", known[0].label, name, days),
			"computed from a query made a moment ago, nothing is being monitored")
	}

	if hasDomain {
		result.Add("GraphCountdown", screen.CountdownProps{
			Title: "domain expires", To: domainExpiry.Format(time.RFC3339), Done: "expired",
			Caption: fmt.Sprintf("%d days, from the %s record", int(domainExpiry.Sub(now).Hours()/24), record.Protocol),
		}, 1)
	} else {
		result.Add("GraphSpec", screen.SpecProps{Title: "domain expires", Rows: []screen.SpecRow{
			{Label: "expiry", Value: "not published"},
			{Label: "source", Value: record.Protocol + " returned no expiry field"},
		}}, 1)
	}

	if leaf != nil {
		result.Add("GraphCountdown", screen.CountdownProps{
			Title: "certificate expires", To: leaf.NotAfter.Format(time.RFC3339), Done: "expired",
			Caption: fmt.Sprintf("%d days, %s", leaf.DaysLeft(now), leaf.Issuer),
		}, 1)
	} else {
		result.Add("GraphSpec", screen.SpecProps{Title: "certificate expires", Rows: []screen.SpecRow{
			{Label: "certificate", Value: "none retrieved"},
			{Label: "reason", Value: orNot(handshake.Err, "no tls on port 443")},
		}}, 1)
	}

	soonest, remaining := "unknown", "-"
	if len(known) > 0 {
		soonest = known[0].label
		remaining = fmt.Sprintf("%dd", int(known[0].when.Sub(now).Hours()/24))
	}
	domainDays, certDays := "-", "-"
	if hasDomain {
		domainDays = fmt.Sprintf("%dd", int(domainExpiry.Sub(now).Hours()/24))
	}
	if leaf != nil {
		certDays = fmt.Sprintf("%dd", leaf.DaysLeft(now))
	}
	result.Add("GraphStat", screen.StatProps{Title: "next to lapse", Items: []screen.StatItem{
		{Value: soonest, Label: "expires first", Accent: true},
		{Value: remaining, Label: "days remaining"},
		{Value: domainDays, Label: "domain"},
		{Value: certDays, Label: "certificate"},
	}}, 2)

	events := []screen.TimelineEvent{{Date: now.Format("2006-01-02"), Label: "queried", State: "now"}}
	for _, entry := range known {
		events = append(events, screen.TimelineEvent{
			Date: entry.when.Format("2006-01-02"), Label: entry.label + " lapses", State: "next",
		})
	}
	result.Add("GraphTimeline", screen.TimelineProps{Title: "horizon", Events: events}, 2)

	result.Note("nothing is being watched. both figures come from a query made a moment ago, and there’s no store, no schedule and no alert behind them. re-run the command to see current numbers.")
}

type eventEntry struct {
	label string
	stamp time.Time
}

func sortedEvents(events map[string]time.Time) []eventEntry {
	out := make([]eventEntry, 0, len(events))
	for label, stamp := range events {
		out = append(out, eventEntry{label, stamp})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].stamp.Before(out[j].stamp) })
	return out
}

func orNot(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func countOrNone(n int) string {
	if n == 0 {
		return "none"
	}
	return strconv.Itoa(n)
}

func splitTLD(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return name[i+1:]
		}
	}
	return name
}
