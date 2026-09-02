// WEBMCP. Whether a site is wired for agents that arrive inside the browser,
// and whether it also answers ones that do not.
//
// What this cannot do, since the screen is built around it: tools register when
// the page runs, so nothing fetched from outside a browser can see them. A tool
// list is not a file on a server. What is left is observable evidence — the
// header native WebMCP requires, the api named in the scripts the page loads,
// and the remote MCP surface — and the screen says which of that is proof and
// which is a hint. A checker printing "12 tools found" from a curl invented it.
package handler

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/zaidmukaddam/dug/pkg/dnsx"
	"github.com/zaidmukaddam/dug/pkg/httpx"
	"github.com/zaidmukaddam/dug/pkg/pagex"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

const (
	// Nothing about bundle order tracks what a chunk contains, so a low ceiling
	// misses the one that matters. Whatever this does not reach is named on the
	// screen: an unread script is not an absent marker.
	maxScripts = 24

	// Enough that two dozen chunks are one round trip, few enough not to open a
	// connection per chunk against someone else's origin.
	scriptWorkers = 8

	// Bundles are routinely megabytes and httpx caps bodies at 256KB, which
	// would cut the file off before the marker.
	scriptBudget = 4 << 20
)

var webmcpSignals = []string{
	"origin-agent-cluster", "api in the page",
	"remote mcp endpoint", "server card", "server manifest",
}

// The strings that survive minification, because they are property names on an
// object the minifier cannot rename: the page has to spell document.modelContext
// exactly for the browser to know what it means.
//
// There is deliberately no check for a polyfill. Whether the api is native or
// installed by @mcp-b/global is a real difference, and it is not one a fetch can
// see: a bundler inlines the package's code and drops its name, so searching for
// "mcp-b" reports "no polyfill" for every site that has one. A signal that is
// false whenever it matters is worse than an absent one.
var apiMarkers = []string{"modelContext", "registerTool"}

func Handler(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(r.URL.Query().Get("target"))
	if target == "" {
		screen.Fail(w, r, "WEBMCP", "", "no domain given", "this command needs a domain name")
		return
	}

	name, err := dnsx.ToName(target)
	if err != nil {
		screen.Fail(w, r, "WEBMCP", target, target+" is not a domain name", err.Error())
		return
	}

	result := screen.New("WEBMCP", name)
	run(r, result, name)
	result.Write(w, r)
}

