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
)

type object = map[string]any

func Handler(w http.ResponseWriter, r *http.Request) {
	base := origin(r)

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
			"describedby": []any{
				object{"href": base + "/llms.txt", "type": "text/plain"},
			},
			"status": []any{
				object{"href": base + "/src", "type": "text/plain"},
			},
		},
		// The MCP server gets its own anchor rather than a link on the origin.
		// It is a distinct protocol at a distinct endpoint, and an agent that
		// found no tools on the page needs to be told where they actually are.
		object{
			"anchor": base + "/api/mcp",
			"describedby": []any{
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
	w.Header().Set("Cache-Control", "public, s-maxage=86400, stale-while-revalidate=172800")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// A JSON pointer escapes / as ~1 and ~ as ~0, in that order. Without it every
// path fragment here would break at its first slash.
func escapePointer(path string) string {
	return strings.ReplaceAll(strings.ReplaceAll(path, "~", "~0"), "/", "~1")
}

func origin(r *http.Request) string {
	scheme := "https"
	if strings.HasPrefix(r.Host, "127.0.0.1") || strings.HasPrefix(r.Host, "localhost") {
		scheme = "http"
	}
	if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded == "http" || forwarded == "https" {
		scheme = forwarded
	}
	return scheme + "://" + r.Host
}
