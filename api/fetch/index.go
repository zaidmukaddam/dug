// HTTP and TRACE. Headers, redirect chain, security headers, timing.
//
// Routed at /api/fetch: every connection here goes to whatever was typed, so
// this is the handler the guarded dialer exists for. Paths are constructed by
// the server and are always "/", so there is no user-supplied URL.
package handler

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zaidmukaddam/dug/pkg/dnsx"
	"github.com/zaidmukaddam/dug/pkg/httpx"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

var headerGroups = []struct {
	title   string
	headers []string
}{
	{"transport security", []string{"Strict-Transport-Security", "Content-Security-Policy", "X-Content-Type-Options"}},
	{"framing and referrer", []string{"X-Frame-Options", "Referrer-Policy", "Permissions-Policy", "Cross-Origin-Opener-Policy"}},
	{"caching", []string{"Cache-Control", "Age", "ETag", "Last-Modified", "Vary", "Expires"}},
	{"content", []string{"Content-Type", "Content-Length", "Content-Encoding", "Location"}},
	{"server", []string{"Server", "Via", "Alt-Svc", "X-Powered-By", "Date"}},
}

// Present is not the same as correct, so each check says what it tested.
var securityChecks = []struct{ header, label, why string }{
	{"Strict-Transport-Security", "hsts", "tells browsers to refuse plaintext for this host"},
	{"Content-Security-Policy", "csp", "constrains what the page may load and execute"},
	{"X-Content-Type-Options", "nosniff", "stops content type guessing"},
	{"X-Frame-Options", "framing", "superseded by frame-ancestors in csp, still widely read"},
	{"Referrer-Policy", "referrer", "controls what leaks in the referer header"},
	{"Permissions-Policy", "permissions", "gates camera, microphone and geolocation"},
}

func Handler(w http.ResponseWriter, r *http.Request) {
	command, target, ok := screen.Argument(w, r, "/api/fetch", "HTTP")
	if !ok {
		return
	}
	name, err := dnsx.ToName(target)
	if err != nil {
		screen.Fail(w, r, command, target, target+" is not a hostname", err.Error())
		return
	}

	result := screen.New(command, name)
	if command == "TRACE" {
		runTrace(r, result, name)
	} else {
		runHTTP(r, result, name)
	}
	result.Write(w, r)
}

