package handler

// A tool call is the one place a caller writes the query a handler will read,
// and an MCP client only ever allowlisted the tool it thought it was calling.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zaidmukaddam/dug/pkg/commands"
)

// Arguments as they arrive: over the wire, where 4 is a JSON number and
// decodes as a float64.
func arguments(t *testing.T, raw string) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("test arguments do not parse: %v", err)
	}
	return decoded
}

func TestToolQueryTakesOnlyDeclaredArguments(t *testing.T) {
	spec, ok := commands.ByName("PING")
	if !ok {
		t.Fatal("PING is not in the command list")
	}

	// PORTS is a port scan; PING is an icmp echo. A client that permits the
	// echo must not be able to reach the scan through it.
	query, err := toolQuery(spec, arguments(t, `{
		"target": "1.1.1.1",
		"command": "PORTS",
		"format": "json",
		"ports": "22,80",
		"count": 4
	}`))
	if err != nil {
		t.Fatalf("toolQuery: %v", err)
	}

	if got := query.Get("command"); got != "PING" {
		t.Errorf("command is %q, an argument overrode the dispatched command", got)
	}
	if got := query.Get("format"); got != "text" {
		t.Errorf("format is %q, want text", got)
	}
	if got := query.Get("ports"); got != "" {
		t.Errorf("ports is %q, PING declares no such argument", got)
	}
	if got := query.Get("target"); got != "1.1.1.1" {
		t.Errorf("target is %q, want 1.1.1.1", got)
	}
	// A model sends a count unquoted as often as quoted, and float64 formatting
	// would otherwise hand the handler "4.000000".
	if got := query.Get("count"); got != "4" {
		t.Errorf("count is %q, want 4", got)
	}
}

// A rebinding attacker names the host, so the host cannot be what decides
// whether the check runs.
// The case that matters is the rebinding shape, where Origin and Host are the
// same attacker-owned name. Any check that compares the two passes it, which is
// why neither may be the reference value.
func TestOriginCheck(t *testing.T) {
	cases := []struct {
		name   string
		host   string
		origin string
		want   bool
	}{
		{name: "rebinding, origin equals host", host: "rebind.evil.com:8787",
			origin: "http://rebind.evil.com:8787"},
		{name: "plain cross origin", host: "127.0.0.1:8787", origin: "https://evil.com"},
		{name: "opaque origin", host: "127.0.0.1:8787", origin: "null"},
		{name: "a name that merely resolves to loopback", host: "127.0.0.1:8787",
			origin: "http://localtest.me:3000"},
		{name: "the dev app on localhost", host: "127.0.0.1:8787",
			origin: "http://localhost:3000", want: true},
		{name: "the dev app on the loopback address", host: "127.0.0.1:8787",
			origin: "http://127.0.0.1:3000", want: true},
		{name: "ipv6 loopback", host: "[::1]:8787", origin: "http://[::1]:3000", want: true},
		{name: "no origin, so not a browser", host: "127.0.0.1:8787", want: true},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/mcp", nil)
			request.Host = test.host
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}

			if got := originAllowed(request); got != test.want {
				t.Errorf("originAllowed(host=%q origin=%q) = %v, want %v",
					test.host, test.origin, got, test.want)
			}
		})
	}
}

// On the hosted copy the server is public, read-only and keyless, so a browser
// from anywhere is welcome and the platform is what says so.
func TestOriginCheckIsOpenOnVercel(t *testing.T) {
	t.Setenv("VERCEL", "1")

	request := httptest.NewRequest(http.MethodPost, "/api/mcp", nil)
	request.Host = "resolve.example"
	request.Header.Set("Origin", "https://somewhere.else")

	if !originAllowed(request) {
		t.Error("the hosted deployment refused a cross-origin browser client")
	}
}

func TestToolQueryRefusesNonScalarArguments(t *testing.T) {
	spec, _ := commands.ByName("DIG")
	if _, err := toolQuery(spec, arguments(t, `{"target": ["example.com"]}`)); err == nil {
		t.Error("an array argument was accepted")
	}
}
