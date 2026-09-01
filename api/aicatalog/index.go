// The AI catalog, served at /.well-known/ai-catalog.json.
//
// Two discovery documents already sit at fixed paths here: the RFC 9727 API
// catalog, which is about HTTP resources, and llms.txt, which is prose. Neither
// answers the question an agent runtime actually asks first — "does this domain
// offer anything I can connect to, and in which protocol" — because neither is
// typed by protocol. The AI Catalog format is, and it is the format the MCP and
// A2A communities are converging on for exactly this.
//
// Written against the AI Catalog 1.0 shape and the ARD entry rules: every entry
// carries identifier, displayName, type, and exactly one of url or data. The
// identifier is the domain-anchored urn:air:<publisher>:<namespace>:<name>.
//
// Both entries point at documents this deployment actually serves, and the
// capability list is generated from the command registry, so this file cannot
// advertise a tool that does not exist. There is no `host` block: the spec's
// example identifies the host by did:web, and dug publishes no DID, so the
// field would be a claim rather than a fact.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/zaidmukaddam/dug/pkg/mcpx"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

type object = map[string]any

func Handler(w http.ResponseWriter, r *http.Request) {
	base := screen.Origin(r)

	doc := object{
		"specVersion": "1.0",
		"entries": []any{
			// The MCP server, pointed at the actual server card now that one is
			// served. This entry named /server.json while the card did not
			// exist, which meant the media type here described a shape the
			// document at the other end was not.
			object{
				"identifier":  "urn:air:dug.sh:mcp:dug",
				"displayName": "dug",
				"description": "Live domain and network diagnostics over MCP. Every call is a fresh " +
					"lookup against dns, rdap, tls and routing upstreams; nothing is stored between calls.",
				"type":         "application/mcp-server-card+json",
				"url":          base + "/.well-known/mcp/server-card.json",
				"capabilities": mcpx.ToolNames(),
				"representativeQueries": []any{
					"when does the tls certificate for github.com expire",
					"has my dns change propagated yet",
					"why is mail from my domain going to spam",
					"who owns example.com and when does the registration lapse",
					"which asn does this ip address belong to",
				},
			},
			// The same capabilities as plain HTTP, for a caller that speaks
			// OpenAPI rather than MCP. Not a second product: one surface, two
			// ways in, and saying so here keeps a client from concluding the
			// REST API is a different service.
			object{
				"identifier":  "urn:air:dug.sh:api:dug",
				"displayName": "dug REST API",
				"description": "The same diagnostics as a plain GET per command. No key and no signup; " +
					"answers in text, json or markdown by content negotiation.",
				"type":         "application/vnd.oai.openapi+json;version=3.1",
				"url":          base + "/openapi.json",
				"capabilities": mcpx.ToolNames(),
				"representativeQueries": []any{
					"curl the mx records for a domain",
					"check a tls certificate chain from a script",
					"look up the network and asn for an address",
				},
			},
		},
	}

	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		http.Error(w, "encoding failed", http.StatusInternalServerError)
		return
	}

	// The media type the AI Catalog specification names for a catalog document.
	w.Header().Set("Content-Type", "application/ai-catalog+json; charset=utf-8")
	// The document exists to be read by an agent that is somewhere else,
	// including one running in a page on another origin.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, s-maxage=86400, stale-while-revalidate=172800")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}
