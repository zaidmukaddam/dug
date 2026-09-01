// The MCP server card, SEP-1649, served at /.well-known/mcp/server-card.json.
//
// What a client would have learned from initialize plus tools/list, without the
// handshake, so it can decide whether to connect and validate every tool
// description before it does. Fields come from pkg/mcpx, the same source
// api/mcp answers from: a card that disagrees with its server is worse than no
// card.
//
// No $schema, which the SEP marks required. The url it names has never been
// published, so every card in the wild cites a 404; a broken pointer is worse
// than an absent one. The rest follows the SEP.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/zaidmukaddam/dug/pkg/mcpx"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

type object = map[string]any

func Handler(w http.ResponseWriter, r *http.Request) {
	// Required by the SEP: a browser client reading a card cross-origin is the
	// case it exists for.
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
		// Stated rather than omitted: "none needed" is a fact a client wants
		// before connecting, and an absent field only says nobody wrote one down.
		"authentication": object{
			"required": false,
			"schemes":  []any{},
		},
		"instructions": mcpx.Instructions,
		// Static rather than the reserved string "dynamic": the set is fixed at
		// build time, so charging a client a tools/list round trip to learn it
		// would be the cost this document exists to remove.
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
