package wiring

// pkg/commands is the grammar in Go, and llms.txt, the OpenAPI document
// and the MCP tool list are all generated from it. app/commands/grammar.ts is
// the same grammar for the browser. A drift between them means an agent is
// told about a command the browser does not have, or offered the wrong
// argument for one it does, and nothing else would catch it.

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/zaidmukaddam/dug/pkg/commands"
)

// Each spec in grammar.ts, in source order.
var specRE = regexp.MustCompile(`name:\s*"([A-Z]+)",\s*\n\s*family:\s*"([a-z]+)",\s*\n\s*endpoint:\s*"([^"]*)",\s*\n\s*argument:\s*"([a-z]+)",\s*\n\s*summary:\s*"([^"]*)",\s*\n\s*example:\s*"([^"]*)"`)

func TestCommandListsMatch(t *testing.T) {
	root := repoRoot(t)
	found := specRE.FindAllStringSubmatch(read(t, root, "app", "commands", "grammar.ts"), -1)

	if len(found) == 0 {
		t.Fatal("could not parse any command out of grammar.ts")
	}

	// HELP is browser-only: it renders the grammar with no upstream, so the Go
	// list leaves it out on purpose. Everything else must appear in both.
	fromTS := map[string][5]string{}
	order := []string{}
	for _, match := range found {
		if match[1] == "HELP" {
			continue
		}
		fromTS[match[1]] = [5]string{match[2], match[3], match[4], match[5], match[6]}
		order = append(order, match[1])
	}

	if len(order) != len(commands.List) {
		t.Fatalf("grammar.ts has %d commands (HELP excluded), pkg/commands has %d",
			len(order), len(commands.List))
	}

	for i, name := range order {
		spec := commands.List[i]
		if spec.Name != name {
			t.Fatalf("position %d: grammar.ts has %s, pkg/commands has %s", i, name, spec.Name)
		}

		got := [5]string{spec.Family, spec.Endpoint, spec.Argument, spec.Summary, spec.Example}
		if want := fromTS[name]; got != want {
			t.Errorf("%s drifted:\n  typescript %v\n  go         %v", name, want, got)
		}
	}
}

// The pretty paths are derived from the verb in next.config.ts and written out
// in pkg/commands. They have to agree, or the route an agent reads in
// llms.txt is not the route the rewrite serves.
func TestPrettyPathsMatchTheRewriteRule(t *testing.T) {
	for _, spec := range commands.List {
		verb := strings.ToLower(spec.Name)

		want := "/" + verb + "/{target}"
		switch spec.Argument {
		case "none":
			want = "/" + verb
		case "pair":
			want = "/" + verb + "/{target}/{other}"
		case "cidr":
			// the argument itself contains a slash, so the rewrite spends a
			// second segment on the prefix length and rejoins the two
			want = "/" + verb + "/{target}/{bits}"
		}

		if spec.Path != want {
			t.Errorf("%s: path is %q, but the rewrite in next.config.ts serves %q",
				spec.Name, spec.Path, want)
		}
	}
}

