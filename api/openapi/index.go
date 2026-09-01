// OpenAPI 3.1, generated from internal/commands.
//
// Written as an operation per pretty path rather than one operation with a
// `command` enum, because a tool picker reads one operation per capability and
// a single overloaded endpoint reads as one capability.
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
	paths := object{}
	for _, spec := range commands.List {
		paths[spec.Path] = object{"get": operation(spec)}
	}

	doc := object{
		"openapi": "3.1.0",
		"info": object{
			"title":   "dug",
			"version": "1.0.0",
			"summary": "Live domain and network diagnostics",
			"description": "Every answer is a fresh lookup. Nothing is precomputed and nothing " +
				"is stored between requests. Reads are open: no key, no signup. " +
				"An upstream failure returns 200 with the failure named in `degraded` " +
				"and the rest of the answer intact; arguments that are wrong return 400 " +
				"with `error.code` set.\n\n" +
				"Versioning: " + screen.VersionPolicy,
			"license": object{"name": "MIT", "identifier": "MIT"},
		},
		"servers": []any{object{"url": origin(r)}},
		"paths":   paths,
		"components": object{"schemas": object{
			"Payload":   payloadSchema(),
			"Error":     errorSchema(),
			"ErrorInfo": errorInfoSchema(),
		}},
	}

	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		http.Error(w, "encoding failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, s-maxage=86400, stale-while-revalidate=172800")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func operation(spec commands.Spec) object {
	parameters := []any{}

	// Path parameters first, in the order they appear in the template.
	for _, name := range pathParams(spec.Path) {
		about := spec.TargetAbout()
		example := firstWordAfter(spec.Example)
		if name != "target" {
			about, example = paramAbout(spec, name)
		}
		parameters = append(parameters, object{
			"name": name, "in": "path", "required": true,
			"description": about,
			"schema":      object{"type": "string"},
			"example":     example,
		})
	}

	for _, param := range spec.Params {
		if strings.Contains(spec.Path, "{"+param.Name+"}") {
			continue
		}
		parameters = append(parameters, object{
			"name": param.Name, "in": "query", "required": param.Required,
			"description": param.About,
			"schema":      object{"type": "string"},
			"example":     param.Example,
		})
	}

	parameters = append(parameters, object{
		"name": "format", "in": "query", "required": false,
		"description": "Force a representation. Without it, text for terminal clients and JSON for everything else.",
		"schema":      object{"type": "string", "enum": []any{"text", "json"}},
	})

	// The version a caller is willing to accept. Optional, because there is one
	// version and omitting it means "whatever is current"; sending a version
	// this surface does not serve is a 400 rather than a silent mismatch. Every
	// response echoes the version it was produced under.
	parameters = append(parameters, object{
		"name": "X-API-Version", "in": "header", "required": false,
		"description": "Pin the API version. Only " + screen.APIVersion + " exists; any other " +
			"value is refused with error.code unsupported_version. Every response carries " +
			"X-API-Version naming the contract it was produced under.",
		"schema": object{"type": "string", "enum": []any{screen.APIVersion}},
	})

	return object{
		"operationId": strings.ToLower(spec.Name),
		"summary":     spec.Summary,
		"description": spec.Summary + ". Example: " + spec.Example,
		"tags":        []any{spec.Family},
		"parameters":  parameters,
		"responses": object{
			"200": object{
				"description": "The answer, and the evidence for it. An upstream that failed " +
					"is still a 200: the failure is named in `degraded` and the rest of the " +
					"answer stands. Check `degraded` before treating a result as complete.",
				"content": object{
					"application/json": object{"schema": object{"$ref": "#/components/schemas/Payload"}},
					"text/plain":       object{"schema": object{"type": "string"}},
				},
			},
			"400": object{
				"description": "The arguments were wrong, so nothing was looked up. The body is " +
					"a Payload whose `error` is set; branch on `error.code`.",
				"content": object{
					"application/json": object{"schema": object{"$ref": "#/components/schemas/Error"}},
					"text/plain":       object{"schema": object{"type": "string"}},
				},
			},
		},
	}
}