func run(r *http.Request, result *screen.Result, name string) {
	ctx := r.Context()
	origin := "https://" + name

	response := httpx.Get(ctx, origin+"/")
	result.Spend(4)
	result.HoldTTL(0, "http")

	if response.Err != "" || len(response.Body) == 0 {
		reason := response.Err
		if reason == "" {
			reason = "the page was empty"
		}
		result.Degrade("http", reason)
		result.SetVerdict("warn", name+" did not serve a page to read", reason)
		result.Add("GraphSpec", screen.SpecProps{Title: "request", Rows: []screen.SpecRow{
			{Label: "url", Value: origin + "/", Accent: true},
			{Label: "result", Value: "no page"},
			{Label: "reason", Value: reason},
		}}, 3)
		return
	}

	body := string(response.Body)

	// Native WebMCP is refused outright in a document that is not
	// origin-isolated, so this header is not a nicety: without it the browser
	// disables the api no matter what the page registers.
	isolation := response.Headers.Get("Origin-Agent-Cluster")

	// The runtime marker, if the site renders it server side. dug does not —
	// it is set on mount — so finding it is meaningful and missing it proves
	// nothing at all. Said on the screen, not just here.
	marker := markerValue(body)

	inHTML := mentions(body, apiMarkers)

	scripts := scriptURLs(origin, body)
	read, inScripts := scanScripts(ctx, result, scripts)

	referenced := inHTML || inScripts

	// The other half: an agent that is not in the page. These are files, so
	// unlike everything above they are simply true or false.
	var (
		card, manifest, llms pagex.Probe
		remote               mcpProbe
	)
	// Four independent reads of one origin, waited on once rather than in
	// turn: a firewalled origin costs one timeout instead of four.
	probes, probeCtx := errgroup.WithContext(ctx)
	probes.Go(func() error { card = pagex.Check(probeCtx, origin, "/.well-known/mcp/server-card.json"); return nil })
	probes.Go(func() error { manifest = pagex.Check(probeCtx, origin, "/server.json"); return nil })
	probes.Go(func() error { llms = pagex.Check(probeCtx, origin, "/llms.txt"); return nil })
	probes.Go(func() error { remote = probeMCP(probeCtx, origin); return nil })
	_ = probes.Wait()
	result.Spend(4)

	signals := map[string]bool{
		"origin-agent-cluster": isolation != "",
		"api in the page":      referenced,
		"remote mcp endpoint":  remote.found,
		"server card":          card.Found,
		"server manifest":      manifest.Found,
	}
	present, total := pagex.Count(signals)

	// Deliberately not "n of m tools". The verdict names what was actually
	// established and refuses to imply the rest.
	switch {
	case referenced && isolation != "":
		result.SetVerdict("ok",
			name+" is wired for agents inside the page",
			"the api is referenced by the page and the document is origin isolated, which is "+
				"what native webmcp requires. the tools themselves register when the page runs, "+
				"so only a browser can list them.")
	case referenced:
		result.SetVerdict("warn",
			name+" references webmcp but isn’t origin isolated",
			"chromium disables the api in a document that isn’t origin isolated, so native "+
				"webmcp will refuse whatever this page registers. sending "+
				"Origin-Agent-Cluster: ?1 is what turns it on.")
	case remote.found || card.Found || manifest.Found:
		result.SetVerdict("warn",
			name+" serves mcp, but nothing in the page registers tools",
			"an agent that is somewhere else is served. one already in the browser finds "+
				"nothing to call.")
	default:
		result.SetVerdict("none",
			"no webmcp signal found on "+name,
			"nothing in the page references the api and no mcp surface answers. this is what "+
				"most sites look like today.")
	}

	result.Add("GraphCheck", screen.CheckProps{
		Title: "webmcp signals",
		Items: screen.Checks(webmcpSignals, signals, map[string]string{
			"origin-agent-cluster": orNone(isolation) + ", required by native webmcp",
			"api in the page":      whereFound(inHTML, inScripts),
			"remote mcp endpoint":  remote.note,
			"server card":          probeRow(card),
			"server manifest":      probeRow(manifest),
		}),
	}, 3)

	// The caveat, given a frame of its own rather than a footnote, because it
	// is the difference between what this screen says and what a reader will
	// assume it says.
	result.Add("GraphSpec", screen.SpecProps{Title: "what this cannot see", Rows: []screen.SpecRow{
		{Label: "tool list", Value: "registered at runtime, not served", Accent: true},
		{Label: "confirmed by", Value: "opening the page in a webmcp browser"},
		{Label: "data-webmcp", Value: markerRow(marker)},
		{Label: "scripts read", Value: itoa(read) + " of " + itoa(len(scripts)) + ", ceiling " + itoa(maxScripts)},
	}}, 1)

	result.Add("GraphSpec", screen.SpecProps{Title: "in the page", Rows: []screen.SpecRow{
		{Label: "document.modelContext", Value: yesNo(referenced), Accent: true},
		{Label: "origin isolated", Value: yesNo(isolation != "")},
		{Label: "found in", Value: whereFound(inHTML, inScripts)},
		{Label: "scripts on the page", Value: itoa(len(scripts))},
	}}, 1)

	result.Add("GraphSpec", screen.SpecProps{Title: "for an agent elsewhere", Rows: []screen.SpecRow{
		{Label: "/mcp", Value: remote.note, Accent: true},
		{Label: "/.well-known/mcp/server-card.json", Value: probeRow(card)},
		{Label: "/server.json", Value: probeRow(manifest)},
		{Label: "/llms.txt", Value: probeRow(llms)},
	}}, 2)

	result.Note("signals present: " + itoa(present) + " of " + itoa(total))
	result.Note("tool registration happens in the browser and can’t be read from here")
}

