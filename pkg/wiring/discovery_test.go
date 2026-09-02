// Checks on the discovery surface: the documents an agent reads before it has
// called anything.
//
// These are worth pinning because every one of them is a document whose only
// consumer is a machine. A person opening /developers notices immediately if it
// is wrong. Nobody opens /server.json, so a field that stops validating, a path
// that stops resolving, or a page list that drifts out of step with the sitemap
// is silent until something that was supposed to find this surface does not.
package wiring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"

	aicatalogapi "github.com/zaidmukaddam/dug/api/aicatalog"
	catalogapi "github.com/zaidmukaddam/dug/api/catalog"
	mcpapi "github.com/zaidmukaddam/dug/api/mcp"
	serverapi "github.com/zaidmukaddam/dug/api/server"
	servercardapi "github.com/zaidmukaddam/dug/api/servercard"
	"github.com/zaidmukaddam/dug/pkg/commands"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

// Run a handler the way the platform would and decode what it wrote.
func fetchJSON(t *testing.T, handler http.HandlerFunc, url string) (map[string]any, *http.Response) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, url, nil))

	result := recorder.Result()
	var doc map[string]any
	if err := json.NewDecoder(result.Body).Decode(&doc); err != nil {
		t.Fatalf("%s did not return decodable json: %v", url, err)
	}
	return doc, result
}

// The registry schema is not a suggestion: a document that breaks one of these
// is rejected rather than degraded, and the two that bite are easy to break by
// editing prose. `name` must be reverse-DNS with exactly one slash, and
// `description` is capped at 100 characters — a sentence one word longer than
// it looks invalidates the file.
func TestServerManifestMatchesTheRegistrySchema(t *testing.T) {
	doc, result := fetchJSON(t, serverapi.Handler, "https://dug.sh/server.json")

	if got := result.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("server.json sends Access-Control-Allow-Origin %q, so a browser agent cannot read it", got)
	}

	name, _ := doc["name"].(string)
	if !regexp.MustCompile(`^[a-zA-Z0-9.-]+/[a-zA-Z0-9._-]+$`).MatchString(name) {
		t.Errorf("server.json name %q does not match the schema's reverse-DNS pattern", name)
	}

	description, _ := doc["description"].(string)
	if length := len([]rune(description)); length > 100 {
		t.Errorf("server.json description is %d characters, the schema caps it at 100", length)
	}
	if description == "" {
		t.Error("server.json has no description, which the schema requires")
	}

	// Semantic versioning, and tied to the contract version so the manifest
	// cannot claim a generation the responses do not answer under.
	version, _ := doc["version"].(string)
	if !strings.HasPrefix(version, screen.APIVersion+".") {
		t.Errorf("server.json version %q does not start from screen.APIVersion %q", version, screen.APIVersion)
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(version) {
		t.Errorf("server.json version %q is not semver", version)
	}

	remotes, _ := doc["remotes"].([]any)
	if len(remotes) != 1 {
		t.Fatalf("server.json declares %d remotes, want 1", len(remotes))
	}
	remote, _ := remotes[0].(map[string]any)
	if remote["type"] != "streamable-http" {
		t.Errorf("server.json remote type is %v, want streamable-http", remote["type"])
	}
	// The short path, because that is the one a client tries and the one the
	// catalog and llms.txt both name.
	if remote["url"] != "https://dug.sh/mcp" {
		t.Errorf("server.json remote url is %v, want https://dug.sh/mcp", remote["url"])
	}
}

// The AI catalog is what an agent runtime reads to decide whether this domain
// speaks a protocol it can use, so a malformed entry is not a cosmetic problem:
// the entry is skipped and the surface looks absent.
func TestAICatalogEntriesAreWellFormed(t *testing.T) {
	doc, result := fetchJSON(t, aicatalogapi.Handler, "https://dug.sh/.well-known/ai-catalog.json")

	if got := result.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/ai-catalog+json") {
		t.Errorf("ai-catalog.json content type is %q, want application/ai-catalog+json", got)
	}
	if doc["specVersion"] != "1.0" {
		t.Errorf("ai-catalog.json specVersion is %v, want 1.0", doc["specVersion"])
	}

	entries, _ := doc["entries"].([]any)
	if len(entries) == 0 {
		t.Fatal("ai-catalog.json has no entries")
	}

	// urn:air:<publisher>:<namespace>:<name>, publisher being the fqdn this is
	// served from.
	urn := regexp.MustCompile(`^urn:air:dug\.sh:[a-z0-9-]+:[a-z0-9-]+$`)

	for i, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("entry %d is not an object", i)
		}

		identifier, _ := entry["identifier"].(string)
		if !urn.MatchString(identifier) {
			t.Errorf("entry %d identifier %q is not a domain-anchored urn:air", i, identifier)
		}
		if name, _ := entry["displayName"].(string); name == "" {
			t.Errorf("entry %d has no displayName, which the spec requires", i)
		}
		if kind, _ := entry["type"].(string); !strings.Contains(kind, "/") {
			t.Errorf("entry %d type %q is not a media type", i, kind)
		}

		// Exactly one of url or data. Both is invalid, not merely redundant.
		_, hasURL := entry["url"]
		_, hasData := entry["data"]
		if hasURL == hasData {
			t.Errorf("entry %d must carry exactly one of url and data", i)
		}

		// The discovery signal. Without it an entry cannot be found by search,
		// which is the whole reason for publishing one.
		queries, _ := entry["representativeQueries"].([]any)
		if len(queries) < 2 || len(queries) > 5 {
			t.Errorf("entry %d has %d representativeQueries, the spec asks for 2 to 5", i, len(queries))
		}
	}
}

