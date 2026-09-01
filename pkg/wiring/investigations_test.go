package wiring

import (
	"regexp"
	"strings"
	"testing"

	"github.com/zaidmukaddam/dug/pkg/commands"
)

// An investigation is a list of command lines built by string interpolation and
// never parsed until someone clicks it. A verb that does not exist, or one that
// wants an ip address when the investigation hands it a domain, fails at the
// point where a person is watching a case build — and it fails as one dead step
// in the middle of four live ones, which reads as the target's problem rather
// than as ours.
//
// Nothing else checks this. The Go registry has never heard of investigations
// and the TypeScript compiler cannot tell a command name from any other string.

var (
	investigationID   = regexp.MustCompile(`id:\s*"([a-z]+)"`)
	investigationStep = regexp.MustCompile("`([A-Z]+) \\$\\{target\\}([^`]*)`")
)

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

// Every investigation needs a sample target on the landing, because that is the
// only way a person without an agent can start one. A missing entry renders a
// button that runs the case against the string "undefined".
func TestEveryInvestigationHasALandingSample(t *testing.T) {
	root := repoRoot(t)

	ids := investigationID.FindAllStringSubmatch(read(t, root, "lib", "investigations.ts"), -1)
	if len(ids) == 0 {
		t.Fatal("no investigation ids found")
	}

	page := read(t, root, "app", "page.tsx")
	samples := page[strings.Index(page, "const SAMPLE"):]
	samples = samples[:strings.Index(samples, "}")]

	for _, id := range ids {
		if !strings.Contains(samples, id[1]+":") {
			t.Errorf("investigation %q has no sample target, so its landing button investigates \"undefined\"", id[1])
		}
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
