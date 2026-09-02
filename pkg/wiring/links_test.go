package wiring

import (
	"strings"
	"testing"
)

// A command url is the one llms.txt tells agents to call, and also the one a
// person shares. Both must keep working: curl gets its representation from Go,
// and a browser navigation is sent to the app with the command in the query,
// which the landing runs on mount. If either half goes, a shared link is raw
// json on a phone again.
func TestSharedLinksOpenTheApp(t *testing.T) {
	root := repoRoot(t)

	proxy := read(t, root, "proxy.ts")
	if !strings.Contains(proxy, `searchParams.set("run"`) || !strings.Contains(proxy, "NextResponse.redirect(") {
		t.Error("proxy.ts does not redirect a browser navigation on a command path to /?run=")
	}
	if !strings.Contains(proxy, `searchParams.has("format")`) {
		t.Error("proxy.ts redirects even when ?format= asked for a representation explicitly")
	}
	if !strings.Contains(proxy, `"sec-fetch-mode"`) {
		t.Error("proxy.ts does not distinguish a navigation from a fetch, so a page's own fetch() with an html Accept would be redirected")
	}

	page := read(t, root, "app", "page.tsx")
	if !strings.Contains(page, `.get("run")`) {
		t.Error("app/page.tsx does not read the run query the proxy sends it")
	}
	if !strings.Contains(page, `history.replaceState(null, "", "/")`) {
		t.Error("app/page.tsx does not clear the run query, so a reload re-runs the command and a development double mount runs it twice")
	}
}
