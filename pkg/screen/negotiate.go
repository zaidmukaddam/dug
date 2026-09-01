package screen

// Which representation a request wants.
//
// Three ways to ask, in order of authority: an explicit ?format=, an Accept
// header, and finally the user agent. The user agent check exists so that a
// bare `curl dug.sh/tls/example.com` is readable, which is the whole point
// of the text form; it is the weakest signal and the easiest to override.
//
// Every response carries Vary: Accept, User-Agent. Without it a shared cache in
// front of this would serve one client's representation to another, and the
// Cache-Control here is deliberately public.

import (
	"net/http"
	"strings"
)

// textAgents are the clients that get text without asking. Matched as a
// prefix of the user agent, which is where these all put their name.
var textAgents = []string{"curl/", "wget/", "httpie/", "http/", "xh/", "fetch/", "powershell/", "lwp::"}

func WantsText(r *http.Request) bool {
	if r == nil {
		return false
	}

	switch strings.ToLower(r.URL.Query().Get("format")) {
	case "text", "txt", "plain":
		return true
	case "json":
		return false
	}

	// An explicit Accept wins over the user agent, in both directions: a
	// browser asking for text/plain gets text, and curl asking for JSON gets
	// JSON without having to also set a user agent.
	accept := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(accept, "application/json") {
		return false
	}
	if strings.Contains(accept, "text/plain") {
		return true
	}
	// text/html means a browser, whatever it claims to be.
	if strings.Contains(accept, "text/html") {
		return false
	}

	agent := strings.ToLower(strings.TrimSpace(r.Header.Get("User-Agent")))
	for _, prefix := range textAgents {
		if strings.HasPrefix(agent, prefix) {
			return true
		}
	}
	return false
}
