// Local router for the api/ handlers. Development only: on Vercel each
// api/<name>/index.go is its own function and the platform does the routing.
package main

import (
	"log"
	"net/http"
	"os"

	addrapi "github.com/zaidmukaddam/dug/api/addr"
	aeoapi "github.com/zaidmukaddam/dug/api/aeo"
	aicatalogapi "github.com/zaidmukaddam/dug/api/aicatalog"
	catalogapi "github.com/zaidmukaddam/dug/api/catalog"
	delegateapi "github.com/zaidmukaddam/dug/api/delegate"
	fetchapi "github.com/zaidmukaddam/dug/api/fetch"
	guardapi "github.com/zaidmukaddam/dug/api/guard"
	llmsapi "github.com/zaidmukaddam/dug/api/llms"
	mailapi "github.com/zaidmukaddam/dug/api/mail"
	mcpapi "github.com/zaidmukaddam/dug/api/mcp"
	meapi "github.com/zaidmukaddam/dug/api/me"
	ogapi "github.com/zaidmukaddam/dug/api/og"
	openapiapi "github.com/zaidmukaddam/dug/api/openapi"
	probeapi "github.com/zaidmukaddam/dug/api/probe"
	propagateapi "github.com/zaidmukaddam/dug/api/propagate"
	rdapapi "github.com/zaidmukaddam/dug/api/rdap"
	resolveapi "github.com/zaidmukaddam/dug/api/resolve"
	seoapi "github.com/zaidmukaddam/dug/api/seo"
	serverapi "github.com/zaidmukaddam/dug/api/server"
	servercardapi "github.com/zaidmukaddam/dug/api/servercard"
	srcapi "github.com/zaidmukaddam/dug/api/src"
	tlsapi "github.com/zaidmukaddam/dug/api/tls"
	vsapi "github.com/zaidmukaddam/dug/api/vs"
	webmcpapi "github.com/zaidmukaddam/dug/api/webmcp"
)

func main() {
	port := os.Getenv("DUG_API_PORT")
	if port == "" {
		port = "8787"
	}

	routes := map[string]http.HandlerFunc{
		"/api/addr":       addrapi.Handler,
		"/api/aeo":        aeoapi.Handler,
		"/api/aicatalog":  aicatalogapi.Handler,
		"/api/catalog":    catalogapi.Handler,
		"/api/delegate":   delegateapi.Handler,
		"/api/fetch":      fetchapi.Handler,
		"/api/guard":      guardapi.Handler,
		"/api/llms":       llmsapi.Handler,
		"/api/mail":       mailapi.Handler,
		"/api/mcp":        mcpapi.Handler,
		"/api/me":         meapi.Handler,
		"/api/og":         ogapi.Handler,
		"/api/openapi":    openapiapi.Handler,
		"/api/probe":      probeapi.Handler,
		"/api/propagate":  propagateapi.Handler,
		"/api/rdap":       rdapapi.Handler,
		"/api/resolve":    resolveapi.Handler,
		"/api/seo":        seoapi.Handler,
		"/api/server":     serverapi.Handler,
		"/api/servercard": servercardapi.Handler,
		"/api/src":        srcapi.Handler,
		"/api/tls":        tlsapi.Handler,
		"/api/vs":         vsapi.Handler,
		"/api/webmcp":     webmcpapi.Handler,
	}

	mux := http.NewServeMux()
	for path, handler := range routes {
		mux.HandleFunc(path, handler)
	}

	log.Printf("api on http://127.0.0.1:%s with %d routes", port, len(routes))
	if err := http.ListenAndServe("127.0.0.1:"+port, mux); err != nil {
		log.Fatalf("api could not listen on %s (set DUG_API_PORT to move it): %v", port, err)
	}
}
