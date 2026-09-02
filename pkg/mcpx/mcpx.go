// The MCP server's identity and tool list, in one place.
//
// Two documents describe the same server: the initialize result, and the server
// card that exists to give a client that same answer without the handshake.
// Neither is written by hand, because a card that disagrees with its server is
// worse than no card.
package mcpx

import (
	"strings"

	"github.com/zaidmukaddam/dug/pkg/commands"
)

// The MCP protocol version this server speaks. The card publishes it so a
// client knows before connecting whether it can talk to this at all.
const ProtocolVersion = "2025-11-25"

const (
	ServerName    = "dug"
	ServerTitle   = "dug"
	ServerVersion = "1"
)

// Instructions is what the server returns from initialize, and what the card
// carries so a client reads the same guidance either way.
const Instructions = "Live domain and network diagnostics. Every call is a fresh lookup; " +
	"nothing is stored between calls. Read the verdict line first, then the evidence. " +
	"If a result mentions a degraded upstream, part of the answer is missing and the " +
	"rest still stands."

// Capabilities is what the server declares. Tools and nothing else, and no
// listChanged: the set is fixed at build time.
func Capabilities() map[string]any {
	return map[string]any{"tools": map[string]any{}}
}

// Card is the server card, SEP-1649: what a client would have learned from
// initialize plus tools/list, without the handshake, so it can decide whether
// to connect and validate every tool description before it does.
//
// The one place the document is built, so the HTTP card and the MCP resource
// that both serve it cannot drift apart from each other.
func Card(origin string) map[string]any {
	return map[string]any{
		// The card document's own schema version, not the server's.
		"version":         "1.0",
		"protocolVersion": ProtocolVersion,
		"serverInfo": map[string]any{
			"name":    ServerName,
			"title":   ServerTitle,
			"version": ServerVersion,
		},
		"description": "Live domain and network diagnostics: dns, tls, mail, rdap, addressing and " +
			"routing. Every call is a fresh lookup and nothing is stored between calls.",
		"documentationUrl": origin + "/developers",
		"transport": map[string]any{
			"type": "streamable-http",
			// A path rather than a url, which is what the SEP asks for. /api/mcp
			// is the same endpoint and also answers.
			"endpoint": "/mcp",
		},
		"capabilities": Capabilities(),
		// Stated rather than omitted: "none needed" is a fact a client wants
		// before connecting, and an absent field only says nobody wrote one down.
		"authentication": map[string]any{
			"required": false,
			"schemes":  []any{},
		},
		"instructions": Instructions,
		// Static rather than the reserved string "dynamic": the set is fixed at
		// build time, so charging a client a tools/list round trip to learn it
		// would be the cost this document exists to remove.
		"tools": Tools(),
	}
}

// ToolName is the one place the prefix lives. It is also how a call is routed
// back to a command, so the two directions cannot disagree about spelling.
func ToolName(spec commands.Spec) string {
	return "dug_" + strings.ToLower(spec.Name)
}

// SpecFor is ToolName in reverse.
func SpecFor(name string) (commands.Spec, bool) {
	return commands.ByName(strings.ToUpper(strings.TrimPrefix(name, "dug_")))
}

// Tools is the MCP tool list, one tool per command. The same value serves
// tools/list and the server card's static `tools` array, so a client that
// validated a description from the card validated what it will be offered.
func Tools() []any {
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
			"name":  ToolName(spec),
			"title": spec.Name,
			"description": spec.Summary + ". Runs live and returns the answer as a sentence " +
				"followed by the evidence. Example: " + spec.Example,
			"inputSchema": map[string]any{
				"type": "object", "properties": properties, "required": required,
			},
			"annotations": map[string]any{
				"readOnlyHint":    true,
				"destructiveHint": false,
				// The answer is built from whatever a third party nameserver,
				// registry or web server returned, which is not content this
				// server vouches for. The page tools say the same.
				"untrustedContentHint": true,
				// Every command reaches a third party that this tool does not
				// control, so no result is reproducible from the input alone.
				"openWorldHint": true,
			},
		})
	}
	return tools
}

// ToolNames is the tool list flattened to names, for documents that index the
// capability set rather than describe it.
func ToolNames() []any {
	names := make([]any, 0, len(commands.List))
	for _, spec := range commands.List {
		names = append(names, ToolName(spec))
	}
	return names
}
