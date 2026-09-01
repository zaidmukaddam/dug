// SEO. What a crawler reads on the home page.
//
// Facts only. Every row is something the document says about itself, measured
// rather than graded: the title and its length, not whether the length is good.
// Where a judgement is unavoidable it is a count of named signals present, and
// the signals are listed so the count can be checked.
//
// There is no ranking estimate here and there will not be one. Rank is a
// function of a search engine's private model of the whole web, and anything
// this could say about it would be a guess wearing a number.
package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/zaidmukaddam/dug/pkg/dnsx"
	"github.com/zaidmukaddam/dug/pkg/pagex"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(r.URL.Query().Get("target"))
	if target == "" {
		screen.Fail(w, r, "SEO", "", "no domain given", "this command needs a domain name")
		return
	}

	name, err := dnsx.ToName(target)
	if err != nil {
		screen.Fail(w, r, "SEO", target, target+" is not a domain name", err.Error())
		return
	}

	result := screen.New("SEO", name)
	run(r, result, name)
	result.Write(w, r)
}

func run(r *http.Request, result *screen.Result, name string) {
	ctx := r.Context()
	origin := "https://" + name

	page := pagex.Fetch(ctx, origin+"/")
	result.Spend(4)
	result.HoldTTL(0, "http")

	if page.Err != "" {
		result.Degrade("http", page.Err)
		result.SetVerdict("warn", name+" did not serve a page to read", page.Err)
		result.Add("GraphSpec", screen.SpecProps{Title: "request", Rows: []screen.SpecRow{
			{Label: "url", Value: origin + "/", Accent: true},
			{Label: "result", Value: "no page"},
			{Label: "reason", Value: page.Err},
		}}, 3)
		return
	}

	robots := pagex.Check(ctx, origin, "/robots.txt")
	sitemap := pagex.Check(ctx, origin, "/sitemap.xml")
	result.Spend(2)

	// The named set. Listed here so the count in the verdict can be checked
	// against the rows underneath rather than taken on trust.
	signals := map[string]bool{
		"title":           page.Title != "",
		"description":     page.Description != "",
		"canonical":       page.Canonical != "",
		"one h1":          page.Headings["h1"] == 1,
		"lang":            page.Lang != "",
		"robots.txt":      robots.Found,
		"sitemap.xml":     sitemap.Found,
		"open graph":      page.OpenGraph["title"] != "" && page.OpenGraph["image"] != "",
		"twitter card":    page.Twitter["card"] != "",
		"structured data": len(page.JSONLDType) > 0,
	}
	present, total := pagex.Count(signals)

	state := "ok"
	if present < total {
		state = "warn"
	}
	result.SetVerdict(state,
		name+" serves "+itoa(present)+" of "+itoa(total)+" crawler signals",
		"each one is listed below with what the page actually said. nothing here is a ranking estimate.")

	result.Add("GraphCheck", screen.CheckProps{
		Title: "crawler signals",
		Items: screen.Checks(crawlerSignals, signals, map[string]string{
			"title":           trim(page.Title, 60),
			"description":     trim(page.Description, 60),
			"canonical":       page.Canonical,
			"one h1":          itoa(page.Headings["h1"]) + " found",
			"lang":            page.Lang,
			"robots.txt":      status(robots),
			"sitemap.xml":     status(sitemap),
			"open graph":      itoa(len(page.OpenGraph)) + " og tags",
			"twitter card":    page.Twitter["card"],
			"structured data": strings.Join(page.JSONLDType, ", "),
		}),
	}, 3)

	result.Add("GraphSpec", screen.SpecProps{Title: "document", Rows: []screen.SpecRow{
		{Label: "status", Value: itoa(page.Status), Accent: true},
		{Label: "redirects", Value: itoa(page.Redirects)},
		{Label: "title", Value: measured(page.Title)},
		{Label: "description", Value: measured(page.Description)},
		{Label: "canonical", Value: orNone(page.Canonical)},
		{Label: "meta robots", Value: orNone(page.MetaRobots)},
		{Label: "lang", Value: orNone(page.Lang)},
		{Label: "viewport", Value: orNone(page.Viewport)},
	}}, 2)

	result.Add("GraphSpec", screen.SpecProps{Title: "structure", Rows: []screen.SpecRow{
		{Label: "h1", Value: itoa(page.Headings["h1"]), Accent: true},
		{Label: "h2", Value: itoa(page.Headings["h2"])},
		{Label: "h3", Value: itoa(page.Headings["h3"])},
		{Label: "links", Value: itoa(page.Links)},
		{Label: "images", Value: itoa(page.Images)},
		{Label: "images with alt", Value: itoa(page.ImagesAlt) + " of " + itoa(page.Images)},
		{Label: "text without js", Value: itoa(page.TextChars) + " characters"},
	}}, 1)

	social := []screen.SpecRow{}
	for _, key := range []string{"title", "description", "image", "type", "url", "site_name"} {
		if value := page.OpenGraph[key]; value != "" {
			social = append(social, screen.SpecRow{Label: "og:" + key, Value: trim(value, 70)})
		}
	}
	for _, key := range []string{"card", "title", "image"} {
		if value := page.Twitter[key]; value != "" {
			social = append(social, screen.SpecRow{Label: "twitter:" + key, Value: trim(value, 70)})
		}
	}
	if len(social) == 0 {
		social = append(social, screen.SpecRow{Label: "og and twitter", Value: "none declared"})
	}
	result.Add("GraphSpec", screen.SpecProps{Title: "social", Rows: social}, 2)

	if len(page.JSONLDType) > 0 {
		rows := make([][]string, 0, len(page.JSONLDType))
		for _, kind := range page.JSONLDType {
			rows = append(rows, []string{kind})
		}
		result.Add("GraphTable", screen.TableProps{
			Title: "structured data", Headers: []string{"@type"}, Rows: rows,
		}, 1)
	}

	if len(page.Hreflang) > 0 {
		result.Add("GraphSpec", screen.SpecProps{Title: "hreflang", Rows: []screen.SpecRow{
			{Label: "declared", Value: strings.Join(page.Hreflang, ", "), Accent: true},
		}}, 1)
	}

	result.Note("read from the home page as served, with no javascript run. a page that " +
		"builds its head on the client will look emptier here than it does in a browser, " +
		"which is also how a crawler that does not execute javascript sees it.")
	result.Note("no ranking estimate is offered. rank depends on a search engine's private " +
		"model of the whole web, and any number this printed for it would be invented.")
}

var crawlerSignals = []string{
	"title", "description", "canonical", "one h1", "lang",
	"robots.txt", "sitemap.xml", "open graph", "twitter card", "structured data",
}

func status(probe pagex.Probe) string {
	if !probe.Found {
		return "http " + itoa(probe.Status)
	}
	return itoa(probe.Bytes) + " bytes"
}

// measured reports the value and its length, because length is the thing every
// guideline argues about and the only part of it that is a fact.
func measured(value string) string {
	if value == "" {
		return "none"
	}
	return trim(value, 60) + "  (" + itoa(len([]rune(value))) + " chars)"
}

func trim(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func orNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func itoa(n int) string { return strconv.Itoa(n) }
