package wiring

import (
	"regexp"
	"strings"
	"testing"

	"github.com/zaidmukaddam/dug/pkg/commands"
)

// An investigation is a list of command lines written with a {target}
// placeholder and never parsed until someone clicks it. A verb that does not exist, or one that
// wants an ip address when the investigation hands it a domain, fails at the
// point where a person is watching a case build — and it fails as one dead step
// in the middle of four live ones, which reads as the target's problem rather
// than as ours.
//
// Nothing else checks this. The Go registry has never heard of investigations
// and the TypeScript compiler cannot tell a command name from any other string.

var investigationStep = regexp.MustCompile(`"([A-Z]+) \{target\}([^"]*)"`)

// What a domain can be handed to. A domain is also a valid host and a valid
// endpoint; it is not an address, an asn, a cidr or a pair.
var takesADomain = map[string]bool{"domain": true, "host": true, "endpoint": true}

func TestInvestigationStepsAreRealCommands(t *testing.T) {
	body := read(t, repoRoot(t), "lib", "investigations.ts")

	steps := investigationStep.FindAllStringSubmatch(body, -1)
	if len(steps) == 0 {
		t.Fatal("no investigation steps found: the shape this greps for has moved, so nothing is being checked")
	}

	for _, step := range steps {
		name, extra := step[1], strings.TrimSpace(step[2])

		spec, ok := commands.ByName(name)
		if !ok {
			t.Errorf("an investigation runs %q, which is not a command", name)
			continue
		}
		if spec.Endpoint == "" {
			t.Errorf("an investigation runs %s, which has no endpoint to answer it", name)
		}
		if !takesADomain[spec.Argument] {
			t.Errorf("an investigation hands %s a domain, but it takes a %s", name, spec.Argument)
		}

		// A second word is a second argument, and only some commands have one.
		if extra != "" && len(spec.Params) == 0 {
			t.Errorf("an investigation calls %s %s, but %s takes no second argument", name, extra, name)
		}
	}
}

// WHY is answered before parse() and is not in the command grammar, so nothing
// in the grammar guards it. What it must not do is swallow input: a topic that
// does not exist, or a topic with no target, has to fall through to the normal
// parse and get a real error rather than quietly doing nothing.
func TestWhyFallsThroughWhenItIsNotAnInvestigation(t *testing.T) {
	body := read(t, repoRoot(t), "lib", "investigations.ts")

	match := strings.Index(body, "export function matchInvestigation")
	if match == -1 {
		t.Fatal("matchInvestigation is gone, so WHY at the prompt does nothing")
	}
	fn := body[match:]
	if end := strings.Index(fn, "\nexport "); end != -1 {
		fn = fn[:end]
	}

	// Exactly one null: the verb is not WHY, so it belongs to parse. A mistyped
	// topic must come back as a failure instead, or the prompt reports "why is
	// not a command" about the one word that was right.
	if got := strings.Count(fn, "return null"); got != 1 {
		t.Errorf("matchInvestigation returns null %d times, want 1 — only a non-WHY verb falls through", got)
	}
	for _, guard := range []string{"!investigation", "targets.length === 0"} {
		if !strings.Contains(fn, guard) {
			t.Errorf("matchInvestigation does not handle %s", guard)
		}
	}
	if strings.Count(fn, "ok: false") != 2 {
		t.Error("matchInvestigation does not report both an unknown topic and a missing target")
	}

	// The prompt has to reach it before parse, or WHY is just an unknown command.
	page := read(t, repoRoot(t), "app", "page.tsx")
	if strings.Index(page, "matchInvestigation(text)") > strings.Index(page, "if (await run(text))") {
		t.Error("submit calls run before matchInvestigation, so WHY never reaches an investigation")
	}
}

// The tool that only exists in the page. If it stops being registered the
// investigations are still clickable by a person and invisible to an agent,
// which is the half that makes them worth having.
func TestInvestigateIsRegisteredAsAWebMcpTool(t *testing.T) {
	body := read(t, repoRoot(t), "lib", "webmcp.ts")

	if !strings.Contains(body, `name: "dug_investigate"`) {
		t.Error("lib/webmcp.ts does not register dug_investigate")
	}
	// It has to reach the page's own state to render anything. Calling the
	// command endpoints directly would return the same data and draw nothing.
	if !strings.Contains(body, "investigateLatest(") {
		t.Error("dug_investigate does not run through the page, so it renders nothing for the person watching")
	}
}