// The capability list is generated from the command registry. If it were hand
// written it would be a list of tools that used to exist.
func TestAICatalogCapabilitiesMatchTheCommandSet(t *testing.T) {
	doc, _ := fetchJSON(t, aicatalogapi.Handler, "https://dug.sh/.well-known/ai-catalog.json")

	entries, _ := doc["entries"].([]any)
	for i, raw := range entries {
		entry, _ := raw.(map[string]any)
		capabilities, _ := entry["capabilities"].([]any)
		if len(capabilities) != len(commands.List) {
			t.Errorf("entry %d lists %d capabilities, the command registry has %d",
				i, len(capabilities), len(commands.List))
		}
	}
}

// The card's whole promise is that a client can trust it instead of connecting.
// So every claim it makes has to be the claim the server would make, and the
// only way to keep that true is for both to come from pkg/mcpx — which this
// checks by comparing the card against a live initialize and tools/list.
func TestServerCardMatchesWhatTheServerAnswers(t *testing.T) {
	card, result := fetchJSON(t, servercardapi.Handler, "https://dug.sh/.well-known/mcp/server-card.json")

	// SEP-1649 requires application/json specifically, and CORS, because a
	// browser client reading the card cross-origin is what it is for.
	if got := result.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("server card content type is %q, the SEP requires application/json", got)
	}
	if got := result.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("server card sends Access-Control-Allow-Origin %q, the SEP requires it", got)
	}

	// The fields the SEP marks required. $schema is deliberately absent: the
	// url the proposal names has never been published.
	for _, field := range []string{"version", "protocolVersion", "serverInfo", "transport", "capabilities"} {
		if _, ok := card[field]; !ok {
			t.Errorf("server card has no %q, which the SEP requires", field)
		}
	}
	if _, ok := card["$schema"]; ok {
		t.Error("server card cites a $schema; the url the SEP names does not resolve, so it must stay out")
	}

	// Now the part that matters: does it agree with the server?
	initialize := rpc(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)

	if card["protocolVersion"] != initialize["protocolVersion"] {
		t.Errorf("card says protocol %v, the server answers %v", card["protocolVersion"], initialize["protocolVersion"])
	}
	if !reflect.DeepEqual(card["serverInfo"], initialize["serverInfo"]) {
		t.Errorf("card serverInfo %v does not match the server's %v", card["serverInfo"], initialize["serverInfo"])
	}
	if !reflect.DeepEqual(card["capabilities"], initialize["capabilities"]) {
		t.Errorf("card capabilities %v do not match the server's %v", card["capabilities"], initialize["capabilities"])
	}
	if card["instructions"] != initialize["instructions"] {
		t.Error("card instructions do not match the server's")
	}

	// The tool list is static rather than the reserved "dynamic", so it has to
	// be the same list, not merely the same length.
	listed := rpc(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if !reflect.DeepEqual(card["tools"], listed["tools"]) {
		cardTools, _ := card["tools"].([]any)
		serverTools, _ := listed["tools"].([]any)
		t.Errorf("card advertises %d tools that differ from the %d the server lists",
			len(cardTools), len(serverTools))
	}

	// The transport has to point somewhere that answers.
	transport, _ := card["transport"].(map[string]any)
	if transport["type"] != "streamable-http" {
		t.Errorf("card transport type is %v, want streamable-http", transport["type"])
	}
	if transport["endpoint"] != "/mcp" {
		t.Errorf("card transport endpoint is %v, want /mcp", transport["endpoint"])
	}
}