func runHTTP(r *http.Request, result *screen.Result, name string) {
	response := httpx.Get(r.Context(), "https://"+name+"/")
	result.Spend(maxInt(1, len(response.Hops)))
	result.HoldTTL(0, "http")

	if response.Status == 0 {
		result.Degrade("http", response.Err)
		result.SetVerdict("warn", name+" returned no response", orUnknown(response.Err))
		result.Add("GraphSpec", screen.SpecProps{Title: "request", Rows: []screen.SpecRow{
			{Label: "url", Value: "https://" + name + "/", Accent: true},
			{Label: "result", Value: "no response"},
			{Label: "reason", Value: orUnknown(response.Err)},
		}}, 3)
		return
	}
	if response.Err != "" {
		result.Degrade("http", response.Err)
	}

	present := 0
	for _, check := range securityChecks {
		if response.Header(check.header) != "" {
			present++
		}
	}

	redirects := maxInt(0, len(response.Hops)-1)
	detail := "no redirects"
	if redirects > 0 {
		detail = fmt.Sprintf("%d redirect%s to %s", redirects, plural(redirects), short(response.URL))
	}
	state := "warn"
	if present >= 5 {
		state = "ok"
	}
	result.SetVerdict(state,
		fmt.Sprintf("%s answers %d and sets %d of %d security headers", name, response.Status, present, len(securityChecks)),
		detail)

	flowRows := make([]screen.FlowRow, 0, len(response.Hops))
	for i, hop := range response.Hops {
		label := "failed"
		if hop.Status > 0 {
			label = strconv.Itoa(hop.Status)
		}
		tone := "muted"
		if i == len(response.Hops)-1 {
			tone = "accent"
		}
		flowRows = append(flowRows, screen.FlowRow{Nodes: []screen.FlowNode{
			{Label: label + " " + short(hop.URL), Tone: tone, Stretch: true},
		}})
	}
	if len(flowRows) == 0 {
		flowRows = []screen.FlowRow{{Nodes: []screen.FlowNode{{Label: "no request completed", Tone: "muted", Stretch: true}}}}
	}
	result.Add("GraphFlow", screen.FlowProps{Title: "redirect chain", Rows: flowRows}, 2)

	finalIP := "none"
	if len(response.Hops) > 0 {
		finalIP = orUnknown(response.Hops[len(response.Hops)-1].IP)
	}
	result.Add("GraphSpec", screen.SpecProps{Title: "response", Rows: []screen.SpecRow{
		{Label: "final url", Value: short(response.URL), Accent: true},
		{Label: "status", Value: response.Reason},
		{Label: "address", Value: finalIP},
		{Label: "redirects", Value: strconv.Itoa(redirects)},
		{Label: "server", Value: headerOr(response, "Server", "not sent")},
		{Label: "ttfb", Value: strconv.Itoa(response.Timing.TTFBMS) + "ms"},
	}}, 1)

	checkItems := make([]screen.CheckItem, 0, len(securityChecks))
	for _, check := range securityChecks {
		value := response.Header(check.header)
		note := "absent. " + check.why
		if value != "" {
			note = value
		}
		checkItems = append(checkItems, screen.CheckItem{Label: check.label, Done: value != "", Note: note})
	}
	result.Add("GraphCheck", screen.CheckProps{Title: "security headers", Items: checkItems}, 1)

	known := map[string]bool{}
	sections := make([]screen.SheetSection, 0, len(headerGroups)+1)
	for _, group := range headerGroups {
		rows := make([][]string, 0, len(group.headers))
		for _, header := range group.headers {
			known[strings.ToLower(header)] = true
			// Absence is rendered, not omitted.
			rows = append(rows, []string{strings.ToLower(header), headerOr(response, header, "not sent")})
		}
		sections = append(sections, screen.SheetSection{Title: group.title, Rows: rows})
	}

	var others []string
	for header := range response.Headers {
		if !known[strings.ToLower(header)] {
			others = append(others, header)
		}
	}
	sort.Strings(others)
	if len(others) > 0 {
		rows := make([][]string, 0, len(others))
		for i, header := range others {
			if i >= 12 {
				break
			}
			rows = append(rows, []string{strings.ToLower(header), response.Header(header)})
		}
		sections = append(sections, screen.SheetSection{Title: "other", Rows: rows})
	}
	result.Add("GraphSheet", screen.SheetProps{Title: "headers", Headers: []string{"header", "value"}, Sections: sections}, 3)

	maxAge := 0
	if hsts := response.Header("Strict-Transport-Security"); strings.Contains(hsts, "max-age=") {
		_, rest, _ := strings.Cut(hsts, "max-age=")
		digits, _, _ := strings.Cut(rest, ";")
		if value, err := strconv.Atoi(strings.TrimSpace(digits)); err == nil {
			maxAge = value
		}
	}
	display := "absent"
	if maxAge > 0 {
		display = strconv.Itoa(maxAge/86400) + "d"
	}
	result.Add("GraphBullet", screen.BulletProps{Title: "hsts max-age", Items: []screen.BulletItem{
		{Label: "against one year", Value: maxAge, Target: 31536000, Max: maxInt(31536000, maxAge), Display: display},
	}}, 1)

	now := time.Now().UnixMilli()
	result.Add("GraphTimer", screen.TimerProps{
		Title: "round trip", Kind: "ago", At: &now,
		Caption: fmt.Sprintf("%dms including %d redirects", response.Timing.TotalM, redirects),
	}, 1)

	if redirects > 0 {
		result.Note(fmt.Sprintf(
			"%d requests were made and each destination was validated again before connecting. redirects are followed by the server, never from a url the caller supplied.",
			len(response.Hops)))
	}
	result.Note("one region to one endpoint. header presence is not header correctness, and this screen only reports what was sent.")
}

