// The MCP server manifest, served at /server.json.
//
// This is the shape the MCP registry publishes and clients read: it names the
// server, its version, and the transport a client should connect over. Until
// now the only way to learn that dug speaks MCP was to read llms.txt or the
// developers page, both of which are prose. A client that wants to add a
// remote server has one file to fetch and one URL to take from it.
//
// Validated against the published schema for the 2025-12-11 registry format,
// which is the newest that resolves. Note the constraints it imposes and that
// this file is written to satisfy: `name` is reverse-DNS with exactly one
// slash, and `description` is capped at 100 characters. pkg/wiring asserts
// both rather than trusting the prose here.
//
// Deliberately not published alongside it: a /.well-known/mcp/server-card.json.
// That shape is SEP-1649, still an open proposal, and the schema URL the
// proposal's examples cite does not resolve. A document that claims conformance
// to a schema nobody can fetch is not a discovery aid.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/zaidmukaddam/dug/pkg/screen"
)

type object = map[string]any

// The registry format this document is written against. Pinned rather than
// tracking a "latest" alias, because there is no such alias: every published
// schema is dated, and a client validating this file resolves exactly this URL.
const schemaURL = "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json"

// Reverse-DNS of the domain that serves this, which is the namespace ownership
// of dug.sh can actually be proved for. The schema requires exactly one slash.
const serverName = "sh.dug/dug"

// At most 100 characters: the schema caps it, and a longer string makes the
// document invalid rather than merely verbose.
const serverDescription = "Live domain and network diagnostics: dns, tls, mail, rdap and routing, one tool per command."

func Handler(w http.ResponseWriter, r *http.Request) {
	base := screen.Origin(r)

	doc := object{
		"$schema":     schemaURL,
		"name":        serverName,
		"title":       "dug",
		"description": serverDescription,
		// Semantic versioning, tied to the API contract version so the manifest
		// cannot claim a generation the responses do not.
		"version":    screen.APIVersion + ".0.0",
		"websiteUrl": base,
		"repository": object{
			"source": "github",
			"url":    "https://github.com/zaidmukaddam/dug",
		},
		"remotes": []any{
			object{
				"type": "streamable-http",
				// The short path, which is the one a client will try first and
				// the one the AI catalog and llms.txt both name. It rewrites to
				// /api/mcp, and both spellings answer.
				"url": base + "/mcp",
			},
		},
	}

	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		http.Error(w, "encoding failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// A browser-side agent discovering this cross-origin is the case the file
	// exists for, and without this header it cannot read the response at all.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, s-maxage=86400, stale-while-revalidate=172800")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}
