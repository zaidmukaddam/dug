// MCP over Streamable HTTP, stateless.
//
// One tool per command, generated from internal/commands, each dispatched to
// the same handler the HTTP route uses. There is no second implementation of
// any command here: a tool call builds a request and runs the handler, so an
// agent and a browser get the same answer from the same code.
//
// Stateless by choice. Nothing is stored between queries anywhere else in this
// tool, so there is nothing for a session to hold. No Mcp-Session-Id is issued,
// and GET is refused because there is no server-initiated stream to open.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"

	addrapi "github.com/zaidmukaddam/dug/api/addr"
	aeoapi "github.com/zaidmukaddam/dug/api/aeo"
	delegateapi "github.com/zaidmukaddam/dug/api/delegate"
	fetchapi "github.com/zaidmukaddam/dug/api/fetch"
	mailapi "github.com/zaidmukaddam/dug/api/mail"
	meapi "github.com/zaidmukaddam/dug/api/me"
	ogapi "github.com/zaidmukaddam/dug/api/og"
	probeapi "github.com/zaidmukaddam/dug/api/probe"
	propagateapi "github.com/zaidmukaddam/dug/api/propagate"
	rdapapi "github.com/zaidmukaddam/dug/api/rdap"
	resolveapi "github.com/zaidmukaddam/dug/api/resolve"
	seoapi "github.com/zaidmukaddam/dug/api/seo"
	srcapi "github.com/zaidmukaddam/dug/api/src"
	tlsapi "github.com/zaidmukaddam/dug/api/tls"
	vsapi "github.com/zaidmukaddam/dug/api/vs"
	"github.com/zaidmukaddam/dug/pkg/commands"
)

const protocolVersion = "2025-11-25"

var byEndpoint = map[string]http.HandlerFunc{
	"/api/addr":      addrapi.Handler,
	"/api/aeo":       aeoapi.Handler,
	"/api/delegate":  delegateapi.Handler,
	"/api/fetch":     fetchapi.Handler,
	"/api/mail":      mailapi.Handler,
	"/api/me":        meapi.Handler,
	"/api/og":        ogapi.Handler,
	"/api/probe":     probeapi.Handler,
	"/api/propagate": propagateapi.Handler,
	"/api/rdap":      rdapapi.Handler,
	"/api/resolve":   resolveapi.Handler,
	"/api/seo":       seoapi.Handler,
	"/api/src":       srcapi.Handler,
	"/api/tls":       tlsapi.Handler,
	"/api/vs":        vsapi.Handler,
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func Handler(w http.ResponseWriter, r *http.Request) {
	if !originAllowed(r) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, MCP-Protocol-Version, Mcp-Session-Id")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")

	switch r.Method {
	case http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
		return
	case http.MethodGet:
		// The transport allows refusing the server-to-client stream. Nothing
		// here is server-initiated, so there is nothing to stream.
		http.Error(w, "this server opens no SSE stream", http.StatusMethodNotAllowed)
		return
	case http.MethodPost:
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if version := r.Header.Get("MCP-Protocol-Version"); version != "" && version != protocolVersion &&
		version != "2025-06-18" && version != "2025-03-26" {
		http.Error(w, "unsupported MCP-Protocol-Version", http.StatusBadRequest)
		return
	}

	var req rpcRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", Error: &rpcError{-32700, "parse error"}})
		return
	}

	// A notification or a response has no id and expects no reply.
	if len(req.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch req.Method {
	case "initialize":
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{
				"name":    "dug",
				"title":   "dug",
				"version": "1",
			},
			"instructions": "Live domain and network diagnostics. Every call is a fresh lookup; " +
				"nothing is stored between calls. Read the verdict line first, then the evidence. " +
				"If a result mentions a degraded upstream, part of the answer is missing and the " +
				"rest still stands.",
		}})

	case "ping":
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}})

	case "tools/list":
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": toolList()}})

	case "tools/call":
		writeRPC(w, callTool(r, req))

	default:
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID,
			Error: &rpcError{-32601, "method not found: " + req.Method}})
	}
}

func toolList() []any {
	tools := make([]any, 0, len(commands.List))
	for _, spec := range commands.List {
		properties := map[string]any{}
		required := []any{}

		if about := spec.TargetAbout(); about != "" {
			properties["target"] = map[string]any{"type": "string", "description": about}
			required = append(required, "target")
		}
		for _, param := range spec.Params {
			properties[param.Name] = map[string]any{"type": "string", "description": param.About}
			if param.Required {
				required = append(required, param.Name)
			}
		}

		tools = append(tools, map[string]any{
			"name":  "dug_" + strings.ToLower(spec.Name),
			"title": spec.Name,
			"description": spec.Summary + ". Runs live and returns the answer as a sentence " +
				"followed by the evidence. Example: " + spec.Example,
			"inputSchema": map[string]any{
				"type": "object", "properties": properties, "required": required,
			},
			"annotations": map[string]any{
				"readOnlyHint":    true,
				"destructiveHint": false,
				// Every command reaches a third party that this tool does not
				// control, so no result is reproducible from the input alone.
				"openWorldHint": true,
			},
		})
	}
	return tools
}

