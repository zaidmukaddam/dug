// OG. The card a scraper builds from the page, and whether it actually works.
//
// SEO already reports that og:image is declared. This fetches it. That is the
// whole reason this is its own command: a declared image tells you nothing
// about whether the card renders, and the ways cards break are all invisible
// from the tag — the url is relative, the file 404s, it sits behind auth, it is
// the wrong shape, or it is too large for the scraper to accept.
//
// The image is fetched through the same guard as everything else, so an
// og:image pointing at private space is refused rather than followed.
package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/zaidmukaddam/dug/pkg/dnsx"
	"github.com/zaidmukaddam/dug/pkg/pagex"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

// The card conventions worth measuring against. These are what the large
// scrapers document, not universal law, and the screen says which is which.
const (
	cardWidth  = 1200
	cardHeight = 630
	// Facebook documents 8MB; X is stricter at 5MB. The lower one is the
	// useful ceiling because a card that fails on one platform is broken.
	maxCardBytes = 5 << 20
)

func Handler(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(r.URL.Query().Get("target"))
	if target == "" {
		screen.Fail(w, r, "OG", "", "no domain given", "this command needs a domain name")
		return
	}

	name, err := dnsx.ToName(target)
	if err != nil {
		screen.Fail(w, r, "OG", target, target+" is not a domain name", err.Error())
		return
	}

	result := screen.New("OG", name)
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

	og := page.OpenGraph
	rawImage := og["image"]

	// The image is only fetched when one is declared. Nothing is invented for
	// a page that has none: absent is the answer.
	var img pagex.Image
	if rawImage != "" {
		img = pagex.FetchImage(ctx, origin+"/", rawImage)
		result.Spend(2)
	}

	// og:title, og:type, og:image and og:url are the four the Open Graph
	// protocol lists as required. The rest below are card quality rather than
	// protocol conformance, and are labelled that way.
	loads := rawImage != "" && img.Err == "" && img.Status == 200 && img.Bytes > 0
	sized := img.Width > 0 && img.Height > 0

	signals := map[string]bool{
		"og:title":       og["title"] != "",
		"og:type":        og["type"] != "",
		"og:image":       rawImage != "",
		"og:url":         og["url"] != "",
		"image loads":    loads,
		"image absolute": rawImage != "" && img.Absolute,
		"image size ok":  loads && img.Bytes <= maxCardBytes,
		"image shape ok": sized && withinCardRatio(img.Width, img.Height),
		"twitter card":   page.Twitter["card"] != "",
	}
	present, total := pagex.Count(signals)

	state := "ok"
	if present < total {
		state = "warn"
	}
	headline := name + " builds " + itoa(present) + " of " + itoa(total) + " card signals"
	detail := "the image was fetched, not just read off the tag."
	switch {
	case rawImage == "":
		headline = name + " declares no og:image"
		detail = "most scrapers will show a card with no picture, or no card at all."
	case !loads:
		headline = name + " declares an og:image that did not load"
		detail = "the tag is there and the file is not: " + imageFailure(img)
	}
	result.SetVerdict(state, headline, detail)

	result.Add("GraphCheck", screen.CheckProps{
		Title: "card signals",
		Items: checkItems(signals, map[string]string{
			"og:title":       trim(og["title"], 60),
			"og:type":        og["type"],
			"og:image":       trim(rawImage, 60),
			"og:url":         trim(og["url"], 60),
			"image loads":    loadNote(rawImage, img, loads),
			"image absolute": "resolved to " + trim(img.URL, 50),
			"image size ok":  kb(img.Bytes) + " of " + kb(maxCardBytes) + " allowed",
			"image shape ok": dimensions(img) + ", " + ratio(img),
		}),
	}, 3)

	if rawImage != "" {
		rows := []screen.SpecRow{
			{Label: "declared", Value: trim(rawImage, 60), Accent: true},
			{Label: "resolved", Value: trim(img.URL, 60)},
			{Label: "status", Value: statusLine(img)},
			{Label: "type", Value: orNone(img.Type)},
			{Label: "size", Value: kb(img.Bytes)},
			{Label: "dimensions", Value: dimensions(img)},
			{Label: "aspect", Value: ratio(img)},
		}
		// Declared dimensions are a hint scrapers use before they download the
		// file, so a declaration that disagrees with the file is worth naming.
		if declared := og["image:width"] + "x" + og["image:height"]; declared != "x" {
			rows = append(rows, screen.SpecRow{
				Label: "declared size",
				Value: declared + agreement(og, img),
			})
		}
		result.Add("GraphSpec", screen.SpecProps{Title: "image", Rows: rows}, 2)
	}

	tags := []screen.SpecRow{}
	for _, key := range []string{"title", "type", "url", "image", "description", "site_name", "locale"} {
		tags = append(tags, screen.SpecRow{
			Label:  "og:" + key,
			Value:  orNone(trim(og[key], 60)),
			Accent: key == "title",
		})
	}
	result.Add("GraphSpec", screen.SpecProps{Title: "open graph", Rows: tags}, 2)

	twitter := []screen.SpecRow{}
	for _, key := range []string{"card", "title", "description", "image", "site"} {
		value := page.Twitter[key]
		note := ""
		// X falls back to the og tag when the twitter one is absent, so an
		// empty row here is usually fine rather than missing.
		if value == "" && og[key] != "" {
			note = "  (falls back to og:" + key + ")"
		}
		twitter = append(twitter, screen.SpecRow{
			Label:  "twitter:" + key,
			Value:  orNone(trim(value, 50)) + note,
			Accent: key == "card",
		})
	}
	result.Add("GraphSpec", screen.SpecProps{Title: "twitter", Rows: twitter}, 2)

	// og:url disagreeing with canonical is how a share ends up attributed to
	// the wrong page, and neither tag looks wrong on its own.
	if og["url"] != "" && page.Canonical != "" {
		agree := "yes"
		if !sameURL(og["url"], page.Canonical) {
			agree = "no, a share may be attributed to a different page"
		}
		result.Add("GraphSpec", screen.SpecProps{Title: "identity", Rows: []screen.SpecRow{
			{Label: "og:url", Value: trim(og["url"], 60), Accent: true},
			{Label: "canonical", Value: trim(page.Canonical, 60)},
			{Label: "agree", Value: agree},
		}}, 1)
	}

	result.Note("the image is fetched from here, not from a scraper's network. an image " +
		"behind a firewall or a geo rule may load for this tool and not for the platform, " +
		"or the reverse.")
	result.Note("1200x630 and the 5mb ceiling are what the large scrapers document rather " +
		"than rules. a card outside them still renders in some places and not others, which " +
		"is why they are reported as measurements and not as failures.")
}

