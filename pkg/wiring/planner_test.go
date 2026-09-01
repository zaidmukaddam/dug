package wiring

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The planner is the only thing on the site that calls a model, and so the
// only thing that costs real money per request. It is a Server Action, which
// is a POST to the page url rather than to a path of its own, so the proxy
// has to recognise it by the Next-Action header. If that check ever goes, a
// public keyless endpoint becomes an open bill.
func TestPlannerIsRateLimited(t *testing.T) {
	proxy := read(t, repoRoot(t), "proxy.ts")

	if !strings.Contains(proxy, `request.headers.has("next-action")`) {
		t.Fatal("proxy.ts does not recognise a Server Action by its next-action header")
	}
	limiter := regexp.MustCompile(`if \(pathname\.startsWith\("/api/"\)[^{]*\{`).FindString(proxy)
	if limiter == "" {
		t.Fatal("could not find the rate limit branch in proxy.ts")
	}
	if !strings.Contains(limiter, "isAction") {
		t.Errorf("the rate limit branch does not include actions: %s", limiter)
	}
}

// A Server Action, not a route. The only caller is the prompt on this page, so
// a route would be a public POST surface with a hand-rolled fetch and an error
// contract to keep in step, and an endpoint agents have to be told not to use.
func TestPlannerIsAnActionNotARoute(t *testing.T) {
	root := repoRoot(t)

	if _, err := os.Stat(filepath.Join(root, "app", "plan", "route.ts")); err == nil {
		t.Error("app/plan/route.ts is back; the planner is a Server Action and must not have a public route")
	}

	action := read(t, root, "app", "plan.ts")
	if !strings.HasPrefix(strings.TrimSpace(action), `"use server"`) {
		t.Error("app/plan.ts does not begin with \"use server\", so it is not a Server Action")
	}
	// Without a key the action must answer cleanly rather than throw, so the
	// prompt shows a sentence with a command to fall back on.
	if !strings.Contains(action, "OPENAI_API_KEY") || !strings.Contains(action, "isn’t configured") {
		t.Error("app/plan.ts does not refuse cleanly when OPENAI_API_KEY is unset")
	}
	// An action is a POST anyone can send once they have the id, so its own
	// input check is the only one there is.
	if !strings.Contains(action, "text.length > 500") {
		t.Error("app/plan.ts does not cap the input, which every action must do for itself")
	}
	// The model must never keep what people type. /privacy says nothing is
	// stored between requests, and the provider's default is to store.
	if !strings.Contains(action, "store: false") {
		t.Error("app/plan.ts does not set store: false on the provider")
	}

	page := read(t, root, "app", "page.tsx")
	if strings.Contains(page, `fetch("/plan"`) {
		t.Error("app/page.tsx still fetches /plan instead of calling the action")
	}
	if !strings.Contains(page, `from "@/app/plan"`) || !strings.Contains(page, "await plan(") {
		t.Error("app/page.tsx does not call the plan action, so the planner is unreachable from the prompt")
	}
	if !strings.Contains(page, `"dug")`) {
		t.Error("a planned case is not attributed to dug, so it would read as planned by an agent that is not there")
	}

	// "why is mail from acme.com going to spam" starts with WHY. The first
	// version handed it to the keyword form, which read "is" as the topic and
	// answered "there is no is investigation", so the planner was unreachable
	// by the one sentence it was built for. A failed WHY may only short-circuit
	// when the input is short enough to be the keyword form.
	submit := page[strings.Index(page, "const submit = useCallback"):]
	submit = submit[:strings.Index(submit, "}, [input,")]
	failedWhy := strings.Index(submit, "setFailure(why.failure)")
	planner := strings.Index(submit, "looksLikeAQuestion(text)")
	if failedWhy == -1 || planner == -1 {
		t.Fatal("submit() no longer has both the WHY failure path and the planner gate")
	}
	guard := submit[:failedWhy]
	if !strings.Contains(guard[strings.LastIndex(guard, "if ("):], "<= 3") {
		t.Error("the WHY failure path is not gated on length, so a sentence starting with why never reaches the planner")
	}
}
