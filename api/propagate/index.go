// PROP. Agreement across the fixed resolver list.
//
// The resolver list and record-type list are both constants, so the query count
// is fixed regardless of input. Authoritative nameservers are not queried:
// public resolvers are what a visitor actually reaches, and NS shows the
// authoritative side.
package handler

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/zaidmukaddam/dug/pkg/dnsx"
	"github.com/zaidmukaddam/dug/pkg/resolvers"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

// Six types across six resolvers, plus a repeat pass. Inside the request cap.
var propTypes = []string{"A", "AAAA", "MX", "NS", "TXT", "SOA"}

func Handler(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	if target == "" {
		screen.Fail(w, r, "PROP", "", "no domain given", "this command needs a domain name")
		return
	}
	name, err := dnsx.ToName(target)
	if err != nil {
		screen.Fail(w, r, "PROP", target, target+" is not a domain name", err.Error())
		return
	}

	result := screen.New("PROP", name)
	run(r, result, name)
	result.Write(w, r)
}

func run(r *http.Request, result *screen.Result, name string) {
	ctx := r.Context()

	jobs := make([]dnsx.Job, 0, len(propTypes)*len(resolvers.List))
	for _, rtype := range propTypes {
		for _, resolver := range resolvers.List {
			jobs = append(jobs, dnsx.Job{Name: name, Type: rtype, Resolver: resolver.IP})
		}
	}
	answers := dnsx.QueryMany(ctx, jobs, 18)
	result.Spend(len(answers))
	result.HoldTTL(dnsx.MinTTL(answers), "dns")

	grid := map[string]map[string]dnsx.Answer{}
	for _, rtype := range propTypes {
		grid[rtype] = map[string]dnsx.Answer{}
	}
	for _, answer := range answers {
		grid[answer.Type][answer.Resolver] = answer
	}

	consensus := map[string]string{}
	agreeCount := map[string]int{}
	for _, rtype := range propTypes {
		best, count := majority(grid[rtype])
		consensus[rtype] = best
		agreeCount[rtype] = count
	}

	// One glyph per resolver on the record type people mean when they ask
	// whether a change has propagated.
	statuses := make([]string, 0, len(resolvers.List))
	agreeing := 0
	for _, resolver := range resolvers.List {
		state := status(grid["A"][resolver.IP], consensus["A"])
		statuses = append(statuses, state)
		if state == "ok" {
			agreeing++
		}
	}

	if agreeing == len(resolvers.List) {
		result.SetVerdict("ok",
			fmt.Sprintf("%d of %d resolvers return the same address for %s", agreeing, len(resolvers.List), name),
			"fully propagated")
	} else if agreeing == 0 {
		result.SetVerdict("warn", "no resolver returned an address for "+name,
			"a change may still be spreading, or the name doesn’t resolve")
	} else {
		result.SetVerdict("warn",
			fmt.Sprintf("%d of %d resolvers return the same address for %s", agreeing, len(resolvers.List), name),
			"a change may still be spreading, or the name is steered by location")
	}

	result.Add("GraphUptime", screen.UptimeProps{
		Title:   "A agreement",
		Days:    statuses,
		From:    resolvers.List[0].Name,
		To:      resolvers.List[len(resolvers.List)-1].Name,
		Columns: len(resolvers.List),
	}, 2)

	kpiData := make([]int, 0, len(propTypes))
	for _, rtype := range propTypes {
		kpiData = append(kpiData, agreeCount[rtype])
	}
	result.Add("GraphKpi", screen.KpiProps{
		Title: "consensus",
		Value: fmt.Sprintf("%d/%d", agreeing, len(resolvers.List)),
		Label: "resolvers agree on A",
		Hint:  "differ or no answer is not the same as wrong",
		Data:  kpiData,
	}, 1)

	columns := make([]string, 0, len(resolvers.List))
	for _, resolver := range resolvers.List {
		columns = append(columns, resolver.Short)
	}

	heatRows := make([]screen.HeatRow, 0, len(propTypes))
	matrixRows := make([]screen.MatrixRow, 0, len(propTypes))
	for _, rtype := range propTypes {
		values := make([]int, 0, len(resolvers.List))
		cells := make([]any, 0, len(resolvers.List))
		for _, resolver := range resolvers.List {
			answer := grid[rtype][resolver.IP]
			switch status(answer, consensus[rtype]) {
			case "ok":
				values = append(values, 4)
			case "degraded":
				values = append(values, 2)
			default:
				values = append(values, 0)
			}
			if answer.Err != "" {
				cells = append(cells, "-")
			} else {
				cells = append(cells, answer.MS)
			}
		}
		heatRows = append(heatRows, screen.HeatRow{Label: rtype, Values: values})
		matrixRows = append(matrixRows, screen.MatrixRow{Label: rtype, Values: cells})
	}

	result.Add("GraphHeatmap", screen.HeatmapProps{
		Title: "agreement", Columns: columns, Rows: heatRows, Max: 4, Legend: true,
		Caption: "filled agrees with the majority, half differs, empty had no answer",
	}, 3)

	result.Add("GraphMatrix", screen.MatrixProps{Title: "latency ms", Columns: columns, Rows: matrixRows}, 3)

	meanMS := map[string]int{}
	for _, resolver := range resolvers.List {
		total := 0
		for _, rtype := range propTypes {
			total += grid[rtype][resolver.IP].MS
		}
		meanMS[resolver.IP] = total / len(propTypes)
	}

	ranked := append([]resolvers.Resolver(nil), resolvers.List...)
	sort.SliceStable(ranked, func(i, j int) bool { return meanMS[ranked[i].IP] > meanMS[ranked[j].IP] })
	rankItems := make([]screen.RankItem, 0, len(ranked))
	for _, resolver := range ranked {
		rankItems = append(rankItems, screen.RankItem{
			Label: resolver.Name, Value: meanMS[resolver.IP], Display: strconv.Itoa(meanMS[resolver.IP]) + "ms",
		})
	}
	result.Add("GraphRank", screen.RankProps{Title: "slowest resolvers", Items: rankItems}, 2)

	spark := make([]int, 0, len(resolvers.List))
	aLatency := make([]int, 0, len(resolvers.List))
	mxLatency := make([]int, 0, len(resolvers.List))
	shortLabels := make([]string, 0, len(resolvers.List))
	lowest, highest := 1<<30, 0
	for _, resolver := range resolvers.List {
		spark = append(spark, meanMS[resolver.IP])
		aLatency = append(aLatency, grid["A"][resolver.IP].MS)
		mxLatency = append(mxLatency, grid["MX"][resolver.IP].MS)
		shortLabels = append(shortLabels, resolver.Short)
		if meanMS[resolver.IP] < lowest {
			lowest = meanMS[resolver.IP]
		}
		if meanMS[resolver.IP] > highest {
			highest = meanMS[resolver.IP]
		}
	}

	result.Add("GraphSpark", screen.SparkProps{
		Title: "mean latency", Data: spark,
		Caption: fmt.Sprintf("%dms to %dms across the list", lowest, highest),
	}, 1)

	result.Add("GraphPlot", screen.PlotProps{
		Title: "A lookup latency", Data: aLatency, Labels: shortLabels, Variant: "area", Height: 7,
	}, 2)

	result.Add("GraphBars", screen.BarsProps{
		Title:     "address vs mail",
		From:      screen.BarSeries{Label: "A", Values: aLatency},
		To:        screen.BarSeries{Label: "MX", Values: mxLatency},
		Processor: "ms per resolver, in list order",
	}, 1)

	addRepeatSlope(r, result, name)
	addDisagreement(result, grid, consensus)

	var differing []string
	for _, rtype := range propTypes {
		for _, resolver := range resolvers.List {
			answer := grid[rtype][resolver.IP]
			if answer.Err != "" {
				result.Degrade(resolver.Name+" "+rtype, answer.Err)
			}
			if status(answer, consensus[rtype]) == "degraded" {
				differing = appendUnique(differing, rtype)
			}
		}
	}

	if len(differing) > 0 {
		result.Note("resolvers disagree on " + joinComma(differing) +
			". that is normal during a change and normal permanently for geo-steered names, so disagreement is reported rather than judged.")
	}
	result.Note(fmt.Sprintf(
		"%d resolvers, %d record types, %d lookups from one region. the resolver list is a constant in the codebase, not a parameter.",
		len(resolvers.List), len(propTypes), result.Payload().Upstream))
}