func checkItems(signals map[string]bool, notes map[string]string) []screen.CheckItem {
	// Fixed order: a map walks differently every request, and a screen that
	// reorders itself between two identical queries reads as unstable.
	order := []string{
		"og:title", "og:type", "og:image", "og:url",
		"image loads", "image absolute", "image size ok", "image shape ok", "twitter card",
	}
	items := make([]screen.CheckItem, 0, len(order))
	for _, label := range order {
		note := notes[label]
		if !signals[label] {
			note = "absent"
			// A failure with a reason is worth more than the word absent.
			if label == "image loads" && notes[label] != "" {
				note = notes[label]
			}
		}
		items = append(items, screen.CheckItem{Label: label, Done: signals[label], Note: note})
	}
	return items
}

// withinCardRatio accepts the band the platforms actually render without
// letterboxing, rather than demanding exactly 1.91:1.
func withinCardRatio(width, height int) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	aspect := float64(width) / float64(height)
	return aspect >= 1.6 && aspect <= 2.1
}

func ratio(img pagex.Image) string {
	if img.Width <= 0 || img.Height <= 0 {
		return "unknown"
	}
	aspect := float64(img.Width) / float64(img.Height)
	text := strconv.FormatFloat(aspect, 'f', 2, 64) + ":1"
	if withinCardRatio(img.Width, img.Height) {
		return text
	}
	return text + ", outside the 1.6 to 2.1 band"
}

func dimensions(img pagex.Image) string {
	if img.Width <= 0 || img.Height <= 0 {
		if img.Type != "" && !strings.HasPrefix(img.Type, "image/") {
			return "not an image, served " + img.Type
		}
		return "unknown, not a png, jpeg or gif"
	}
	text := itoa(img.Width) + "x" + itoa(img.Height)
	if img.Width == cardWidth && img.Height == cardHeight {
		return text + ", the recommended size"
	}
	return text
}

func agreement(og map[string]string, img pagex.Image) string {
	if img.Width <= 0 {
		return ""
	}
	if og["image:width"] == itoa(img.Width) && og["image:height"] == itoa(img.Height) {
		return ", matches the file"
	}
	return ", does not match the file"
}

// loadNote distinguishes the three outcomes that the word "absent" flattens:
// there was no image to fetch, the fetch failed, or it worked. "http 0" on a
// page that declares no og:image reads as a failed request that never happened.
func loadNote(declared string, img pagex.Image, ok bool) string {
	switch {
	case declared == "":
		return "no og:image to load"
	case ok:
		return "http " + itoa(img.Status) + ", " + kb(img.Bytes)
	default:
		return imageFailure(img)
	}
}

func imageFailure(img pagex.Image) string {
	if img.Err != "" {
		return img.Err
	}
	if img.Status != 200 {
		return "http " + itoa(img.Status)
	}
	return "empty response"
}

func statusLine(img pagex.Image) string {
	if img.Err != "" {
		return img.Err
	}
	return "http " + itoa(img.Status)
}

// sameURL compares ignoring a trailing slash, which is the difference most
// sites have between their og:url and their canonical and which no scraper
// minds.
func sameURL(a, b string) bool {
	return strings.TrimSuffix(a, "/") == strings.TrimSuffix(b, "/")
}

func kb(n int) string {
	if n <= 0 {
		return "0 kb"
	}
	return itoa((n+1023)/1024) + " kb"
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