// One initialize or tools/list call against the real handler.
func rpc(t *testing.T, body string) map[string]any {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "https://dug.sh/mcp", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	mcpapi.Handler(recorder, request)

	var response struct {
		Result map[string]any `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(recorder.Result().Body).Decode(&response); err != nil {
		t.Fatalf("decoding the rpc response: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("the server refused the call: %s", response.Error.Message)
	}
	return response.Result
}

// The catalog is the one document that is supposed to name every other one. It
// missed the two added alongside it once already, which is how a discovery
// document quietly stops being a discovery document.
func TestCatalogNamesTheDiscoveryDocuments(t *testing.T) {
	recorder := httptest.NewRecorder()
	catalogapi.Handler(recorder, httptest.NewRequest(http.MethodGet, "https://dug.sh/.well-known/api-catalog", nil))
	body := recorder.Body.String()

	for _, want := range []string{
		"https://dug.sh/openapi.json",
		"https://dug.sh/llms.txt",
		"https://dug.sh/.well-known/ai-catalog.json",
		"https://dug.sh/.well-known/mcp/server-card.json",
		"https://dug.sh/server.json",
		"https://dug.sh/mcp",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the api catalog does not name %s", want)
		}
	}
}

// Each of these is a fixed path — three by specification or convention, one to
// undo a 404. None of them is served by a file, so the rewrite is the only
// thing making them exist.
func TestDiscoveryPathsAreRewritten(t *testing.T) {
	config := read(t, repoRoot(t), "next.config.ts")

	for _, route := range []struct{ source, destination string }{
		{"/llms.txt", "/api/llms"},
		{"/openapi.json", "/api/openapi"},
		{"/.well-known/api-catalog", "/api/catalog"},
		{"/.well-known/ai-catalog.json", "/api/aicatalog"},
		{"/.well-known/mcp/server-card.json", "/api/servercard"},
		{"/server.json", "/api/server"},
		{"/mcp", "/api/mcp"},
	} {
		if !strings.Contains(config, `source: "`+route.source+`"`) {
			t.Errorf("next.config.ts has no rewrite for %s", route.source)
		}
		if !strings.Contains(config, `api("`+route.destination+`")`) {
			t.Errorf("next.config.ts never routes to %s", route.destination)
		}
	}
}

// proxy.ts answers a json 404 for any /api path not on this list, so a handler
// that exists but is not listed is unreachable by its own name.
func TestProxyKnowsEveryDiscoveryEndpoint(t *testing.T) {
	proxy := read(t, repoRoot(t), "proxy.ts")

	for _, endpoint := range []string{"/api/catalog", "/api/aicatalog", "/api/server", "/api/servercard", "/api/mcp"} {
		if !strings.Contains(proxy, `"`+endpoint+`"`) {
			t.Errorf("proxy.ts API_ENDPOINTS does not list %s, so it answers 404 there", endpoint)
		}
	}
}

// /mcp rewrites to /api/mcp and every tool call runs a real command through it.
// Counting only the long spelling would make the published quota opt-in.
func TestMcpShortPathIsRateLimited(t *testing.T) {
	proxy := read(t, repoRoot(t), "proxy.ts")

	limiter := regexp.MustCompile(`if \(pathname\.startsWith\("/api/"\)[^)]*\) \{`).FindString(proxy)
	if limiter == "" {
		t.Fatal("could not find the rate limit branch in proxy.ts")
	}
	if !strings.Contains(limiter, `pathname === "/mcp"`) {
		t.Errorf("the rate limit branch does not cover /mcp: %s", limiter)
	}
}

var pagePathRE = regexp.MustCompile(`"(/[a-z-]*)"`)

// Three lists name the html pages: the sitemap, the Link headers in
// next.config.ts, and the set proxy.ts uses to decide whether a markdown
// request is a 404. A page missing from the second is undiscoverable and a page
// missing from the third answers 404 to any agent that prefers markdown, which
// is what /developers did.
func TestPageListsAgree(t *testing.T) {
	root := repoRoot(t)

	sitemap := map[string]bool{}
	for _, match := range regexp.MustCompile(`url: "https://dug\.sh(/[a-z-]*)?"`).FindAllStringSubmatch(read(t, root, "app", "sitemap.ts"), -1) {
		path := match[1]
		if path == "" {
			path = "/"
		}
		sitemap[path] = true
	}
	if len(sitemap) == 0 {
		t.Fatal("could not read any page out of app/sitemap.ts")
	}

	config := read(t, root, "next.config.ts")
	pages := listAfter(t, config, "const PAGES = [")

	// PAGE_PATHS spreads the keys of PAGES rather than repeating them, so the
	// two blocks have to be read together to see the whole set.
	proxy := read(t, root, "proxy.ts")
	paths := listAfter(t, proxy, "const PAGE_PATHS = new Set([")
	for path := range listAfter(t, proxy, "const PAGES: Record<string, () => string> = {") {
		paths[path] = true
	}

	for path := range sitemap {
		if !pages[path] {
			t.Errorf("%s is in the sitemap but not in next.config.ts PAGES, so it sends no Link header", path)
		}
		if !paths[path] {
			t.Errorf("%s is in the sitemap but not in proxy.ts PAGE_PATHS, so Accept: text/markdown gets a 404", path)
		}
	}
	for path := range pages {
		if !sitemap[path] {
			t.Errorf("next.config.ts PAGES has %s, which is not in the sitemap", path)
		}
	}
}

func listAfter(t *testing.T, body, marker string) map[string]bool {
	t.Helper()
	start := strings.Index(body, marker)
	if start == -1 {
		t.Fatalf("could not find %q", marker)
	}
	// Either an array or an object literal, so stop at whichever bracket closes
	// this one.
	rest := body[start+len(marker):]
	end := strings.IndexAny(rest, "]}")
	if end == -1 {
		t.Fatalf("%q is never closed", marker)
	}

	found := map[string]bool{}
	for _, match := range pagePathRE.FindAllStringSubmatch(rest[:end], -1) {
		found[match[1]] = true
	}
	if len(found) == 0 {
		t.Fatalf("%q lists no paths", marker)
	}
	return found
}

// The relations are set in three places for two different readers: the Go
// handlers set them on every api response, next.config.ts sets them on the html
// pages, and the layout repeats them as <link> elements for a crawler that
// keeps the body and drops the headers. All three have to name the same
// documents with the same media types — the api half advertised openapi.json as
// application/json for a while after it stopped being served as that.
func TestServiceLinksAreSetInBothPlaces(t *testing.T) {
	root := repoRoot(t)
	config := read(t, root, "next.config.ts")
	// A formatter breaks a long <link> across lines, so match on the collapsed
	// text rather than on how it happens to be wrapped today.
	layout := regexp.MustCompile(`\s+`).ReplaceAllString(read(t, root, "app", "layout.tsx"), " ")

	for _, want := range []struct{ rel, href string }{
		{"service-desc", "/openapi.json"},
		{"service-doc", "/developers"},
		{"describedby", "/llms.txt"},
		{"describedby", "/.well-known/ai-catalog.json"},
	} {
		if !strings.Contains(config, `<`+want.href+`>; rel="`+want.rel+`"`) {
			t.Errorf("next.config.ts sends no Link rel=%q for %s", want.rel, want.href)
		}
		if !strings.Contains(layout, `rel="`+want.rel+`" href="`+want.href+`"`) {
			t.Errorf("app/layout.tsx has no <link rel=%q href=%q>", want.rel, want.href)
		}
		if !strings.Contains(screen.ServiceLinks, `<`+want.href+`>; rel="`+want.rel+`"`) {
			t.Errorf("screen.ServiceLinks sends no rel=%q for %s", want.rel, want.href)
		}
	}

	// Not just the same relations: the same media types. The header is the only
	// thing telling a client what it will get before it fetches.
	for _, header := range strings.Split(screen.ServiceLinks, ", ") {
		if !strings.Contains(config, strings.TrimSpace(header)) {
			t.Errorf("next.config.ts does not send the Link value the api sends: %s", header)
		}
	}
}

// The policy moved out of a fragment on /developers and onto its own page,
// because nothing can link to, index or cite a fragment — which is why nothing
// ever found it.
func TestDeprecationPolicyHasItsOwnPage(t *testing.T) {
	root := repoRoot(t)

	page := read(t, root, "app", "deprecation", "page.tsx")
	for _, want := range []string{"Deprecation", "Sunset", "180 days"} {
		if !strings.Contains(page, want) {
			t.Errorf("app/deprecation/page.tsx does not mention %q", want)
		}
	}

	if strings.Contains(read(t, root, "app", "developers", "page.tsx"), `id="deprecation"`) {
		t.Error("the deprecation fragment is back on /developers; the policy lives at /deprecation")
	}

	if !strings.Contains(read(t, root, "api", "llms", "index.go"), "/deprecation") {
		t.Error("llms.txt does not name /deprecation, so an agent has no pointer to the policy")
	}
}

// The same command set is offered twice, in the page and over /mcp, and the
// two sets of annotations described the same answers differently: the page
// said untrusted, the server said nothing. Both feed agents, so both say it.
func TestRemoteAndPageToolsAgreeTheContentIsUntrusted(t *testing.T) {
	root := repoRoot(t)

	page := read(t, root, "lib", "webmcp.ts")
	if !strings.Contains(page, "untrustedContentHint: true") {
		t.Fatal("lib/webmcp.ts no longer marks tool content untrusted; decide for both sides, not one")
	}

	server := read(t, root, "pkg", "mcpx", "mcpx.go")
	if !strings.Contains(server, `"untrustedContentHint": true`) {
		t.Error("pkg/mcpx/mcpx.go does not mark tool content untrusted, so tools/list and the server card omit it")
	}
}
