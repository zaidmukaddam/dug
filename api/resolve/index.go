// DIG and TTL. Record lookup against one resolver.
package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/zaidmukaddam/dug/pkg/dnsx"
	"github.com/zaidmukaddam/dug/pkg/resolvers"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	target := query.Get("target")
	command := query.Get("command")
	if command == "" {
		command = "DIG"
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
	resolver := resolvers.ByID(query.Get("resolver"))

	if command == "TTL" {
		runTTL(r, result, name, resolver)
	} else {
		runDig(r, result, name, resolver, query.Get("type"))
	}
	result.Write(w, r)
}

func runDig(r *http.Request, result *screen.Result, name string, resolver resolvers.Resolver, only string) {
	types := dnsx.RecordTypes
	if only != "" {
		for _, candidate := range dnsx.RecordTypes {
			if candidate == only {
				types = []string{only}
				break
			}
		}
	}

	jobs := make([]dnsx.Job, 0, len(types))
	for _, rtype := range types {
		jobs = append(jobs, dnsx.Job{Name: name, Type: rtype, Resolver: resolver.IP})
	}
	answers := dnsx.QueryMany(r.Context(), jobs, 12)
	result.Spend(len(answers))
	result.HoldTTL(dnsx.MinTTL(answers), "dns")

	total, present, slowest := 0, 0, 0
	for _, answer := range answers {
		total += len(answer.Records)
		if len(answer.Records) > 0 {
			present++
		}
		if answer.MS > slowest {
			slowest = answer.MS
		}
		if answer.Err != "" {
			result.Degrade(resolver.ID+" "+answer.Type, answer.Err)
		}
	}

	unicodeForm, asciiForm := dnsx.DisplayForms(name)
	ttl := result.Payload().TTL

	if present > 0 {
		result.SetVerdict("ok",
			fmt.Sprintf("%s has %d record%s across %d of %d types", name, total, plural(total), present, len(answers)),
			fmt.Sprintf("answered by %s, good for %ds", resolver.Name, ttl))
	} else {
		result.SetVerdict("none",
			name+" returned no records at all",
			"either the name does not exist or the resolver has nothing for it")
	}

	result.Add("GraphStat", screen.StatProps{Title: "answer", Items: []screen.StatItem{
		{Value: strconv.Itoa(total), Label: "records", Accent: true},
		{Value: fmt.Sprintf("%d/%d", present, len(answers)), Label: "types present"},
		{Value: strconv.Itoa(ttl) + "s", Label: "cache lifetime", Hint: "shortest ttl, floored at 30s"},
		{Value: strconv.Itoa(slowest) + "ms", Label: "slowest lookup"},
	}}, 3)

	result.Add("GraphTable", screen.TableProps{
		Title:   "records",
		Headers: []string{"type", "ttl", "value"},
		Align:   []string{"left", "right", "left"},
		Rows:    recordRows(answers),
	}, 2)

	items := make([]screen.CheckItem, 0, len(answers))
	for _, answer := range answers {
		note := "none"
		switch {
		case len(answer.Records) > 0:
			note = fmt.Sprintf("%d record%s", len(answer.Records), plural(len(answer.Records)))
		case answer.Err != "":
			note = answer.Err
		case answer.Rcode != "NOERROR":
			note = answer.Rcode
		}
		items = append(items, screen.CheckItem{Label: answer.Type, Done: len(answer.Records) > 0, Note: note})
	}
	result.Add("GraphCheck", screen.CheckProps{Title: "types present", Items: items}, 1)

	rows := []screen.SpecRow{{Label: "query", Value: asciiForm, Accent: true}}
	if unicodeForm != asciiForm {
		rows = append(rows, screen.SpecRow{Label: "as typed", Value: unicodeForm + " (idn)"})
	}
	rows = append(rows,
		screen.SpecRow{Label: "resolver", Value: resolver.Name + " " + resolver.IP},
		screen.SpecRow{Label: "types asked", Value: join(types)},
	)
	result.Add("GraphSpec", screen.SpecProps{Title: "query", Rows: rows}, 1)

	now := time.Now().UnixMilli()
	result.Add("GraphTimer", screen.TimerProps{
		Title:   "answered",
		Kind:    "ago",
		At:      &now,
		Caption: fmt.Sprintf("%dms slowest of %d lookups", slowest, len(answers)),
	}, 1)

	if unicodeForm != asciiForm {
		result.Note("the name was typed as " + unicodeForm + " and sent as " + asciiForm +
			". both forms are shown because a name that renders as one thing and resolves as another is worth seeing.")
	}
	result.Note("one lookup from one region to " + resolver.Name + ". timings are not a global latency measurement.")
}