// status maps an answer onto Uptime's own vocabulary: match, differ, no answer
// land on ok, degraded, down.
func status(answer dnsx.Answer, consensus string) string {
	if answer.Err != "" || answer.Rcode != "NOERROR" || len(answer.Records) == 0 {
		return "down"
	}
	if consensus == "" {
		return "degraded"
	}
	if answer.Fingerprint() == consensus {
		return "ok"
	}
	return "degraded"
}

func majority(byResolver map[string]dnsx.Answer) (string, int) {
	counts := map[string]int{}
	for _, answer := range byResolver {
		if len(answer.Records) > 0 && answer.Err == "" {
			counts[answer.Fingerprint()]++
		}
	}
	best, bestCount := "", 0
	for print, count := range counts {
		if count > bestCount {
			best, bestCount = print, count
		}
	}
	return best, bestCount
}

// addRepeatSlope measures a first query against an immediate repeat. Labelled
// repeat rather than cached, because the upstream resolver may already have
// held the record before the first query.
func addRepeatSlope(r *http.Request, result *screen.Result, name string) {
	jobs := make([]dnsx.Job, 0, len(resolvers.List))
	for _, resolver := range resolvers.List {
		jobs = append(jobs, dnsx.Job{Name: name, Type: "A", Resolver: resolver.IP})
	}
	cold := dnsx.QueryMany(r.Context(), jobs, len(jobs))
	warm := dnsx.QueryMany(r.Context(), jobs, len(jobs))
	result.Spend(len(cold) + len(warm))

	items := make([]screen.SlopeItem, 0, len(resolvers.List))
	for i, resolver := range resolvers.List {
		items = append(items, screen.SlopeItem{Label: resolver.Name, From: cold[i].MS, To: warm[i].MS})
	}
	result.Add("GraphSlope", screen.SlopeProps{
		Title: "first vs repeat", FromLabel: "first", ToLabel: "repeat", Items: items,
	}, 2)
}