// errorSchema is a Payload that is guaranteed to carry `error`. Kept as a
// narrowing of Payload rather than a separate shape, because a refusal still
// renders as a screen and a caller that already parses Payload should not need
// a second parser for the failure case.
func errorSchema() object {
	return object{
		"allOf": []any{
			object{"$ref": "#/components/schemas/Payload"},
			object{
				"type":     "object",
				"required": []any{"error"},
				"properties": object{
					"error": object{"$ref": "#/components/schemas/ErrorInfo"},
				},
			},
		},
	}
}

func errorInfoSchema() object {
	return object{
		"type":        "object",
		"description": "Set only on a 4xx. Absent from every successful answer.",
		"required":    []any{"code", "message"},
		"properties": object{
			"code": object{
				"type":        "string",
				"description": "Closed set. Branch on this rather than on the message.",
				"enum": []any{
					screen.CodeMissingArgument, screen.CodeInvalidArgument,
					screen.CodeUnsupportedVersion, screen.CodeUnknownEndpoint,
				},
			},
			"message": object{
				"type":        "string",
				"description": "The refusal in one sentence, the same text as verdict.headline.",
			},
			"hint": object{
				"type":        "string",
				"description": "What a corrected call looks like.",
			},
		},
	}
}

func payloadSchema() object {
	return object{
		"type": "object",
		"required": []any{"command", "target", "verdict", "blocks", "notes",
			"degraded", "ts", "ttl", "elapsed_ms", "upstream_queries"},
		"properties": object{
			"command": object{"type": "string", "description": "The verb that ran."},
			"target":  object{"type": "string", "description": "What it ran against, normalised."},
			"verdict": object{
				"type":        "object",
				"description": "The answer in one sentence. Read this first.",
				"required":    []any{"state", "headline", "detail"},
				"properties": object{
					"state":    object{"type": "string", "enum": []any{"ok", "warn", "none"}},
					"headline": object{"type": "string"},
					"detail":   object{"type": "string"},
				},
			},
			"blocks": object{
				"type":        "array",
				"description": "The evidence. Each block names a display component and carries its props.",
				"items": object{
					"type":     "object",
					"required": []any{"component", "props", "span"},
					"properties": object{
						"component": object{"type": "string"},
						"props":     object{"type": "object"},
						"span":      object{"type": "integer"},
					},
				},
			},
			"notes": object{"type": "array", "items": object{"type": "string"},
				"description": "Provenance and limits."},
			"degraded": object{
				"type":        "array",
				"description": "Upstreams that failed while the rest of the answer stood. Check before treating a result as complete.",
				"items": object{
					"type":       "object",
					"required":   []any{"source", "reason"},
					"properties": object{"source": object{"type": "string"}, "reason": object{"type": "string"}},
				},
			},
			"ts":               object{"type": "integer", "description": "When the answer was produced, unix milliseconds."},
			"ttl":              object{"type": "integer", "description": "Seconds this answer stays valid."},
			"elapsed_ms":       object{"type": "integer"},
			"upstream_queries": object{"type": "integer", "description": "How many lookups it cost."},
		},
	}
}

func pathParams(path string) []string {
	out := []string{}
	for _, segment := range strings.Split(path, "/") {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			out = append(out, strings.Trim(segment, "{}"))
		}
	}
	return out
}

func paramAbout(spec commands.Spec, name string) (string, string) {
	for _, param := range spec.Params {
		if param.Name == name {
			return param.About, param.Example
		}
	}
	return "", ""
}

// firstWordAfter pulls the argument out of a command's own example, so the
// OpenAPI example and the grammar example can never disagree.
func firstWordAfter(example string) string {
	fields := strings.Fields(example)
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}

func origin(r *http.Request) string {
	scheme := "https"
	if isLocal(r.Host) {
		scheme = "http"
	}
	// Vercel terminates TLS ahead of the function, so the scheme only survives
	// in this header. Constrained to the two known values: it is client-set
	// text, and it lands in a document other tools read as authoritative.
	if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded == "http" || forwarded == "https" {
		scheme = forwarded
	}
	return scheme + "://" + r.Host
}

func isLocal(host string) bool {
	return strings.HasPrefix(host, "127.0.0.1") || strings.HasPrefix(host, "localhost")
}
