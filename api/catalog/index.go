// The API catalog, RFC 9727, served at /.well-known/api-catalog.
//
// A caller that has the origin and nothing else needs one predictable place to
// find out what is here. RFC 9727 defines exactly that place, and the format is
// an RFC 9264 linkset: a list of anchors, each with typed links hanging off it.
//
// Everything it points at already exists. This adds a discovery step, not a
// surface, and the links are built from the same command registry so it cannot
// name a route that is not served.
package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/zaidmukaddam/dug/pkg/commands"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

type object = map[string]any

func Handler(w http.ResponseWriter, r *http.Request) {
	base := screen.Origin(r)

	// The origin itself, described by the three documents that describe the
	// whole surface. rel values are the registered ones: service-desc for the
	// machine description, service-doc for the human one, describedby for the
	// rest.
	links := []any{
		object{
			"anchor": base,
			"service-desc": []any{
				object{"href": base + "/openapi.json", "type": "application/json"},
			},
			"service-doc": []any{
				object{"href": base + "/developers", "type": "text/html"},
			},
			// Three documents describe this origin rather than one. llms.txt is
			// the prose, and the AI catalog is the same surface typed by
			// protocol, which is the form an agent runtime reads before it
			// knows whether it can connect at all.
			"describedby": []any{
				object{"href": base + "/llms.txt", "type": "text/plain"},
				object{
					"href":  base + "/.well-known/ai-catalog.json",
					"type":  "application/ai-catalog+json",
					"title": "AI catalog: the MCP and REST surfaces, typed by protocol",
				},
			},
			"status": []any{
				object{"href": base + "/src", "type": "text/plain"},
			},
		},
		// The MCP server gets its own anchor rather than a link on the origin.
		// It is a distinct protocol at a distinct endpoint, and an agent that
		// found no tools on the page needs to be told where they actually are.
		//
		// Anchored at /mcp, the path a client tries first. /api/mcp is the same
		// endpoint and still answers.
		object{
			"anchor": base + "/mcp",
			"describedby": []any{
				object{
					"href":  base + "/.well-known/mcp/server-card.json",
					"type":  "application/json",
					"title": "MCP server card: protocol version, transport, capabilities and every tool",
				},
				object{
					"href":  base + "/server.json",
					"type":  "application/json",
					"title": "MCP server manifest: name, version and transport",
				},
				object{
					"href":  base + "/llms.txt",
					"type":  "text/plain",
					"title": "MCP over Streamable HTTP, one tool per command, stateless",
				},
			},
			"service-doc": []any{
				object{"href": base + "/developers", "type": "text/html"},
			},
		},
	}

	// One anchor per command, so a caller can walk the catalog to the exact
	// operation rather than parsing the whole OpenAPI document to find it.
	for _, spec := range commands.List {
		links = append(links, object{
			"anchor": base + spec.Path,
			"service-desc": []any{
				object{"href": base + "/openapi.json", "type": "application/json"},
			},
			"describedby": []any{
				object{
					"href":  base + "/openapi.json#/paths/" + escapePointer(spec.Path),
					"type":  "application/json",
					"title": spec.Summary,
				},
			},
		})
	}

	body, err := json.MarshalIndent(object{"linkset": links}, "", "  ")
	if err != nil {
		http.Error(w, "encoding failed", http.StatusInternalServerError)
		return
	}

	// The media type RFC 9264 defines for a JSON linkset. Not application/json:
	// a client content-negotiating for a linkset is asking for this shape.
	w.Header().Set("Content-Type", "application/linkset+json")
	// A well-known document is read by clients that are not this origin. That
	// is the whole point of fixing its path, and it does not work from a
	// browser without this.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, s-maxage=86400, stale-while-revalidate=172800")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// A JSON pointer escapes / as ~1 and ~ as ~0, in that order. Without it every
// path fragment here would break at its first slash.
func escapePointer(path string) string {
	return strings.ReplaceAll(strings.ReplaceAll(path, "~", "~0"), "/", "~1")
}