func addDisagreement(result *screen.Result, grid map[string]map[string]dnsx.Answer, consensus map[string]string) {
	for _, rtype := range propTypes {
		best := consensus[rtype]
		if best == "" {
			continue
		}
		for _, resolver := range resolvers.List {
			answer := grid[rtype][resolver.IP]
			if len(answer.Records) == 0 || answer.Fingerprint() == best {
				continue
			}

			majorityRecords := map[string]bool{}
			for _, record := range splitLines(best) {
				majorityRecords[record] = true
			}
			theirs := map[string]bool{}
			for _, record := range answer.Records {
				theirs[record] = true
			}

			all := make([]string, 0, len(majorityRecords)+len(theirs))
			for record := range majorityRecords {
				all = append(all, record)
			}
			for record := range theirs {
				if !majorityRecords[record] {
					all = append(all, record)
				}
			}
			sort.Strings(all)

			rows := make([]screen.DiffRow, 0, len(all))
			for _, record := range all {
				sign := "add"
				switch {
				case majorityRecords[record] && theirs[record]:
					sign = "keep"
				case majorityRecords[record]:
					sign = "remove"
				}
				rows = append(rows, screen.DiffRow{Label: rtype, Value: record, Sign: sign})
			}

			count := 0
			for _, other := range resolvers.List {
				if grid[rtype][other.IP].Fingerprint() == best {
					count++
				}
			}
			result.Add("GraphDiff", screen.DiffProps{
				Title: resolver.Name + " differs",
				Rows:  rows,
				Footer: &screen.DiffRow{
					Label: "majority",
					Value: fmt.Sprintf("%d of %d resolvers", count, len(resolvers.List)),
				},
			}, 2)
			return
		}
	}

	result.Add("GraphDiff", screen.DiffProps{
		Title: "disagreement",
		Rows:  []screen.DiffRow{{Label: "none", Value: "every resolver returned the same answer", Sign: "keep"}},
	}, 2)
}

func splitLines(value string) []string {
	if value == "" {
		return nil
	}
	out := []string{}
	current := ""
	for _, ch := range value {
		if ch == '\n' {
			out = append(out, current)
			current = ""
			continue
		}
		current += string(ch)
	}
	return append(out, current)
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func joinComma(values []string) string {
	out := ""
	for i, value := range values {
		if i > 0 {
			out += ", "
		}
		out += value
	}
	return out
}
