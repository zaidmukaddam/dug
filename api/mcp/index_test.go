package handler

// A tool call is the one place a caller writes the query a handler will read,
// and an MCP client only ever allowlisted the tool it thought it was calling.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zaidmukaddam/dug/pkg/commands"
	"github.com/zaidmukaddam/dug/pkg/mcpx"
)

// rpcCall runs one JSON-RPC request through the real Handler and decodes the
// response, the way pkg/wiring's discovery tests do.
func rpcCall(t *testing.T, body string) rpcResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/mcp", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	Handler(recorder, request)

	var response rpcResponse
	if err := json.NewDecoder(recorder.Result().Body).Decode(&response); err != nil {
		t.Fatalf("decoding the rpc response: %v", err)
	}
	return response
}

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

// The resource list has exactly the one entry the server card publishes, at
// the uri a client would then read it from.
func TestResourcesListReturnsTheCard(t *testing.T) {
	response := rpcCall(t, `{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}`)
	if response.Error != nil {
		t.Fatalf("resources/list returned an error: %v", response.Error)
	}

	result, _ := response.Result.(map[string]any)
	resources, _ := result["resources"].([]any)
	if len(resources) != 1 {
		t.Fatalf("resources/list returned %d resources, want 1", len(resources))
	}

	resource, _ := resources[0].(map[string]any)
	if resource["uri"] != mcpx.CardURI {
		t.Errorf("resource uri is %v, want %v", resource["uri"], mcpx.CardURI)
	}
}

// Reading the card resource returns the same document the HTTP card serves:
// the same server identity and the same tool set.
func TestResourcesReadReturnsTheCard(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"` + mcpx.CardURI + `"}}`
	response := rpcCall(t, body)
	if response.Error != nil {
		t.Fatalf("resources/read returned an error: %v", response.Error)
	}

	result, _ := response.Result.(map[string]any)
	contents, _ := result["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("resources/read returned %d contents, want 1", len(contents))
	}

	content, _ := contents[0].(map[string]any)
	if content["mimeType"] != "application/json" {
		t.Errorf("content mimeType is %v, want application/json", content["mimeType"])
	}

	text, _ := content["text"].(string)
	var card struct {
		ServerInfo struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
		Tools []any `json:"tools"`
	}
	if err := json.Unmarshal([]byte(text), &card); err != nil {
		t.Fatalf("resource text does not parse as json: %v", err)
	}
	if card.ServerInfo.Name != "dug" {
		t.Errorf("resource card serverInfo.name is %q, want dug", card.ServerInfo.Name)
	}
	if len(card.Tools) != len(mcpx.Tools()) {
		t.Errorf("resource card lists %d tools, want %d", len(card.Tools), len(mcpx.Tools()))
	}
}

// An unknown uri is the resource-not-found error the spec names, not a
// generic failure.
func TestResourcesReadUnknownURI(t *testing.T) {
	response := rpcCall(t, `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"mcp://nope"}}`)
	if response.Error == nil {
		t.Fatal("resources/read of an unknown uri returned no error")
	}
	if response.Error.Code != -32002 {
		t.Errorf("error code is %d, want -32002", response.Error.Code)
	}
}

// initialize's capabilities have to say resources now, or a client has no way
// to learn the resource exists short of just trying resources/list.
func TestInitializeCapabilitiesIncludeResources(t *testing.T) {
	response := rpcCall(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if response.Error != nil {
		t.Fatalf("initialize returned an error: %v", response.Error)
	}

	result, _ := response.Result.(map[string]any)
	capabilities, _ := result["capabilities"].(map[string]any)
	if _, ok := capabilities["resources"]; !ok {
		t.Errorf("initialize capabilities %v do not include resources", capabilities)
	}
}
