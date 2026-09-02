// Package wiring checks that the Go backend and the TypeScript frontend agree.
//
// Three drifts this catches, all silent until a request runs:
//
//   - lib/resolvers.ts diverging from internal/resolvers. The Go list is what
//     gets queried and the TypeScript list is what gets displayed, so a drift
//     means the screen names a resolver that was never asked.
//   - A command in the grammar routed at an endpoint with no handler package.
//   - A handler emitting a component the frontend registry cannot render.
package wiring

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/zaidmukaddam/dug/pkg/resolvers"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not find repo root")
	return ""
}

func read(t *testing.T, root string, parts ...string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(append([]string{root}, parts...)...))
	if err != nil {
		t.Fatalf("reading %v: %v", parts, err)
	}
	return string(body)
}

var resolverRE = regexp.MustCompile(`\{\s*id:\s*"([^"]+)",\s*name:\s*"([^"]+)",\s*short:\s*"([^"]+)",\s*ip:\s*"([^"]+)"\s*\}`)

func TestResolverListsMatch(t *testing.T) {
	root := repoRoot(t)
	found := resolverRE.FindAllStringSubmatch(read(t, root, "lib", "resolvers.ts"), -1)

	if len(found) != len(resolvers.List) {
		t.Fatalf("lib/resolvers.ts has %d entries, internal/resolvers has %d", len(found), len(resolvers.List))
	}
	for i, match := range found {
		want := resolvers.List[i]
		got := [4]string{match[1], match[2], match[3], match[4]}
		if got != [4]string{want.ID, want.Name, want.Short, want.IP} {
			t.Errorf("resolver %d drifted: typescript %v, go %v", i, got, want)
		}
	}
}

var endpointRE = regexp.MustCompile(`endpoint:\s*"/api/([a-z]+)"`)

func TestEveryRoutedEndpointHasAHandler(t *testing.T) {
	root := repoRoot(t)
	grammar := read(t, root, "app", "commands", "grammar.ts")

	seen := map[string]bool{}
	for _, match := range endpointRE.FindAllStringSubmatch(grammar, -1) {
		seen[match[1]] = true
	}
	if len(seen) == 0 {
		t.Fatal("no endpoints found in the grammar")
	}

	for endpoint := range seen {
		path := filepath.Join(root, "api", endpoint, "index.go")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("grammar routes /api/%s but api/%s/index.go does not exist", endpoint, endpoint)
		}
	}
}

var commandRE = regexp.MustCompile(`name:\s*"([A-Z]+)",\s*\n\s*family:`)

func TestGrammarCommandSet(t *testing.T) {
	root := repoRoot(t)
	grammar := read(t, root, "app", "commands", "grammar.ts")

	got := map[string]bool{}
	for _, match := range commandRE.FindAllStringSubmatch(grammar, -1) {
		got[match[1]] = true
	}

	want := []string{
		"DIG", "PROP", "TTL", "NS", "DNSSEC", "RDAP", "WATCH", "TLS", "HTTP",
		"TRACE", "MAIL", "SPF", "IP", "ASN", "NET", "PING", "ROUTE", "PORTS",
		"VS", "SRC", "ME", "SEO", "AEO", "OG", "WEBMCP", "HELP",
	}
	for _, command := range want {
		if !got[command] {
			t.Errorf("grammar is missing %s", command)
		}
		delete(got, command)
	}
	for extra := range got {
		t.Errorf("grammar has an unexpected command %s", extra)
	}
}

var componentRE = regexp.MustCompile(`"(Graph[A-Za-z]+)"`)

