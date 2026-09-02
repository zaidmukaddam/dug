package screen

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The default applies when no command is given at all, which is the shape
// every plain /api/resolve?target=... call takes.
func TestArgumentAppliesDefaultCommand(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/resolve?target=example.com", nil)

	command, target, ok := Argument(recorder, request, "/api/resolve", "DIG")

	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if command != "DIG" {
		t.Errorf("command = %q, want DIG", command)
	}
	if target != "example.com" {
		t.Errorf("target = %q, want example.com", target)
	}
}

// A lowercase command is what a hand-typed query string carries. The grammar
// is case sensitive internally, so the helper is the one place that upcases
// it rather than every caller checking twice.
func TestArgumentUppercasesCommand(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/resolve?command=ttl&target=example.com", nil)

	command, target, ok := Argument(recorder, request, "/api/resolve", "DIG")

	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if command != "TTL" {
		t.Errorf("command = %q, want TTL", command)
	}
	if target != "example.com" {
		t.Errorf("target = %q, want example.com", target)
	}
}

// TLS is a real command, just not one /api/resolve answers. The refusal has
// to name what the endpoint does answer, or an agent that got this wrong has
// no way to self-correct.
func TestArgumentRefusesACommandTheEndpointDoesNotAnswer(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/resolve?command=TLS&target=example.com&format=json", nil)

	_, _, ok := Argument(recorder, request, "/api/resolve", "DIG")

	if ok {
		t.Fatalf("ok = true, want false")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}

	var payload Payload
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("body is not json: %v", err)
	}
	if payload.Error == nil {
		t.Fatal("payload.error is absent on a refusal")
	}
	if payload.Error.Code != CodeInvalidArgument {
		t.Errorf("code = %q, want %q", payload.Error.Code, CodeInvalidArgument)
	}
	if !strings.Contains(payload.Error.Hint, "DIG") || !strings.Contains(payload.Error.Hint, "TTL") {
		t.Errorf("hint = %q, want it to list DIG and TTL", payload.Error.Hint)
	}
}

// An empty target is a missing_argument, not an invalid one: nothing was
// even sent to reject.
func TestArgumentRefusesAnEmptyTarget(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/resolve?format=json", nil)

	_, _, ok := Argument(recorder, request, "/api/resolve", "DIG")

	if ok {
		t.Fatalf("ok = true, want false")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}

	var payload Payload
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("body is not json: %v", err)
	}
	if payload.Error == nil {
		t.Fatal("payload.error is absent on a refusal")
	}
	if payload.Error.Code != CodeMissingArgument {
		t.Errorf("code = %q, want %q", payload.Error.Code, CodeMissingArgument)
	}
	if payload.Error.Message != "no domain given" {
		t.Errorf("message = %q, want %q", payload.Error.Message, "no domain given")
	}
}

// PING's argument kind is "endpoint" in the grammar, but nobody types
// "endpoint" — the refusal has to read as a word a person would type.
func TestArgumentNamesAnEndpointArgumentAsHost(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/probe?format=json", nil)

	_, _, ok := Argument(recorder, request, "/api/probe", "PING")

	if ok {
		t.Fatalf("ok = true, want false")
	}

	var payload Payload
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("body is not json: %v", err)
	}
	if payload.Error == nil {
		t.Fatal("payload.error is absent on a refusal")
	}
	if payload.Error.Message != "no host given" {
		t.Errorf("message = %q, want %q", payload.Error.Message, "no host given")
	}
}

// Padding around a target is a copy-paste accident, not intent, and it used
// to be the handler's job to strip it. Now it is the helper's, once, for
// every caller.
func TestArgumentTrimsThePaddedTarget(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/resolve?target=%20%20example.com%20%20", nil)

	_, target, ok := Argument(recorder, request, "/api/resolve", "DIG")

	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if target != "example.com" {
		t.Errorf("target = %q, want example.com", target)
	}
}
