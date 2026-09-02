// Package pagex reads the facts a crawler or an agent would read.
//
// Everything here is lifted from the document as served: the title that is in
// the html, the canonical that is declared, the structured data that parses.
// Nothing is scored and nothing is advised. "Your title is too long" is an
// opinion that changes with the search engine and the year; "your title is 71
// characters" is true, and is what the screens report.
//
// The one judgement made is counting: how many of a named set of signals are
// present. That is arithmetic over facts, not a model's opinion of a page.
package pagex

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"net/url"
	"strings"

	// Registered for their DecodeConfig side effect only. Without these the
	// decoder knows no formats and every image reports unknown dimensions.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"golang.org/x/net/html"

	"github.com/zaidmukaddam/dug/pkg/httpx"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

// Page is a single fetched document, reduced to what is worth reporting.
type Page struct {
	URL       string
	Status    int
	Err       string
	Redirects int

	Title       string
	Description string
	Canonical   string
	MetaRobots  string
	Lang        string
	Viewport    string

	Headings   map[string]int
	FirstH1    string
	OpenGraph  map[string]string
	Twitter    map[string]string
	JSONLDType []string
	Hreflang   []string

	// TextChars counts the text a client that runs no javascript would see,
	// with script, style, template and noscript removed. It is the measure
	// behind "the page is empty without javascript".
	TextChars int
	Images    int
	ImagesAlt int
	Links     int
}

// NoPage records a page that could not be read, as SEO, AEO and OG all do
// before their own analysis. It reports whether the caller should stop.
func NoPage(result *screen.Result, name, origin string, page *Page) bool {
	if page.Err == "" {
		return false
	}
	result.Degrade("http", page.Err)
	result.SetVerdict("warn", name+" did not serve a page to read", page.Err)
	result.Add("GraphSpec", screen.SpecProps{Title: "request", Rows: []screen.SpecRow{
		{Label: "url", Value: origin + "/", Accent: true},
		{Label: "result", Value: "no page"},
		{Label: "reason", Value: page.Err},
	}}, 3)
	return true
}

// Fetch reads one url and reduces it. A non-html answer still returns a Page
// with its status, because "it redirected to a login" is an answer too.
func Fetch(ctx context.Context, rawURL string) *Page {
	page := &Page{
		URL:       rawURL,
		Headings:  map[string]int{},
		OpenGraph: map[string]string{},
		Twitter:   map[string]string{},
	}

	response := httpx.Get(ctx, rawURL)
	page.Status = response.Status
	page.Err = response.Err
	if len(response.Hops) > 0 {
		page.Redirects = len(response.Hops) - 1
	}
	if response.Err != "" || len(response.Body) == 0 {
		return page
	}

	root, err := html.Parse(bytes.NewReader(response.Body))
	if err != nil {
		page.Err = "unparseable html"
		return page
	}

	var text strings.Builder
	walk(root, page, &text, rawURL)
	page.TextChars = len(strings.Join(strings.Fields(text.String()), " "))
	return page
}

// skipText are the elements whose contents are not page text. A crawler that
// counted script bodies as content would find every page enormous.
var skipText = map[string]bool{
	"script": true, "style": true, "template": true, "noscript": true, "svg": true,
}

func walk(node *html.Node, page *Page, text *strings.Builder, base string) {
	if node.Type == html.TextNode {
		text.WriteString(node.Data)
		text.WriteString(" ")
	}

	if node.Type == html.ElementNode {
		switch node.Data {
		case "html":
			page.Lang = attr(node, "lang")
		case "title":
			if page.Title == "" {
				page.Title = strings.TrimSpace(textOf(node))
			}
		case "meta":
			readMeta(node, page)
		case "link":
			readLink(node, page, base)
		case "h1", "h2", "h3", "h4", "h5", "h6":
			page.Headings[node.Data]++
			if node.Data == "h1" && page.FirstH1 == "" {
				page.FirstH1 = strings.Join(strings.Fields(textOf(node)), " ")
			}
		case "img":
			page.Images++
			if strings.TrimSpace(attr(node, "alt")) != "" {
				page.ImagesAlt++
			}
		case "a":
			if attr(node, "href") != "" {
				page.Links++
			}
		case "script":
			if strings.Contains(strings.ToLower(attr(node, "type")), "ld+json") {
				page.JSONLDType = append(page.JSONLDType, ldTypes(textOf(node))...)
			}
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && skipText[child.Data] {
			// Still visited, because a ld+json script is read for its types;
			// its text just does not count as page text.
			var ignored strings.Builder
			walk(child, page, &ignored, base)
			continue
		}
		walk(child, page, text, base)
	}
}

func readMeta(node *html.Node, page *Page) {
	name := strings.ToLower(attr(node, "name"))
	property := strings.ToLower(attr(node, "property"))
	content := strings.TrimSpace(attr(node, "content"))
	if content == "" {
		return
	}

	switch {
	case name == "description":
		page.Description = content
	case name == "robots":
		page.MetaRobots = content
	case name == "viewport":
		page.Viewport = content
	case strings.HasPrefix(property, "og:"):
		page.OpenGraph[strings.TrimPrefix(property, "og:")] = content
	// Twitter's tags are written as name= by the spec and as property= by half
	// the internet, so both are read.
	case strings.HasPrefix(name, "twitter:"):
		page.Twitter[strings.TrimPrefix(name, "twitter:")] = content
	case strings.HasPrefix(property, "twitter:"):
		page.Twitter[strings.TrimPrefix(property, "twitter:")] = content
	}
}

func readLink(node *html.Node, page *Page, base string) {
	rel := strings.ToLower(strings.TrimSpace(attr(node, "rel")))
	href := strings.TrimSpace(attr(node, "href"))
	if href == "" {
		return
	}
	switch rel {
	case "canonical":
		page.Canonical = absolute(base, href)
	case "alternate":
		if lang := attr(node, "hreflang"); lang != "" {
			page.Hreflang = append(page.Hreflang, lang)
		}
	}
}

// ldTypes pulls every @type out of one ld+json block, whether it is a single
// node, an array, or a @graph of them.
func ldTypes(body string) []string {
	var decoded any
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &decoded); err != nil {
		return nil
	}
	out := []string{}
	var visit func(any)
	visit = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			switch name := typed["@type"].(type) {
			case string:
				out = append(out, name)
			case []any:
				for _, entry := range name {
					if text, ok := entry.(string); ok {
						out = append(out, text)
					}
				}
			}
			if graph, ok := typed["@graph"]; ok {
				visit(graph)
			}
		case []any:
			for _, entry := range typed {
				visit(entry)
			}
		}
	}
	visit(decoded)
	return out
}