func TestEmittedComponentsAreRegistered(t *testing.T) {
	root := repoRoot(t)
	registrySource := read(t, root, "app", "screens", "screen.tsx")

	_, block, found := strings.Cut(registrySource, "const REGISTRY = {")
	if !found {
		t.Fatal("could not find the component registry")
	}
	block, _, _ = strings.Cut(block, "} as unknown")

	registered := map[string]bool{}
	for _, match := range regexp.MustCompile(`\b(Graph[A-Za-z]+)\b`).FindAllStringSubmatch(block, -1) {
		registered[match[1]] = true
	}

	emitted := map[string]bool{}
	err := filepath.Walk(filepath.Join(root, "api"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range componentRE.FindAllStringSubmatch(string(body), -1) {
			emitted[match[1]] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var missing []string
	for component := range emitted {
		if !registered[component] {
			missing = append(missing, component)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("handlers emit components the screen cannot render: %v", missing)
	}

	// The three the scope drops on purpose must stay dropped.
	for _, dropped := range []string{"GraphActivity", "GraphCalendar", "GraphInvoice"} {
		if emitted[dropped] {
			t.Errorf("%s needs history or a billing relationship, which this tool does not have", dropped)
		}
	}
}

// The frontend keys props by name, so a rename on either side breaks silently.
func TestScreenPropsUseExpectedJSONNames(t *testing.T) {
	root := repoRoot(t)
	props := read(t, root, "pkg", "screen", "props.go")

	for _, name := range []string{
		`json:"title"`, `json:"rows"`, `json:"items"`, `json:"headers"`,
		`json:"sections"`, `json:"nodes"`, `json:"events"`, `json:"days"`,
		`json:"columns"`, `json:"values"`, `json:"segments"`, `json:"steps"`,
		`json:"fromLabel"`, `json:"toLabel"`,
	} {
		if !strings.Contains(props, name) {
			t.Errorf("props.go no longer emits %s, which the components read", name)
		}
	}
}

// The API version is stated in two places: screen.APIVersion, which every Go
// response echoes, and API_VERSION in proxy.ts, which refuses a pinned version
// before a request reaches a handler. If they drift, the proxy rejects the very
// version the handlers serve.
func TestAPIVersionsAgree(t *testing.T) {
	root := repoRoot(t)
	proxy := read(t, root, "proxy.ts")

	match := regexp.MustCompile(`const API_VERSION = "([^"]+)"`).FindStringSubmatch(proxy)
	if match == nil {
		t.Fatal("could not find API_VERSION in proxy.ts")
	}
	if match[1] != screen.APIVersion {
		t.Errorf("proxy.ts has version %q, screen.APIVersion is %q", match[1], screen.APIVersion)
	}
}

// The quota is enforced in proxy.ts and published in llms.txt and the OpenAPI
// document from the Go constants. Drift means the documents tell a caller to
// pace against a number nothing applies, which is worse than publishing none.
func TestRateLimitConstantsAgree(t *testing.T) {
	proxy := read(t, repoRoot(t), "proxy.ts")

	for _, test := range []struct {
		name string
		re   string
		want int
	}{
		{"RATE_LIMIT", `const RATE_LIMIT = (\d+)`, screen.RateLimit},
		{"RATE_WINDOW_SECONDS", `const RATE_WINDOW_SECONDS = (\d+)`, screen.RateWindowSeconds},
	} {
		match := regexp.MustCompile(test.re).FindStringSubmatch(proxy)
		if match == nil {
			t.Errorf("could not find %s in proxy.ts", test.name)
			continue
		}
		if match[1] != strconv.Itoa(test.want) {
			t.Errorf("proxy.ts %s is %s, Go publishes %d", test.name, match[1], test.want)
		}
	}
}

// The cells component fills a cell only on the exact value 1, and the net
// handler is its only emitter. A different value on either side draws every
// address as unnamed while the headline counts the named ones, which is what
// shipped once. Both sides are pinned here because neither compiler sees the
// other.
func TestNetGridCellsUseTheValueTheComponentFills(t *testing.T) {
	root := repoRoot(t)

	component := read(t, root, "components", "graph-cells.tsx")
	if !strings.Contains(component, "const filled = cell === 1") {
		t.Fatal("components/graph-cells.tsx no longer fills on cell === 1; update the net handler and this test together")
	}

	handler := read(t, root, "api", "addr", "index.go")
	if !strings.Contains(handler, "value = 1") {
		t.Error("api/addr/index.go does not mark a named address with 1, so the cells component draws it as empty")
	}
	if strings.Contains(handler, "value = 4") {
		t.Error("api/addr/index.go still marks a named address with 4, which the cells component draws as empty")
	}
}
