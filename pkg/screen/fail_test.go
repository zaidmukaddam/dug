package screen

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A rejected argument is a 400 carrying a code a caller can branch on, in both
// representations. Before this it was a 200 whose only signal was English in
// the verdict, so an agent could not tell a refusal from an answer at all.
func TestFailRefusesWithTypedError(t *testing.T) {
	for _, test := range []struct {
		name   string
		target string
		want   string
	}{
		{"no argument", "", CodeMissingArgument},
		{"rejected argument", "not a domain", CodeInvalidArgument},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/tls?format=json", nil)
			Fail(recorder, request, "TLS", test.target, "headline here", "hint here")

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}

			// A refusal is about this request, so it must never be cached for
			// the next one.
			if store := recorder.Header().Get("Cache-Control"); store != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", store)
			}
			if vary := recorder.Header().Get("Vary"); !strings.Contains(vary, "Accept") {
				t.Errorf("Vary = %q, want it to contain Accept", vary)
			}

			var payload Payload
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatalf("body is not json: %v", err)
			}
			if payload.Error == nil {
				t.Fatal("payload.error is absent on a refusal")
			}
			if payload.Error.Code != test.want {
				t.Errorf("code = %q, want %q", payload.Error.Code, test.want)
			}
			if payload.Error.Message == "" || payload.Error.Hint == "" {
				t.Errorf("message and hint must both be set, got %+v", payload.Error)
			}
		})
	}
}

// The text form is what curl and most agents actually read, so the code has to
// survive into it rather than being a JSON-only affordance.
func TestFailTextCarriesTheCode(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/tls?format=text", nil)
	Fail(recorder, request, "TLS", "", "no host given", "this command needs a hostname")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "error "+CodeMissingArgument) {
		t.Errorf("text body does not name the code:\n%s", body)
	}
	// "live, held 3600s" on something that looked nothing up would be a lie.
	if strings.Contains(body, "live, held") {
		t.Errorf("refusal claims freshness it does not have:\n%s", body)
	}
}

// The counterpart, and the reason Fail was left at 200 for so long: an upstream
// that dies mid-answer is NOT a refusal. Part of that answer is real, it is
// cacheable, and turning it into a 4xx would throw away work that succeeded.
func TestDegradedAnswerStays200(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/tls?format=json", nil)

	result := New("TLS", "example.com")
	result.SetVerdict("warn", "answered in part", "one upstream failed")
	result.Degrade("rdap", "upstream timed out")
	result.HoldTTL(300, "tls")
	result.Write(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a degraded answer", recorder.Code)
	}
	if store := recorder.Header().Get("Cache-Control"); !strings.HasPrefix(store, "public") {
		t.Errorf("Cache-Control = %q, want a cacheable answer", store)
	}

	var payload Payload
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("body is not json: %v", err)
	}
	if payload.Error != nil {
		t.Errorf("degraded answer carries an error object: %+v", payload.Error)
	}
	if len(payload.Degraded) != 1 {
		t.Errorf("degraded list = %+v, want the one failure named", payload.Degraded)
	}
}
