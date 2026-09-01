package httpx

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

// A body cut at the cap used to be indistinguishable from a body that ended
// there, and a reset mid-body used to read as a clean short body.
func TestReadBodyReportsTruncationAndKeepsTheError(t *testing.T) {
	oversized := strings.Repeat("x", MaxBody+64)
	body, truncated, err := readBody(strings.NewReader(oversized))
	if !truncated {
		t.Error("a body past the cap was not reported as truncated")
	}
	if len(body) != MaxBody {
		t.Errorf("kept %d bytes, want %d", len(body), MaxBody)
	}
	if err != nil {
		t.Errorf("reading a long body returned %v", err)
	}

	exact := strings.Repeat("x", MaxBody)
	if body, truncated, _ = readBody(strings.NewReader(exact)); truncated {
		t.Error("a body of exactly the cap was reported as truncated")
	} else if len(body) != MaxBody {
		t.Errorf("kept %d bytes of an exact-cap body, want %d", len(body), MaxBody)
	}

	reset := errors.New("connection reset by peer")
	body, truncated, err = readBody(io.MultiReader(strings.NewReader(`{"partial":`), iotest.ErrReader(reset)))
	if !errors.Is(err, reset) {
		t.Errorf("a mid-body reset returned err %v, want it kept", err)
	}
	if truncated {
		t.Error("a short body that failed to read was reported as truncated")
	}
	if string(body) != `{"partial":` {
		t.Errorf("dropped what had been read: %q", body)
	}
}

// The misleading reason is half the bug: a 300 KiB RDAP record that was cut at
// the cap is not a registry serving broken json.
func TestTruncatedBodyIsNotCalledUnparseable(t *testing.T) {
	var into map[string]any

	cut := &Response{Body: []byte(`{"handle":"EXAM`), Truncated: true}
	err := cut.decode(&into)
	if err == nil {
		t.Fatal("a truncated body decoded without complaint")
	}
	if strings.Contains(err.Error(), "unparseable") || !strings.Contains(err.Error(), "truncated") {
		t.Errorf("truncated body reported as %q", err)
	}

	half := &Response{Body: []byte(`{"handle":"EXAM`), BodyErr: "connection reset by peer"}
	if err = half.decode(&into); err == nil || strings.Contains(err.Error(), "unparseable") {
		t.Errorf("half-read body reported as %v", err)
	}

	broken := &Response{Body: []byte(`{"handle":}`)}
	if err = broken.decode(&into); err == nil || !strings.Contains(err.Error(), "unparseable json") {
		t.Errorf("genuinely broken json reported as %v, want unparseable json", err)
	}

	whole := &Response{Body: []byte(`{"handle":"EXAMPLE"}`)}
	if err = whole.decode(&into); err != nil {
		t.Errorf("a complete body failed to decode: %v", err)
	}
}