// scriptURLs is the same origin scripts the page loads, deduplicated. A third
// party script mentioning the api is not this site wiring itself up.
func scriptURLs(origin, body string) []string {
	found := []string{}
	seen := map[string]bool{}
	site, err := url.Parse(origin)
	if err != nil {
		return found
	}

	for _, src := range pagex.ScriptSources(body, origin) {
		parsed, err := url.Parse(src)
		// Same origin means the same scheme and host, compared as parsed
		// parts. A string prefix let example.com.evil.com count as
		// example.com's own script, which is the case this filter exists for.
		if err != nil || parsed.Scheme != site.Scheme || parsed.Host != site.Host || seen[src] {
			continue
		}
		seen[src] = true
		found = append(found, src)
	}
	return found
}

// scanScripts reads up to maxScripts of them, several at a time, and reports
// how many it got through so the screen can say what it did not look at.
func scanScripts(ctx context.Context, result *screen.Result, scripts []string) (int, bool) {
	read := scripts
	if len(read) > maxScripts {
		read = read[:maxScripts]
	}
	result.Spend(len(read))

	var (
		mu   sync.Mutex
		api  bool
		wait sync.WaitGroup
	)

	queue := make(chan string)
	for worker := 0; worker < scriptWorkers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for src := range queue {
				response := httpx.GetLimited(ctx, src, scriptBudget)
				if response.Err != "" || len(response.Body) == 0 {
					continue
				}
				if !mentions(string(response.Body), apiMarkers) {
					continue
				}
				mu.Lock()
				api = true
				mu.Unlock()
			}
		}()
	}

	for _, src := range read {
		queue <- src
	}
	close(queue)
	wait.Wait()

	return len(read), api
}

type mcpProbe struct {
	found bool
	note  string
}

// /mcp answering a GET with 405 is the correct behaviour for Streamable HTTP,
// so a 405 is a stronger signal than a 200: it means something there knows what
// it is refusing. A 404 means nothing is there.
func probeMCP(ctx context.Context, origin string) mcpProbe {
	response := httpx.Get(ctx, origin+"/mcp")
	switch {
	case response.Err != "":
		return mcpProbe{false, response.Err}
	case response.Status == 405:
		return mcpProbe{true, "405, post only, which is the transport behaving correctly"}
	case response.Status == 200:
		return mcpProbe{true, "200"}
	case response.Status == 404:
		return mcpProbe{false, "404, nothing served"}
	default:
		return mcpProbe{false, itoa(response.Status)}
	}
}

func mentions(body string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(body, marker) {
			return true
		}
	}
	return false
}

// The value of data-webmcp if the html carries one, which it only does when a
// site renders the marker server side.
func markerValue(body string) string {
	index := strings.Index(body, "data-webmcp=")
	if index == -1 {
		return ""
	}
	rest := body[index+len("data-webmcp="):]
	if len(rest) == 0 || (rest[0] != '"' && rest[0] != '\'') {
		return ""
	}
	quote := rest[0]
	end := strings.IndexByte(rest[1:], quote)
	if end == -1 {
		return ""
	}
	return rest[1 : 1+end]
}

func markerRow(marker string) string {
	if marker == "" {
		return "not in the served html, which is normal and proves nothing"
	}
	return marker + ", rendered server side"
}

func whereFound(inHTML, inScripts bool) string {
	switch {
	case inHTML && inScripts:
		return "in the html and in a script"
	case inHTML:
		return "in the html"
	case inScripts:
		return "in a script the page loads"
	default:
		return "not referenced"
	}
}

func probeRow(probe pagex.Probe) string {
	if !probe.Found {
		return itoa(probe.Status) + ", not served"
	}
	return itoa(probe.Status) + ", " + itoa(probe.Bytes) + " bytes"
}

func yesNo(ok bool) string {
	if ok {
		return "yes"
	}
	return "no"
}

func orNone(value string) string {
	if value == "" {
		return "not sent"
	}
	return value
}

func itoa(n int) string { return strconv.Itoa(n) }