func absolute(base, href string) string {
	parsed, err := url.Parse(base)
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return parsed.ResolveReference(ref).String()
}

func attr(node *html.Node, name string) string {
	for _, a := range node.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}
	return ""
}

func textOf(node *html.Node) string {
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(node)
	return b.String()
}

// Probe is one url checked for existence and shape, which is most of what both
// the crawler surface and the agent surface come down to.
type Probe struct {
	Path   string
	Status int
	Type   string
	Bytes  int
	Found  bool
}

func Check(ctx context.Context, origin, path string) Probe {
	response := httpx.Get(ctx, origin+path)
	probe := Probe{
		Path:   path,
		Status: response.Status,
		Type:   strings.TrimSpace(strings.Split(response.Headers.Get("Content-Type"), ";")[0]),
		Bytes:  len(response.Body),
	}
	// A soft 404 that answers 200 with an html error page is not a found file,
	// so an empty body or an html answer to a non-html path does not count.
	probe.Found = response.Status == 200 && probe.Bytes > 0
	return probe
}

// ScriptSources is every script the document loads, resolved against base.
//
// Parsed from the html rather than taken off a Page, because the caller that
// wants this wants the bodies too: what a page does at runtime is written in
// its bundles, not in the markup, and the markup is only the index to them.
// Inline scripts have no src and are already in the body the caller holds.
func ScriptSources(body, base string) []string {
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil
	}

	found := []string{}
	var walkScripts func(node *html.Node)
	walkScripts = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "script" {
			if src := attr(node, "src"); src != "" {
				if absolute := absolute(base, src); absolute != "" {
					found = append(found, absolute)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walkScripts(child)
		}
	}
	walkScripts(root)
	return found
}

// Image is a card image actually fetched, which is the whole difference
// between "og:image is declared" and "the card works".
type Image struct {
	URL      string
	Status   int
	Type     string
	Bytes    int
	Width    int
	Height   int
	Err      string
	Absolute bool
}

// FetchImage retrieves the image a scraper would retrieve and reads its real
// dimensions from the header bytes.
//
// DecodeConfig rather than Decode: it reads the size out of the first few
// hundred bytes and never allocates the pixels, so a 4000x4000 png costs the
// same here as a thumbnail. png, jpeg and gif are registered below; anything
// else still reports status, type and size, with the dimensions left at zero
// rather than guessed.
func FetchImage(ctx context.Context, base, raw string) Image {
	resolved := absolute(base, raw)
	img := Image{URL: resolved}

	// A relative og:image is the single most common broken card: the page
	// renders fine and every scraper that does not resolve it gets nothing.
	if parsed, err := url.Parse(raw); err == nil {
		img.Absolute = parsed.IsAbs()
	}

	response := httpx.Get(ctx, resolved)
	img.Status = response.Status
	img.Err = response.Err
	img.Type = strings.TrimSpace(strings.Split(response.Headers.Get("Content-Type"), ";")[0])
	img.Bytes = len(response.Body)
	if response.Err != "" || len(response.Body) == 0 {
		return img
	}

	// A truncated body is normal for a large image and is not a failure: the
	// header carrying the dimensions is at the front.
	if config, _, err := image.DecodeConfig(bytes.NewReader(response.Body)); err == nil {
		img.Width = config.Width
		img.Height = config.Height
	}
	return img
}

// Markdown asks a url for markdown and reports what came back, which is the
// acceptmarkdown.com negotiation seen from the outside.
func Markdown(ctx context.Context, origin string) (contentType string, vary string, ok bool) {
	response := httpx.GetWithHeaders(ctx, origin, map[string]string{"Accept": "text/markdown"})
	contentType = strings.TrimSpace(strings.Split(response.Headers.Get("Content-Type"), ";")[0])
	vary = response.Headers.Get("Vary")
	ok = contentType == "text/markdown"
	return contentType, vary, ok
}

// Count is the arithmetic the verdicts are built from: how many of a named set
// of signals were present.
func Count(found map[string]bool) (present int, total int) {
	for _, ok := range found {
		if ok {
			present++
		}
	}
	return present, len(found)
}
