// AEO. What an answer engine can read, parse and cite.
//
// The same idea people also sell as GEO. There is no separate command for that
// because there is no separate measurement: both come down to whether a machine
// that is not a browser can get the content, understand its shape, and find a
// structured way in. Two names, one set of facts.
//
// Every row is a live fetch of a published, checkable thing: a file that is
// either served or not, a header that is either sent or not, a negotiation that
// either happens or does not. Nothing here rates writing quality or guesses at
// whether a model would cite the page, because neither is observable from
// outside and a number for either would be invented.
package handler

import (
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/zaidmukaddam/dug/pkg/dnsx"
	"github.com/zaidmukaddam/dug/pkg/pagex"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

// The minimum text a page has to carry before a client that runs no javascript
// is reading a page rather than an empty shell. Named rather than inline: it is
// a threshold this tool chose, not a law, and it should be visible as such.
const readableChars = 500

func Handler(w http.ResponseWriter, r *http.Request) {
	command, target, ok := screen.Argument(w, r, "/api/aeo", "AEO")
	if !ok {
		return
	}

	name, err := dnsx.ToName(target)
	if err != nil {
		screen.Fail(w, r, command, target, target+" is not a domain name", err.Error())
		return
	}

	result := screen.New(command, name)
	run(r, result, name)
	result.Write(w, r)
}

func run(r *http.Request, result *screen.Result, name string) {
	ctx := r.Context()
	origin := "https://" + name

	page := pagex.Fetch(ctx, origin+"/")
	result.Spend(4)
	result.HoldTTL(0, "http")

	if pagex.NoPage(result, name, origin, page) {
		return
	}

	var (
		llms, openapi, catalog, sitemap pagex.Probe
		markdownType, markdownVary      string
		markdownOK                      bool
	)
	// Five independent reads of one origin, waited on once. In turn they cost
	// the sum of five timeouts against a slow host; together they cost the
	// slowest.
	probes, probeCtx := errgroup.WithContext(ctx)
	probes.Go(func() error { llms = pagex.Check(probeCtx, origin, "/llms.txt"); return nil })
	probes.Go(func() error { openapi = pagex.Check(probeCtx, origin, "/openapi.json"); return nil })
	probes.Go(func() error { catalog = pagex.Check(probeCtx, origin, "/.well-known/api-catalog"); return nil })
	probes.Go(func() error { sitemap = pagex.Check(probeCtx, origin, "/sitemap.xml"); return nil })
	probes.Go(func() error {
		markdownType, markdownVary, markdownOK = pagex.Markdown(probeCtx, origin+"/")
		return nil
	})
	_ = probes.Wait()
	result.Spend(5)

	signals := map[string]bool{
		"content without js": page.TextChars >= readableChars,
		"structured data":    len(page.JSONLDType) > 0,
		"one h1":             page.Headings["h1"] == 1,
		"description":        page.Description != "",
		"sitemap.xml":        sitemap.Found,
		"llms.txt":           llms.Found,
		"markdown":           markdownOK,
		"openapi":            openapi.Found,
		"api catalog":        catalog.Found,
	}
	present, total := pagex.Count(signals)

	state := "ok"
	if present < total {
		state = "warn"
	}
	result.SetVerdict(state,
		name+" carries "+itoa(present)+" of "+itoa(total)+" signals an answer engine reads",
		"each one is a file, a header or a negotiation that either happened or didn’t. "+
			"none of it predicts whether a model will cite the page.")

	result.Add("GraphCheck", screen.CheckProps{
		Title: "agent signals",
		Items: screen.Checks(agentSignals, signals, map[string]string{
			"content without js": itoa(page.TextChars) + " characters, threshold " + itoa(readableChars),
			"structured data":    strings.Join(page.JSONLDType, ", "),
			"one h1":             quoted(page.FirstH1),
			"description":        itoa(len([]rune(page.Description))) + " characters",
			"sitemap.xml":        found(sitemap),
			"llms.txt":           found(llms),
			"markdown":           "served " + markdownType,
			"openapi":            found(openapi),
			"api catalog":        found(catalog),
		}),
	}, 3)

	// The measure behind the first signal, spelled out. A page that is empty
	// without javascript is the single most common reason an answer engine has
	// nothing to quote, and the number is the whole argument.
	result.Add("GraphKpi", screen.KpiProps{
		Title: "readable without javascript",
		Value: itoa(page.TextChars),
		Label: "characters of text with scripts removed",
		Hint:  "threshold " + itoa(readableChars),
		Data:  []int{1, 1, 1},
	}, 1)

	result.Add("GraphSpec", screen.SpecProps{Title: "discovery", Rows: []screen.SpecRow{
		{Label: "/llms.txt", Value: probeRow(llms), Accent: true},
		{Label: "/openapi.json", Value: probeRow(openapi)},
		{Label: "/.well-known/api-catalog", Value: probeRow(catalog)},
		{Label: "/sitemap.xml", Value: probeRow(sitemap)},
	}}, 2)

	result.Add("GraphSpec", screen.SpecProps{Title: "negotiation", Rows: []screen.SpecRow{
		{Label: "accept", Value: "text/markdown", Accent: true},
		{Label: "answered", Value: screen.OrNone(markdownType)},
		{Label: "vary", Value: screen.OrNone(markdownVary)},
		{Label: "cache safe", Value: varySafe(markdownVary, markdownOK)},
	}}, 1)

	result.Add("GraphSpec", screen.SpecProps{Title: "shape", Rows: []screen.SpecRow{
		{Label: "title", Value: screen.OrNone(trim(page.Title, 60)), Accent: true},
		{Label: "h1", Value: screen.OrNone(trim(page.FirstH1, 60))},
		{Label: "headings", Value: headingLine(page)},
		{Label: "lang", Value: screen.OrNone(page.Lang)},
		{Label: "canonical", Value: screen.OrNone(page.Canonical)},
	}}, 2)

	if len(page.JSONLDType) > 0 {
		rows := make([][]string, 0, len(page.JSONLDType))
		for _, kind := range page.JSONLDType {
			rows = append(rows, []string{kind})
		}
		result.Add("GraphTable", screen.TableProps{
			Title: "structured data", Headers: []string{"@type"}, Rows: rows,
		}, 1)
	}

	result.Note("aeo and geo are measured the same way here. both ask whether a machine " +
		"that isn’t a browser can read the content and find a structured way in, and " +
		"that’s one set of facts however it’s marketed.")
	result.Note("nothing on this screen predicts citation. whether a model quotes a page " +
		"depends on its training and its retrieval, neither of which is observable from " +
		"outside, so no number for it is offered.")
	result.Note("the page is read with no javascript run, which is what a crawler without " +
		"a renderer sees. a client-rendered page scores lower here than it looks in a browser, " +
		"and that gap is the finding rather than a fault in the measurement.")
}

var agentSignals = []string{
	"content without js", "structured data", "one h1", "description",
	"sitemap.xml", "llms.txt", "markdown", "openapi", "api catalog",
}

// varySafe is the half of markdown negotiation that is easy to miss: without
// Accept in Vary a shared cache can hand the html variant to the next client
// that asked for markdown.
func varySafe(vary string, negotiating bool) string {
	if !negotiating {
		return "not negotiating"
	}
	if strings.Contains(strings.ToLower(vary), "accept") {
		return "yes, Accept is in Vary"
	}
	return "no, Accept is missing from Vary"
}

func headingLine(page *pagex.Page) string {
	parts := []string{}
	for _, level := range []string{"h1", "h2", "h3", "h4", "h5", "h6"} {
		if count := page.Headings[level]; count > 0 {
			parts = append(parts, level+"×"+itoa(count))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "  ")
}

func probeRow(probe pagex.Probe) string {
	if !probe.Found {
		return "http " + itoa(probe.Status)
	}
	return itoa(probe.Bytes) + " bytes, " + screen.OrNone(probe.Type)
}

func found(probe pagex.Probe) string {
	if !probe.Found {
		return "http " + itoa(probe.Status)
	}
	return itoa(probe.Bytes) + " bytes"
}

func quoted(value string) string {
	if value == "" {
		return "none"
	}
	return trim(value, 60)
}

func trim(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func itoa(n int) string { return strconv.Itoa(n) }
