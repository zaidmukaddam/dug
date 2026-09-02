package screen

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// writePayload is the single exit for every representation: the
// Cache-Control it picks decides whether a shared cache in front of this
// can reuse the answer for the next caller.
func TestEnvelopeCacheControl(t *testing.T) {
	for _, test := range []struct {
		name         string
		status       int
		cacheControl string
		want         string
	}{
		{name: "200 with no override derives from ttl", status: http.StatusOK, want: "public, s-maxage=60, stale-while-revalidate=120"},
		{name: "400 with no override is no-store", status: http.StatusBadRequest, want: "no-store"},
		{name: "explicit cache control wins even on 200", status: http.StatusOK, cacheControl: "private, no-store", want: "private, no-store"},
		{name: "explicit cache control wins even on 400", status: http.StatusBadRequest, cacheControl: "private, no-store", want: "private, no-store"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/tls/example.com?format=json", nil)
			payload := Payload{Command: "TLS", Target: "example.com", TTL: 60}

			writePayload(recorder, request, payload, test.status, test.cacheControl)

			if got := recorder.Header().Get("Cache-Control"); got != test.want {
				t.Errorf("Cache-Control = %q, want %q", got, test.want)
			}
		})
	}
}

// A response without Vary would let a shared cache serve one client's
// representation (json vs text) to the next, and X-API-Version is the only
// thing a caller has to detect a shape change.
func TestEnvelopeHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/tls/example.com?format=json", nil)
	payload := Payload{Command: "TLS", Target: "example.com", TTL: 60}

	writePayload(recorder, request, payload, http.StatusOK, "")

	if vary := recorder.Header().Get("Vary"); !strings.Contains(vary, "Accept") || !strings.Contains(vary, "User-Agent") {
		t.Errorf("Vary = %q, want it to contain both Accept and User-Agent", vary)
	}
	if version := recorder.Header().Get("X-API-Version"); version != APIVersion {
		t.Errorf("X-API-Version = %q, want %q", version, APIVersion)
	}
}

// The text form has to be negotiated through the same path as everything
// else, or curl gets a content type its body does not match.
func TestEnvelopeTextContentType(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/tls/example.com?format=text", nil)
	payload := Payload{Command: "TLS", Target: "example.com", TTL: 60}

	writePayload(recorder, request, payload, http.StatusOK, "")

	want := "text/plain; charset=utf-8"
	if got := recorder.Header().Get("Content-Type"); got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
}

// HoldTTL folds every upstream's own freshness into one number: the
// response can only be as fresh as its shortest-lived component, floored
// and capped so neither a bad upstream nor a missing one breaks the cache.
func TestHoldTTL(t *testing.T) {
	t.Run("below floor is raised to the floor", func(t *testing.T) {
		r := New("TLS", "x")
		r.HoldTTL(1, "tls")
		if got := r.Payload().TTL; got != TTLFloor {
			t.Errorf("ttl = %d, want floor %d", got, TTLFloor)
		}
	})

	t.Run("above cap is lowered to the cap", func(t *testing.T) {
		r := New("TLS", "x")
		r.HoldTTL(999999, "tls")
		want := ttlCaps["tls"]
		if got := r.Payload().TTL; got != want {
			t.Errorf("ttl = %d, want cap %d", got, want)
		}
	})

	t.Run("unknown kind with seconds<=0 uses the default cap", func(t *testing.T) {
		r := New("TLS", "x")
		r.HoldTTL(0, "not-a-real-kind")
		if got := r.Payload().TTL; got != 3600 {
			t.Errorf("ttl = %d, want default cap 3600", got)
		}
	})

	t.Run("a second lower call lowers the held ttl", func(t *testing.T) {
		r := New("TLS", "x")
		r.HoldTTL(3000, "tls")
		r.HoldTTL(300, "tls")
		if got := r.Payload().TTL; got != 300 {
			t.Errorf("ttl = %d, want 300", got)
		}
	})

	t.Run("a second higher call does not raise the held ttl", func(t *testing.T) {
		r := New("TLS", "x")
		r.HoldTTL(300, "tls")
		r.HoldTTL(3000, "tls")
		if got := r.Payload().TTL; got != 300 {
			t.Errorf("ttl = %d, want 300 (unchanged)", got)
		}
	})
}