// recordRows renders one row per record. Types with nothing get a row reading
// none: absence is rendered, not omitted, because a missing row looks like the
// tool did not check.
func recordRows(answers []dnsx.Answer) [][]string {
	rows := make([][]string, 0, len(answers))
	for _, answer := range answers {
		switch {
		case answer.Err != "":
			rows = append(rows, []string{answer.Type, "-", "error: " + answer.Err})
		case answer.Rcode != "NOERROR":
			rows = append(rows, []string{answer.Type, "-", answer.Rcode})
		case len(answer.Records) == 0:
			rows = append(rows, []string{answer.Type, "-", "none"})
		default:
			for i, record := range answer.Records {
				rtype, ttl := "", ""
				if i == 0 {
					rtype = answer.Type
					if answer.TTL > 0 {
						ttl = strconv.Itoa(answer.TTL)
					}
				}
				rows = append(rows, []string{rtype, ttl, record})
			}
		}
	}
	return rows
}

func runTTL(r *http.Request, result *screen.Result, name string, resolver resolvers.Resolver) {
	jobs := make([]dnsx.Job, 0, len(dnsx.RecordTypes))
	for _, rtype := range dnsx.RecordTypes {
		jobs = append(jobs, dnsx.Job{Name: name, Type: rtype, Resolver: resolver.IP})
	}
	answers := dnsx.QueryMany(r.Context(), jobs, 12)
	result.Spend(len(answers))
	result.HoldTTL(dnsx.MinTTL(answers), "dns")

	present := make([]dnsx.Answer, 0, len(answers))
	for _, answer := range answers {
		if len(answer.Records) > 0 && answer.TTL > 0 {
			present = append(present, answer)
		}
	}

	if len(present) == 0 {
		result.SetVerdict("none", name+" has no records to report a ttl for", "no type returned an answer")
		result.Degrade("dns", "no records to report a ttl for")
		result.Add("GraphSpec", screen.SpecProps{Title: "ttl", Rows: []screen.SpecRow{
			{Label: "records", Value: "none"},
			{Label: "reason", Value: "no type returned an answer"},
		}}, 3)
		return
	}

	shortest, longest, ceiling := present[0], present[0], 0
	for _, answer := range present {
		if answer.TTL < shortest.TTL {
			shortest = answer
		}
		if answer.TTL > longest.TTL {
			longest = answer
		}
		if answer.TTL > ceiling {
			ceiling = answer.TTL
		}
	}

	state := "ok"
	if shortest.TTL < 300 {
		state = "warn"
	}
	result.SetVerdict(state,
		fmt.Sprintf("a change to %s takes up to %s to be picked up", name, duration(shortest.TTL)),
		fmt.Sprintf("set by the %s record, the shortest of %d types", shortest.Type, len(present)))

	data := make([]int, 0, len(present))
	for _, answer := range present {
		data = append(data, answer.TTL)
	}
	result.Add("GraphKpi", screen.KpiProps{
		Title: "shortest ttl",
		Value: strconv.Itoa(shortest.TTL) + "s",
		Label: shortest.Type + " record",
		Hint:  "how long a change takes to be picked up everywhere",
		Data:  data,
	}, 1)

	sorted := append([]dnsx.Answer(nil), present...)
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].TTL > sorted[i].TTL {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	items := make([]screen.RankItem, 0, len(sorted))
	rows := make([][]string, 0, len(sorted))
	for _, answer := range sorted {
		items = append(items, screen.RankItem{Label: answer.Type, Value: answer.TTL, Display: duration(answer.TTL)})
		rows = append(rows, []string{answer.Type, strconv.Itoa(answer.TTL), duration(answer.TTL), strconv.Itoa(len(answer.Records))})
	}

	result.Add("GraphRank", screen.RankProps{Title: "ttl per record", Items: items, Max: ceiling}, 2)

	ttl := result.Payload().TTL
	result.Add("GraphMeter", screen.MeterProps{
		Title:   "cache lifetime",
		Value:   float64(ttl) / 3600.0,
		Caption: fmt.Sprintf("%ds of the one hour cap", ttl),
	}, 1)

	result.Add("GraphTable", screen.TableProps{
		Title:   "records",
		Headers: []string{"type", "ttl", "expires in", "records"},
		Align:   []string{"left", "right", "right", "right"},
		Rows:    rows,
	}, 2)

	result.Note(fmt.Sprintf(
		"ttls are as served by %s, which counts down its own cached copy. the authoritative value can be higher. longest here is %s at %s.",
		resolver.Name, longest.Type, duration(longest.TTL)))
}

func duration(seconds int) string {
	switch {
	case seconds < 60:
		return strconv.Itoa(seconds) + "s"
	case seconds < 3600:
		return strconv.Itoa(seconds/60) + "m"
	case seconds < 86400:
		return strconv.Itoa(seconds/3600) + "h"
	default:
		return strconv.Itoa(seconds/86400) + "d"
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func join(values []string) string {
	out := ""
	for i, value := range values {
		if i > 0 {
			out += " "
		}
		out += value
	}
	return out
}
