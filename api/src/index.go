// SRC. Where the answers come from, and whether the upstreams are answering.
package handler

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/zaidmukaddam/dug/pkg/dnsx"
	"github.com/zaidmukaddam/dug/pkg/guard"
	"github.com/zaidmukaddam/dug/pkg/httpx"
	"github.com/zaidmukaddam/dug/pkg/icmpx"
	"github.com/zaidmukaddam/dug/pkg/rdap"
	"github.com/zaidmukaddam/dug/pkg/resolvers"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

const probeName = "example.com"

func Handler(w http.ResponseWriter, r *http.Request) {
	result := screen.New("SRC", "sources")
	run(r, result)
	result.Write(w, r)
}

func run(r *http.Request, result *screen.Result) {
	ctx := r.Context()

	jobs := make([]dnsx.Job, 0, len(resolvers.List))
	for _, resolver := range resolvers.List {
		jobs = append(jobs, dnsx.Job{Name: probeName, Type: "A", Resolver: resolver.IP})
	}
	answers := dnsx.QueryMany(ctx, jobs, len(jobs))
	result.Spend(len(answers))
	result.HoldTTL(60, "dns")

	statuses := make([]string, 0, len(answers))
	healthy := 0
	latency := make([]int, 0, len(answers))
	rows := make([][]string, 0, len(answers))

	for i, answer := range answers {
		state := "ok"
		switch {
		case answer.Err != "" || len(answer.Records) == 0:
			state = "down"
		case answer.MS > 400:
			state = "degraded"
		}
		if state == "ok" {
			healthy++
		}
		if state == "down" {
			result.Degrade(resolvers.List[i].Name, orText(answer.Err, "no answer"))
		}
		statuses = append(statuses, state)
		latency = append(latency, answer.MS)
		rows = append(rows, []string{
			resolvers.List[i].Name, resolvers.List[i].IP, state, strconv.Itoa(answer.MS),
		})
	}

	state := "warn"
	if healthy == len(resolvers.List) {
		state = "ok"
	}
	result.SetVerdict(state,
		fmt.Sprintf("%d of %d resolvers are answering normally", healthy, len(resolvers.List)),
		"nothing is stored between queries, so every screen is a live lookup")

	result.Add("GraphUptime", screen.UptimeProps{
		Title: "resolver health", Days: statuses,
		From: resolvers.List[0].Name, To: resolvers.List[len(resolvers.List)-1].Name,
		Columns: len(resolvers.List),
	}, 2)

	result.Add("GraphKpi", screen.KpiProps{
		Title: "upstreams", Value: fmt.Sprintf("%d/%d", healthy, len(resolvers.List)),
		Label: "resolvers answering", Hint: "degraded means slow, down means no answer", Data: latency,
	}, 1)

	result.Add("GraphTable", screen.TableProps{
		Title: "resolver list", Headers: []string{"name", "address", "status", "ms"},
		Align: []string{"left", "left", "left", "right"}, Rows: rows,
	}, 2)

	rankItems := make([]screen.RankItem, 0, len(answers))
	for i, answer := range answers {
		rankItems = append(rankItems, screen.RankItem{
			Label: resolvers.List[i].Name, Value: answer.MS, Display: strconv.Itoa(answer.MS) + "ms",
		})
	}
	sort.SliceStable(rankItems, func(i, j int) bool { return rankItems[i].Value > rankItems[j].Value })
	result.Add("GraphRank", screen.RankProps{Title: "slowest upstreams", Items: rankItems}, 2)

	lowest, highest := latency[0], latency[0]
	for _, value := range latency {
		if value < lowest {
			lowest = value
		}
		if value > highest {
			highest = value
		}
	}
	result.Add("GraphSpark", screen.SparkProps{
		Title: "latency", Data: latency,
		Caption: fmt.Sprintf("%dms to %dms, in list order", lowest, highest),
	}, 1)

	result.Add("GraphCheck", screen.CheckProps{Title: "upstream services", Items: probeUpstreams(ctx)}, 1)

	icmpOK, icmpWhy := icmpx.Available(false)
	region := os.Getenv("VERCEL_REGION")
	if region == "" {
		region = "local"
	}
	result.Add("GraphSpec", screen.SpecProps{Title: "runtime", Rows: []screen.SpecRow{
		{Label: "region", Value: region, Accent: true},
		{Label: "runtime", Value: "go"},
		{Label: "icmp sockets", Value: availability(icmpOK) + ", " + icmpWhy},
		{Label: "lookup timeout", Value: dnsx.Timeout.String()},
		{Label: "query cap per request", Value: strconv.Itoa(screen.MaxUpstream)},
		{Label: "resolvers", Value: strconv.Itoa(len(resolvers.List)) + " fixed"},
		{Label: "root servers", Value: strconv.Itoa(len(resolvers.Roots)) + " fixed"},
	}}, 1)

	ports := guard.AllowedPorts()
	sort.Ints(ports)
	portText := ""
	for i, port := range ports {
		if i > 0 {
			portText += " "
		}
		portText += strconv.Itoa(port)
	}

	result.Add("GraphSheet", screen.SheetProps{
		Title:   "policy",
		Headers: []string{"setting", "value"},
		Sections: []screen.SheetSection{
			{Title: "cache ceilings", Rows: [][]string{
				{"dns", "1h"}, {"rdap", "6h"}, {"tls", "1h"},
				{"asn", "24h"}, {"http", "5m"}, {"floor", strconv.Itoa(screen.TTLFloor) + "s"},
			}},
			{Title: "guard", Rows: [][]string{
				{"allowed ports", portText},
				{"allowed schemes", "http https"},
				{"explicit denied ranges", strconv.Itoa(len(guard.Denylist()))},
				{"validation", "in net.Dialer.Control, immediately before connect"},
			}},
			{Title: "not done", Rows: [][]string{
				{"monitoring", "non-goal, nothing is stored between queries"},
				{"registrant lookup", "redacted at source, and there is an official channel"},
				{"private address space", "refused on every command"},
			}},
		},
	}, 2)

	denyRows := make([][]string, 0, len(guard.Denylist()))
	for _, entry := range guard.Denylist() {
		denyRows = append(denyRows, []string{entry[0], entry[1]})
	}
	result.Add("GraphTable", screen.TableProps{
		Title: "explicit denylist", Headers: []string{"range", "reason"}, Rows: denyRows,
	}, 3)

	now := time.Now().UnixMilli()
	result.Add("GraphTimer", screen.TimerProps{
		Title: "checked", Kind: "ago", At: &now,
		Caption: fmt.Sprintf("%d resolvers probed", len(answers)),
	}, 1)

	result.Note("egress addresses on vercel are shared and rotating, so an upstream may rate limit this deployment for reasons that have nothing to do with it. that’s why every screen carries a degraded state, not an error.")
	result.Note("the guard runs in net.Dialer.Control, which fires after resolution and before connect, so there’s no window between the check and the connection for a second dns answer.")
}

func probeUpstreams(ctx context.Context) []screen.CheckItem {
	items := make([]screen.CheckItem, 0, 2)

	started := time.Now()
	_, err := rdap.Bootstrap(ctx)
	ms := int(time.Since(started).Milliseconds())
	if err != nil {
		items = append(items, screen.CheckItem{Label: "iana rdap bootstrap", Done: false, Note: err.Error()})
	} else {
		items = append(items, screen.CheckItem{
			Label: "iana rdap bootstrap", Done: true, Note: strconv.Itoa(ms) + "ms",
		})
	}

	answer := dnsx.Query(ctx, "AS13335.asn.cymru.com", "TXT", resolvers.Default.IP)
	if len(answer.Records) > 0 {
		items = append(items, screen.CheckItem{
			Label: "team cymru dns", Done: true, Note: strconv.Itoa(answer.MS) + "ms",
		})
	} else {
		items = append(items, screen.CheckItem{
			Label: "team cymru dns", Done: false, Note: orText(answer.Err, "no answer"),
		})
	}

	return items
}

func availability(ok bool) string {
	if ok {
		return "available"
	}
	return "unavailable"
}

func orText(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

var _ = httpx.UserAgent
