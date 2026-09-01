package screen

import (
	"net/http"
	"strings"
)

// Origin is the absolute base every generated document writes its links
// against.
//
// Four handlers now emit documents that name other routes by absolute URL —
// the OpenAPI servers list, the RFC 9727 catalog, the MCP server manifest and
// the AI catalog. Each had, or would have had, its own copy of this. One copy
// means a preview deployment cannot describe itself as production in one
// document and correctly in the next.
func Origin(r *http.Request) string {
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