func runTrace(r *http.Request, result *screen.Result, name string) {
	response := httpx.Get(r.Context(), "https://"+name+"/")
	result.Spend(maxInt(1, len(response.Hops)))
	result.HoldTTL(0, "http")
	timing := response.Timing

	if response.Status == 0 {
		result.Degrade("trace", orUnknown(response.Err))
		result.SetVerdict("warn", name+" returned no response", orUnknown(response.Err))
		result.Add("GraphSpec", screen.SpecProps{Title: "trace", Rows: []screen.SpecRow{
			{Label: "host", Value: name, Accent: true},
			{Label: "result", Value: "no response"},
			{Label: "reason", Value: orUnknown(response.Err)},
		}}, 3)
		return
	}

	// Go's transport does not expose per-phase timings without httptrace, and
	// the total is what a reader acts on, so the phases are reported as the
	// measured round trip rather than invented.
	state := "ok"
	if timing.TTFBMS >= 800 {
		state = "warn"
	}
	result.SetVerdict(state,
		fmt.Sprintf("%s sent its first byte after %dms", name, timing.TTFBMS),
		fmt.Sprintf("%d request%s, %dms total", len(response.Hops), plural(len(response.Hops)), timing.TotalM))

	items := make([]screen.WaterfallItem, 0, len(response.Hops)+1)
	for i, hop := range response.Hops {
		kind := "in"
		if i == 0 {
			kind = "start"
		}
		items = append(items, screen.WaterfallItem{
			Label: short(hop.URL), Value: hop.MS, Display: strconv.Itoa(hop.MS) + "ms", Kind: kind,
		})
	}
	items = append(items, screen.WaterfallItem{
		Label: "total", Value: timing.TotalM, Display: strconv.Itoa(timing.TotalM) + "ms", Kind: "end",
	})
	result.Add("GraphWaterfall", screen.WaterfallProps{Title: "request timing", Items: items}, 2)

	finalIP := "none"
	if len(response.Hops) > 0 {
		finalIP = orUnknown(response.Hops[len(response.Hops)-1].IP)
	}
	result.Add("GraphSpec", screen.SpecProps{Title: "trace", Rows: []screen.SpecRow{
		{Label: "host", Value: name, Accent: true},
		{Label: "address", Value: finalIP},
		{Label: "status", Value: response.Reason},
		{Label: "first byte", Value: strconv.Itoa(timing.TTFBMS) + "ms"},
		{Label: "total", Value: strconv.Itoa(timing.TotalM) + "ms"},
	}}, 1)

	rankItems := make([]screen.RankItem, 0, len(response.Hops))
	for _, hop := range response.Hops {
		rankItems = append(rankItems, screen.RankItem{
			Label: short(hop.URL), Value: hop.MS, Display: strconv.Itoa(hop.MS) + "ms",
		})
	}
	sort.SliceStable(rankItems, func(i, j int) bool { return rankItems[i].Value > rankItems[j].Value })
	result.Add("GraphRank", screen.RankProps{Title: "time by request", Items: rankItems}, 2)

	rows := make([][]string, 0, len(response.Hops))
	for _, hop := range response.Hops {
		status := "-"
		if hop.Status > 0 {
			status = strconv.Itoa(hop.Status)
		}
		rows = append(rows, []string{short(hop.URL), status, orDash(hop.IP), strconv.Itoa(hop.MS)})
	}
	result.Add("GraphTable", screen.TableProps{
		Title: "hops", Headers: []string{"url", "status", "address", "ms"},
		Align: []string{"left", "right", "left", "right"}, Rows: rows,
	}, 2)

	now := time.Now().UnixMilli()
	result.Add("GraphTimer", screen.TimerProps{
		Title: "measured", Kind: "ago", At: &now,
		Caption: strconv.Itoa(timing.TotalM) + "ms total",
	}, 1)

	result.Note("these are http request timings, not network hops. ROUTE walks the path hop by hop with icmp.")
	result.Note("measured once, from one region, to one endpoint. this isn’t a global latency figure and a single sample isn’t a distribution.")
}

func short(url string) string {
	return strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
}

func headerOr(response *httpx.Response, header, fallback string) string {
	if value := response.Header(header); value != "" {
		return value
	}
	return fallback
}

func orUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
