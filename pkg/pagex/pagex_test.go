package pagex

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// parse runs the extraction without a network fetch, which is what makes the
// reduction testable at all.
func parse(t *testing.T, body string) *Page {
	t.Helper()
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	page := &Page{
		Headings:  map[string]int{},
		OpenGraph: map[string]string{},
		Twitter:   map[string]string{},
	}
	var text strings.Builder
	walk(root, page, &text, "https://example.com/")
	page.TextChars = len(strings.Join(strings.Fields(text.String()), " "))
	return page
}

func TestReadsTheDocumentAsServed(t *testing.T) {
	page := parse(t, `<html lang="en">
  <head>
    <title>  Example  Title  </title>
    <meta name="description" content="A description.">
    <meta name="robots" content="index, follow">
    <link rel="canonical" href="/home">
    <link rel="alternate" hreflang="fr" href="/fr">
    <meta property="og:title" content="OG Title">
    <meta property="og:image" content="https://example.com/og.png">
    <meta name="twitter:card" content="summary_large_image">
  </head>
  <body>
    <h1>The Heading</h1><h2>a</h2><h2>b</h2>
    <img src="a.png" alt="described"><img src="b.png" alt="">
    <a href="/one">one</a><a>no href</a>
    <p>Body text.</p>
  </body></html>`)

	if page.Title != "Example  Title" {
		t.Errorf("title = %q", page.Title)
	}
	if page.Description != "A description." || page.MetaRobots != "index, follow" {
		t.Errorf("meta = %q / %q", page.Description, page.MetaRobots)
	}
	// A relative canonical is resolved, because the value a crawler compares
	// against is the absolute one.
	if page.Canonical != "https://example.com/home" {
		t.Errorf("canonical = %q, want it resolved against the page url", page.Canonical)
	}
	if page.Lang != "en" {
		t.Errorf("lang = %q", page.Lang)
	}
	if page.Headings["h1"] != 1 || page.Headings["h2"] != 2 {
		t.Errorf("headings = %v", page.Headings)
	}
	if page.FirstH1 != "The Heading" {
		t.Errorf("first h1 = %q", page.FirstH1)
	}
	if page.OpenGraph["title"] != "OG Title" || page.Twitter["card"] != "summary_large_image" {
		t.Errorf("social = %v / %v", page.OpenGraph, page.Twitter)
	}
	if page.Hreflang[0] != "fr" {
		t.Errorf("hreflang = %v", page.Hreflang)
	}
	// An empty alt is a decorative image, not a described one.
	if page.Images != 2 || page.ImagesAlt != 1 {
		t.Errorf("images = %d, with alt = %d", page.Images, page.ImagesAlt)
	}
	// An anchor with no href is not a link.
	if page.Links != 1 {
		t.Errorf("links = %d, want 1", page.Links)
	}
}

// The whole point of the character count is to answer "is this page empty
// without javascript", so script and style bodies must not be counted as text.
// A bundle inlined in a script tag would otherwise make every empty shell look
// like a substantial document.
func TestTextCountIgnoresScriptsAndStyles(t *testing.T) {
	page := parse(t, `<html><head>
    <style>.a{color:red;padding:0;margin:0;border:0}</style>
    <script>const x = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";</script>
  </head><body><p>Only this counts.</p></body></html>`)

	if page.TextChars != len("Only this counts.") {
		t.Errorf("TextChars = %d, want %d — script or style text is being counted",
			page.TextChars, len("Only this counts."))
	}
}

// ld+json lives in a script tag, which is skipped for text, so it has to still
// be visited for its types. This is the case the skip logic could silently
// break.
func TestStructuredDataIsReadFromSkippedScripts(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want []string
	}{
		{
			name: "single node",
			body: `{"@context":"https://schema.org","@type":"WebSite","name":"x"}`,
			want: []string{"WebSite"},
		},
		{
			name: "graph",
			body: `{"@graph":[{"@type":"Organization"},{"@type":"Person"}]}`,
			want: []string{"Organization", "Person"},
		},
		{
			name: "array of nodes",
			body: `[{"@type":"Article"},{"@type":"BreadcrumbList"}]`,
			want: []string{"Article", "BreadcrumbList"},
		},
		{
			name: "multiple types on one node",
			body: `{"@type":["Product","Offer"]}`,
			want: []string{"Product", "Offer"},
		},
		{
			// Invalid JSON is common in the wild and must not take the screen
			// down or be reported as a type.
			name: "unparseable is ignored",
			body: `{"@type": oops}`,
			want: []string{},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			page := parse(t, `<html><head><script type="application/ld+json">`+
				test.body+`</script></head><body>x</body></html>`)

			if len(page.JSONLDType) != len(test.want) {
				t.Fatalf("types = %v, want %v", page.JSONLDType, test.want)
			}
			for i, want := range test.want {
				if page.JSONLDType[i] != want {
					t.Errorf("type %d = %q, want %q", i, page.JSONLDType[i], want)
				}
			}
		})
	}
}

// Half the web writes the twitter tags as property= rather than the name= the
// spec asks for. Reading only one spelling reports a card that is there as
// absent.
func TestTwitterTagsAreReadFromEitherSpelling(t *testing.T) {
	page := parse(t, `<html><head>
    <meta property="twitter:card" content="summary">
    <meta name="twitter:title" content="T">
  </head><body>x</body></html>`)

	if page.Twitter["card"] != "summary" {
		t.Errorf("card from property= was not read: %v", page.Twitter)
	}
	if page.Twitter["title"] != "T" {
		t.Errorf("title from name= was not read: %v", page.Twitter)
	}
}

func TestCountIsArithmeticOverTheNamedSet(t *testing.T) {
	present, total := Count(map[string]bool{"a": true, "b": false, "c": true})
	if present != 2 || total != 3 {
		t.Errorf("count = %d of %d, want 2 of 3", present, total)
	}
}

// The dimensions are read with DecodeConfig, which parses the header and never
// allocates pixels. This checks the formats actually register: without the
// blank image/png and image/jpeg imports the decoder knows no formats and every
// card silently reports unknown dimensions, which would make the shape check in
// OG always fail.
func TestImageDimensionsAreReadFromHeaderBytes(t *testing.T) {
	// Encoded rather than hand-written: png carries a crc per chunk and the
	// decoder checks it, so a fixture typed out by hand is rejected as
	// corrupt rather than testing anything.
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1200, 630))); err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("png not decoded, are the blank image format imports still there: %v", err)
	}
	if format != "png" {
		t.Errorf("format = %q, want png", format)
	}
	if config.Width != 1200 || config.Height != 630 {
		t.Errorf("size = %dx%d, want 1200x630", config.Width, config.Height)
	}

	// And the header alone is enough: truncating to the first 100 bytes still
	// yields the size, which is what keeps a large card image cheap to measure.
	short, _, err := image.DecodeConfig(bytes.NewReader(buf.Bytes()[:100]))
	if err != nil {
		t.Fatalf("dimensions need more than the header: %v", err)
	}
	if short.Width != 1200 || short.Height != 630 {
		t.Errorf("truncated size = %dx%d, want 1200x630", short.Width, short.Height)
	}
}