func callTool(r *http.Request, req rpcRequest) rpcResponse {
	var params struct {
		Name string `json:"name"`
		// Not map[string]string: a model sends a count as 4 as often as "4",
		// and decoding into strings fails the whole call on the first one.
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{-32602, "invalid params"}}
	}

	spec, ok := commands.ByName(strings.ToUpper(strings.TrimPrefix(params.Name, "dug_")))
	if !ok {
		return rpcResponse{JSONRPC: "2.0", ID: req.ID,
			Error: &rpcError{-32602, "unknown tool: " + params.Name}}
	}

	handler, ok := byEndpoint[spec.Endpoint]
	if !ok {
		return rpcResponse{JSONRPC: "2.0", ID: req.ID,
			Error: &rpcError{-32603, "no handler for " + spec.Endpoint}}
	}

	query, err := toolQuery(spec, params.Arguments)
	if err != nil {
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{-32602, err.Error()}}
	}

	proxied := httptest.NewRequest(http.MethodGet, spec.Endpoint+"?"+query.Encode(), nil)
	recorder := httptest.NewRecorder()
	handler(recorder, proxied.WithContext(r.Context()))

	text := recorder.Body.String()
	if strings.TrimSpace(text) == "" {
		text = "the command produced no output"
	}

	return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
		"content": []any{map[string]any{"type": "text", "text": text}},
		"isError": recorder.Code != http.StatusOK,
	}}
}

// toolQuery is the whole of what a tool call may say to a handler.
//
// Only the names the invoked tool declares are read. An MCP client allowlists
// individual tools, so an argument the tool never offered must not reach the
// dispatcher: before this, a call to the ICMP ping carrying command=PORTS ran
// a port scan. command and format are also written after the arguments, so no
// ordering leaves either of them for a caller to set.
func toolQuery(spec commands.Spec, arguments map[string]any) (url.Values, error) {
	declared := []string{}
	if spec.TargetAbout() != "" {
		declared = append(declared, "target")
	}
	for _, param := range spec.Params {
		declared = append(declared, param.Name)
	}

	query := url.Values{}
	for _, name := range declared {
		value, present := arguments[name]
		if !present {
			continue
		}
		text, scalar := asText(value)
		if !scalar {
			return nil, errors.New(name + " must be a string, a number or a boolean")
		}
		if text != "" {
			query.Set(name, text)
		}
	}

	// Text, not JSON. It is the same answer either way, and the rendered form
	// is what an agent can actually read; the JSON envelope stays available
	// over HTTP for callers that want to walk the blocks.
	query.Set("command", spec.Name)
	query.Set("format", "text")
	return query, nil
}

// Every argument arrives as a JSON scalar and leaves as a query value. A
// number decodes as float64, so a count of 4 has to render as "4" rather than
// "4.000000"; an object or an array is not an argument this grammar has.
func asText(value any) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return "", true
	case string:
		return typed, true
	case bool:
		return strconv.FormatBool(typed), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	}
	return "", false
}

// originAllowed implements the transport's DNS-rebinding protection.
//
// Rebinding works by pointing a name the attacker owns at 127.0.0.1, so during
// the attack the browser sends that name as both Origin and Host. Neither can
// be trusted, and comparing them to each other proves nothing at all: they
// agree, because they are the same attacker-supplied string.
//
// Two things the request cannot influence decide it instead. Whether this is
// the public copy comes from the platform: Vercel sets VERCEL on its functions,
// and there the server is public, read-only, keyless and already refuses every
// private destination, so any origin is welcome. Everywhere else the server is
// reachable at a loopback address, which is the shape rebinding aims at, and
// only a page actually served from loopback may drive it. The attacker's page
// is served from the attacker's own name, so it is refused whatever Host it
// claims.
func originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if os.Getenv("VERCEL") != "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return isLoopbackHost(parsed.Hostname())
}

// isLoopbackHost is deliberately strict: "null", an empty host and any name
// that merely resolves to loopback today are all refused, because a name is
// exactly what an attacker rebinds.
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.Unmap().IsLoopback()
}

func writeRPC(w http.ResponseWriter, response rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}