// Every command must be dispatchable by the MCP server, or a tool it lists
// cannot be called.
func TestEveryCommandHasAnMCPRoute(t *testing.T) {
	root := repoRoot(t)
	mcp := read(t, root, "api", "mcp", "index.go")

	_, block, found := strings.Cut(mcp, "var byEndpoint = map[string]http.HandlerFunc{")
	if !found {
		t.Fatal("could not find the MCP endpoint table")
	}
	block, _, _ = strings.Cut(block, "}")

	var missing []string
	for _, spec := range commands.List {
		if !strings.Contains(block, `"`+spec.Endpoint+`"`) {
			missing = append(missing, spec.Name+" -> "+spec.Endpoint)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("MCP lists tools it cannot dispatch: %v", missing)
	}
}

// The tool prefix is spelled in three places, and each drift fails differently.
// Server list against server dispatch is the quiet one: tools/list keeps
// advertising the right names while every call trims a prefix that is no longer
// there and comes back "unknown tool". Server against browser is louder but
// worse, because an agent inside the page and one outside it end up calling
// differently named tools for the same command.
//
// The first two now live together in pkg/mcpx, which is a real improvement —
// they are two lines apart rather than two files apart. They are still both
// checked, because "adjacent" is not "the same string".
var (
	mcpListedPrefix  = regexp.MustCompile(`return "([a-z]+)_"\s*\+\s*strings\.ToLower`)
	mcpTrimmedPrefix = regexp.MustCompile(`strings\.TrimPrefix\(name,\s*"([a-z]+)_"\)`)
	webMcpPrefix     = regexp.MustCompile("`([a-z]+)_\\$\\{spec\\.name\\.toLowerCase\\(\\)\\}`")
)

func TestMCPToolPrefixesAgree(t *testing.T) {
	root := repoRoot(t)

	prefix := func(what string, re *regexp.Regexp, body string) string {
		match := re.FindStringSubmatch(body)
		if match == nil {
			t.Fatalf("could not read the %s tool prefix: the code this greps for has moved, so the check is no longer guarding anything", what)
		}
		return match[1]
	}

	mcp := read(t, root, "pkg", "mcpx", "mcpx.go")
	listed := prefix("name pkg/mcpx builds", mcpListedPrefix, mcp)
	trimmed := prefix("prefix pkg/mcpx trims to dispatch", mcpTrimmedPrefix, mcp)
	browser := prefix("name lib/webmcp.ts registers", webMcpPrefix, read(t, root, "lib", "webmcp.ts"))

	if listed != trimmed {
		t.Errorf("pkg/mcpx names tools %s_* but dispatches by trimming %s_, so every tools/call fails as an unknown tool",
			listed, trimmed)
	}
	if listed != browser {
		t.Errorf("pkg/mcpx registers %s_* and lib/webmcp.ts registers %s_*", listed, browser)
	}
}

// WebMCP has moved twice and this file shipped against the first shape, which
// registered nothing in any browser. These pin the parts that were wrong, so a
// revert to the removed API fails here rather than silently on the page.
func TestWebMcpTargetsTheCurrentAPI(t *testing.T) {
	body := read(t, repoRoot(t), "lib", "webmcp.ts")

	// provideContext({tools}) was removed from the spec: it declared the whole
	// tool list at once, so on a page with more than one script registering
	// tools, replace-all is takeover.
	if regexp.MustCompile(`\bcontext\.provideContext\b`).MatchString(body) {
		t.Error("webmcp.ts calls provideContext, which was removed from the spec")
	}

	// document.modelContext is canonical. navigator.modelContext is a
	// deprecated alias and is absent in browsers that only have the new shape,
	// so reading it first registers nothing there.
	if !strings.Contains(body, "document as Document & { modelContext?: ModelContext }") {
		t.Error("webmcp.ts does not read document.modelContext, which is the canonical entry point")
	}

	// Removal is by aborting the signal handed to registerTool. Without the
	// signal there is no way to take a tool back off the page.
	if !strings.Contains(body, "signal: controller.signal") {
		t.Error("webmcp.ts does not pass an AbortSignal to registerTool, so tools cannot be removed")
	}
	if !strings.Contains(body, "new AbortController()") {
		t.Error("webmcp.ts has no AbortController, so the cleanup path removes nothing")
	}

	// The marker said nothing on a page carrying its whole tool set, twice over,
	// and these are the two shapes that caused it.
	//
	// One: returning early when the signal is aborted. It fires between React's
	// two mount passes and on unmount, and neither answers whether an agent can
	// find the tools.
	if regexp.MustCompile(`\.aborted\) \{\n\s*return\n`).MatchString(body) {
		t.Error("webmcp.ts returns early on an aborted signal, which skips the mark on a page whose tools are present")
	}

	// Two: waiting on the registrations. In a production build at least one
	// registerTool promise never settles, so gating the check on Promise.all
	// meant it never ran at all. Development hid it — the abort between React's
	// two passes rejected the pending calls, which is the only reason it ever
	// completed there.
	if strings.Contains(body, "Promise.all(") && !strings.Contains(body, "Promise.race(") {
		t.Error("webmcp.ts waits on Promise.all over registerTool with no bound, and those calls do not always settle")
	}
	if !strings.Contains(body, "SETTLE_BUDGET_MS") {
		t.Error("webmcp.ts has no bounded wait for the tools to come up, so the mark can hang or fire early")
	}
	if !strings.Contains(body, "await present(context)") {
		t.Error("webmcp.ts does not decide the mark from getTools, which is the only thing that answers it")
	}
}

// llms.txt tells an agent to read data-webmcp-server before concluding a page
// has no tools, so the value has to be a path that answers.
func TestWebMcpServerAttributePointsAtTheAdvertisedPath(t *testing.T) {
	body := read(t, repoRoot(t), "lib", "webmcp.ts")

	if !strings.Contains(body, `dataset.webmcpServer = "/mcp"`) {
		t.Error("webmcp.ts does not point data-webmcp-server at /mcp, the path llms.txt and server.json name")
	}
}

// Native WebMCP is refused outright in a document that is not origin-isolated,
// so the header is load-bearing rather than decorative.
func TestOriginAgentClusterHeaderIsSet(t *testing.T) {
	config := read(t, repoRoot(t), "next.config.ts")
	if !strings.Contains(config, "Origin-Agent-Cluster") {
		t.Error("next.config.ts does not send Origin-Agent-Cluster, which native WebMCP requires")
	}
}
