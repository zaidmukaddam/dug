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
