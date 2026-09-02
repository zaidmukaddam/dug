package screen

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The order of authority: ?format= beats Accept, Accept beats the user
// agent. Getting this wrong means a shared cache in front of this serves
// one client's representation to another (see the Vary header this feeds).
func TestWantsText(t *testing.T) {
	for _, test := range []struct {
		name   string
		query  string
		accept string
		agent  string
		want   bool
	}{
		{name: "format text wins over accept json", query: "?format=text", accept: "application/json", want: true},
		{name: "format json wins over curl agent", query: "?format=json", agent: "curl/8.4.0", want: false},
		{name: "accept json means false", accept: "application/json", want: false},
		{name: "accept text/plain means true", accept: "text/plain", want: true},
		{name: "accept text/html beats curl agent", accept: "text/html", agent: "curl/8.4.0", want: false},
		{name: "curl agent with no accept", agent: "curl/8.4.0", want: true},
		{name: "wget agent with no accept", agent: "wget/1.21.4", want: true},
		{name: "httpie agent with no accept", agent: "httpie/3.2.2", want: true},
		{name: "HTTPie http agent with no accept", agent: "http/3.2.2", want: true},
		{name: "xh agent with no accept", agent: "xh/0.19.3", want: true},
		{name: "fetch agent with no accept", agent: "fetch/1.0", want: true},
		{name: "powershell agent with no accept", agent: "powershell/7.4", want: true},
		{name: "lwp agent with no accept", agent: "lwp::useragent/6.72", want: true},
		{name: "browser agent with no accept means false", agent: "Mozilla/5.0 (Macintosh)", want: false},
		{name: "accept is case insensitive", accept: "TEXT/PLAIN", want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/tls/example.com"+test.query, nil)
			if test.accept != "" {
				request.Header.Set("Accept", test.accept)
			}
			if test.agent != "" {
				request.Header.Set("User-Agent", test.agent)
			}
			if got := WantsText(request); got != test.want {
				t.Errorf("WantsText() = %v, want %v", got, test.want)
			}
		})
	}
}
