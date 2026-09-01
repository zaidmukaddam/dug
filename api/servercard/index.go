// The MCP server card, served at /.well-known/mcp/server-card.json.
//
// SEP-1649. What a client would have learned from initialize plus tools/list,
// without the handshake: the protocol version, the transport and its endpoint,
// the capabilities, whether authentication is needed, and the full static tool
// list. A client can decide whether this server is worth connecting to, and
// validate every tool description against its own classifiers, before it opens
// a single connection.
//
// Every field here comes from pkg/mcpx, which is the same source api/mcp
// answers initialize and tools/list from. That is the whole design constraint:
// a card that disagrees with its server is worse than no card, because a client
// that trusted it connects expecting something else.
//
// One deliberate departure. The SEP lists `$schema` as required and points it
// at https://static.modelcontextprotocol.io/schemas/mcp-server-card/v1.json,
// which does not resolve — the proposal was closed without the schema ever
// being published. Every other server card in the wild cites a 404 as a result.
// Omitting the field is the smaller problem: a broken pointer tells a validator
// to fetch something that is not there, while its absence costs a reader
// nothing they could have used. Everything else follows the SEP exactly.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/zaidmukaddam/dug/pkg/mcpx"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

type object = map[string]any

func Handler(w http.ResponseWriter, r *http.Request) {
	// The SEP requires CORS on the discovery endpoint, because a browser-based
	// client reading a card cross-origin is the case it is for.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	card := object{
		// The card document's own schema version, not the server's.
		"version":         "1.0",
		"protocolVersion": mcpx.ProtocolVersion,
		"serverInfo": object{
			"name":    mcpx.ServerName,
			"title":   mcpx.ServerTitle,
			"version": mcpx.ServerVersion,
		},
		"description": "Live domain and network diagnostics: dns, tls, mail, rdap, addressing and " +
			"routing. Every call is a fresh lookup and nothing is stored between calls.",
		"documentationUrl": screen.Origin(r) + "/developers",
		"transport": object{
			"type": "streamable-http",
			// A path rather than a url, which is what the SEP asks for. /api/mcp
			// is the same endpoint and also answers.
			"endpoint": "/mcp",
		},
		"capabilities": mcpx.Capabilities(),
		// Stated rather than omitted. "No authentication" is a fact a client
		// wants before connecting, and an absent field only says nobody wrote
		// one down.
		"authentication": object{
			"required": false,
			"schemes":  []any{},
		},
		"instructions": mcpx.Instructions,
		// A static list rather than the reserved string "dynamic": the set is
		// fixed at build time and cannot change under a connected client, so
		// making a client spend a tools/list round trip to learn it would be
		// the exact cost this document exists to remove.
		"tools": mcpx.Tools(),
	}

	body, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		http.Error(w, "encoding failed", http.StatusInternalServerError)
		return
	}

	// The SEP requires application/json specifically.
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, s-maxage=86400, stale-while-revalidate=172800")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}
