package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zaidmukaddam/dug/pkg/screen"
)

// The leftmost X-Forwarded-For entry is the original client; everything to the
// right of it is infrastructure that appended itself on the way in. Reading the
// wrong end reports the proxy's address as the caller's.
func TestClientAddrPrefersTheOriginalClient(t *testing.T) {
	for _, test := range []struct {
		name       string
		headers    map[string]string
		remote     string
		wantAddr   string
		wantSource string
	}{
		{
			name:       "forwarded chain",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.9, 70.41.3.18, 150.172.238.178"},
			remote:     "10.0.0.1:4000",
			wantAddr:   "203.0.113.9",
			wantSource: "x-forwarded-for",
		},
		{
			name:       "real ip when there is no chain",
			headers:    map[string]string{"X-Real-IP": "198.51.100.7"},
			remote:     "10.0.0.1:4000",
			wantAddr:   "198.51.100.7",
			wantSource: "x-real-ip",
		},
		{
			name:       "socket when there are no headers",
			remote:     "192.0.2.44:51000",
			wantAddr:   "192.0.2.44",
			wantSource: "socket",
		},
		{
			// An ipv4-in-ipv6 wrapper is the same address; reporting the
			// wrapper back would not match what anything else shows.
			name:       "ipv4 in ipv6 is unwrapped",
			headers:    map[string]string{"X-Forwarded-For": "::ffff:203.0.113.9"},
			remote:     "10.0.0.1:4000",
			wantAddr:   "203.0.113.9",
			wantSource: "x-forwarded-for",
		},
		{
			// Garbage in the header must not shadow a usable socket address.
			name:       "unparseable header falls through",
			headers:    map[string]string{"X-Forwarded-For": "not-an-address"},
			remote:     "192.0.2.44:51000",
			wantAddr:   "192.0.2.44",
			wantSource: "socket",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
			for key, value := range test.headers {
				request.Header.Set(key, value)
			}
			if test.remote != "" {
				request.RemoteAddr = test.remote
			}

			address, source := clientAddr(request)
			if address != test.wantAddr {
				t.Errorf("address = %q, want %q", address, test.wantAddr)
			}
			if source != test.wantSource {
				t.Errorf("source = %q, want %q", source, test.wantSource)
			}
		})
	}
}

// The whole reason this command needs its own writer. Every other answer here
// is a fact about the world and is publicly cacheable; this one is true for one
// requester, and a shared cache entry would hand their address to the next
// person who asked.
func TestMeIsNeverPubliclyCached(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/me?format=json", nil)
	request.RemoteAddr = "192.168.1.50:9000"

	Handler(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if store := recorder.Header().Get("Cache-Control"); store != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", store)
	}
}

// A private caller is the local development case, not a fault. The address
// guard refuses private space for a named target on purpose, and surfacing that
// refusal here would read as the tool being broken on localhost.
func TestPrivateCallerIsAnsweredNotRefused(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/me?format=json", nil)
	request.RemoteAddr = "127.0.0.1:9000"

	Handler(recorder, request)

	var payload screen.Payload
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("body is not json: %v", err)
	}
	if payload.Verdict.State != "ok" {
		t.Errorf("state = %q, want ok for a private caller", payload.Verdict.State)
	}
	if !strings.Contains(payload.Verdict.Headline, "127.0.0.1") {
		t.Errorf("headline does not name the address: %q", payload.Verdict.Headline)
	}
	if payload.Target != "127.0.0.1" {
		t.Errorf("target = %q, want 127.0.0.1", payload.Target)
	}
}
