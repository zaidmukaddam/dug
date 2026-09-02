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

	body, err := json.MarshalIndent(mcpx.Card(screen.Origin(r)), "", "  ")
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
