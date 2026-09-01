// Package screen builds the response every handler returns.
//
// A response is a list of blocks, each naming a frontend component and its
// exact props. Layout lives here in Span, not in the frontend. Upstream
// failures return 200 with a legible block and an entry in Degraded rather
// than a 4xx, so every failure mode still renders a screen.
package screen

import (
	"encoding/json"
	"net/http"
	"time"
)

const (
	TTLFloor    = 30
	MaxUpstream = 64
)

var ttlCaps = map[string]int{
	"dns":       3600,
	"rdap":      6 * 3600,
	"tls":       3600,
	"asn":       24 * 3600,
	"bootstrap": 24 * 3600,
	"http":      300,
}

type Block struct {
	Component string `json:"component"`
	Props     any    `json:"props"`
	Span      int    `json:"span"`
}

type Verdict struct {
	State    string `json:"state"` // ok, warn, none
	Headline string `json:"headline"`
	Detail   string `json:"detail"`
}

type Degraded struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}

// ErrorInfo is the machine-readable half of a refusal.
//
// The verdict says what went wrong in a sentence, which is the right shape for
// a person and the wrong one for a caller that has to branch on it. Code is a
// closed set, Message repeats the verdict headline, and Hint says what a
// corrected call looks like. Only ever set alongside a 4xx.
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// The closed set of Code values. An upstream that fails is not in here: that
// is a degraded 200, because part of the answer still stands.
const (
	// The command needs an argument and none arrived.
	CodeMissingArgument = "missing_argument"
	// An argument arrived and is not a thing this command can take.
	CodeInvalidArgument = "invalid_argument"
	// The caller pinned an API version this surface does not serve.
	CodeUnsupportedVersion = "unsupported_version"
	// A path under /api that no function answers on.
	CodeUnknownEndpoint = "unknown_endpoint"
	// The caller went past the published quota. Raised in proxy.ts, before a
	// request reaches any handler, which is why no Go code returns it.
	CodeRateLimited = "rate_limited"
)

// APIVersion is the contract this surface answers under. Sent on every
// response as X-API-Version, and accepted on a request as the same header for
// a caller that wants to pin it.
const APIVersion = "1"

// The published quota. Enforced in proxy.ts, which is the one place that sees
// every /api request before a handler runs; these exist so llms.txt and the
// OpenAPI document quote the same numbers the proxy applies, and pkg/wiring
// fails if the two drift.
const (
	RateLimit         = 60
	RateWindowSeconds = 60
)

// VersionPolicy is the promise a caller needs before it will hardcode a path.
//
// Stated rather than implemented as a /v1/ prefix, because a version segment
// that has never once been incremented tells a caller nothing. What is worth
// publishing is what happens when the shape does change and how a caller finds
// out. Lives here so llms.txt and the OpenAPI document quote one string rather
// than two that can disagree.
const VersionPolicy = "The paths are unversioned and additive. New commands, new blocks and " +
	"new fields may appear at any time, so parse defensively and ignore what you do " +
	"not recognise. A change that would break an existing caller ships under a /v2/ " +
	"path prefix instead of changing these; the current paths would then carry " +
	"Deprecation and Sunset headers (RFC 9745 and RFC 8594) for at least 180 days " +
	"before removal. No such header is set today, which is the signal that nothing " +
	"is deprecated."

type Payload struct {
	Command   string     `json:"command"`
	Target    string     `json:"target"`
	Verdict   Verdict    `json:"verdict"`
	TS        int64      `json:"ts"`
	TTL       int        `json:"ttl"`
	ElapsedMS int64      `json:"elapsed_ms"`
	Upstream  int        `json:"upstream_queries"`
	Notes     []string   `json:"notes"`
	Degraded  []Degraded `json:"degraded"`
	Blocks    []Block    `json:"blocks"`
	// Present only on a 4xx. Absent from every successful answer, which is why
	// it is omitempty rather than a zero-valued object on every response.
	Error *ErrorInfo `json:"error,omitempty"`
}

type Result struct {
	Command  string
	Target   string
	blocks   []Block
	notes    []string
	degraded []Degraded
	verdict  *Verdict
	ttl      int
	started  time.Time
	spent    int
	Budget   int
}

func New(command, target string) *Result {
	return &Result{
		Command: command,
		Target:  target,
		ttl:     -1,
		started: time.Now(),
		Budget:  MaxUpstream,
	}
}

func (r *Result) Add(component string, props any, span int) *Result {
	r.blocks = append(r.blocks, Block{component, props, FitSpan(component, props, span)})
	return r
}

// Note is provenance and limits, rendered as a footer below the graphs.
func (r *Result) Note(text string) *Result {
	r.notes = append(r.notes, text)
	return r
}

// Verdict is the answer in one sentence. State is ok, warn, or none.
func (r *Result) SetVerdict(state, headline, detail string) *Result {
	r.verdict = &Verdict{state, headline, detail}
	return r
}

// Degrade records a partial answer. Never silently returned as complete.
func (r *Result) Degrade(source, reason string) *Result {
	r.degraded = append(r.degraded, Degraded{source, reason})
	return r
}

func (r *Result) Spend(n int) { r.spent += n }

// HoldTTL keeps the response only as fresh as its shortest-lived component.
func (r *Result) HoldTTL(seconds int, kind string) {
	cap, ok := ttlCaps[kind]
	if !ok {
		cap = 3600
	}
	value := cap
	if seconds > 0 {
		value = seconds
	}
	if value < TTLFloor {
		value = TTLFloor
	}
	if value > cap {
		value = cap
	}
	if r.ttl < 0 || value < r.ttl {
		r.ttl = value
	}
}

func (r *Result) Payload() Payload {
	ttl := r.ttl
	if ttl < 0 {
		ttl = TTLFloor
	}

	blocks := r.blocks
	if len(r.degraded) > 0 {
		items := make([]CheckItem, 0, len(r.degraded))
		for _, entry := range r.degraded {
			items = append(items, CheckItem{Label: entry.Source, Done: false, Note: entry.Reason})
		}
		blocks = append([]Block{{
			Component: "GraphCheck",
			Props:     CheckProps{Title: "degraded", Items: items},
			Span:      3,
		}}, blocks...)
	}

	verdict := Verdict{State: "none", Headline: "answered"}
	if r.verdict != nil {
		verdict = *r.verdict
	}

	if r.notes == nil {
		r.notes = []string{}
	}
	if r.degraded == nil {
		r.degraded = []Degraded{}
	}
	if blocks == nil {
		blocks = []Block{}
	}

	return Payload{
		Command:   r.Command,
		Target:    r.Target,
		Verdict:   verdict,
		TS:        time.Now().UnixMilli(),
		TTL:       ttl,
		ElapsedMS: time.Since(r.started).Milliseconds(),
		Upstream:  r.spent,
		Notes:     r.notes,
		Degraded:  r.degraded,
		Blocks:    blocks,
	}
}

// Write sends the payload with a Cache-Control derived from its own TTL, as
// JSON or as text depending on what the request asked for.
func (r *Result) Write(w http.ResponseWriter, req *http.Request) {
	WritePayload(w, req, r.Payload())
}

// WritePrivate is Write for an answer that is about the caller rather than
// about the world.
//
// Every other answer here is a fact about a domain: two people asking get the
// same one, which is why the cache is public. An answer describing the request
// itself is true for exactly one requester, and putting it in a shared cache
// would hand one person's address to the next.
func (r *Result) WritePrivate(w http.ResponseWriter, req *http.Request) {
	writePayload(w, req, r.Payload(), http.StatusOK, "no-store")
}

// WritePayload is the single exit for every representation, so the text and the
// JSON can never describe different answers.
func WritePayload(w http.ResponseWriter, req *http.Request, payload Payload) {
	WritePayloadStatus(w, req, payload, http.StatusOK)
}

// WritePayloadStatus is WritePayload with the status spelled out, for the one
// case that is not 200: a refusal, where nothing was attempted because the
// request itself was wrong.
func WritePayloadStatus(w http.ResponseWriter, req *http.Request, payload Payload, status int) {
	writePayload(w, req, payload, status, "")
}

// cacheControl empty means derive it from the payload's own ttl.
func writePayload(
	w http.ResponseWriter, req *http.Request, payload Payload, status int, cacheControl string,
) {
	var body []byte
	contentType := "application/json"

	if WantsText(req) {
		body = []byte(payload.Text())
		contentType = "text/plain; charset=utf-8"
	} else {
		encoded, err := json.Marshal(payload)
		if err != nil {
			http.Error(w, "encoding failed", http.StatusInternalServerError)
			return
		}
		body = encoded
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Vary", "Accept, User-Agent")

	// Which contract this response was produced under. A caller pinning a
	// version has something to compare against, and when a second one exists
	// the deprecated surface is where Deprecation and Sunset will appear.
	w.Header().Set("X-API-Version", APIVersion)

	writeRateLimit(w, req)

	// RFC 8631: the machine description of this endpoint, and the page a person
	// would read instead. Discoverable from any response rather than only from
	// the one document that lists them.
	//
	// The same four relations next.config.ts sets on the html pages, so the two
	// halves of the site describe themselves identically. pkg/wiring fails if
	// they drift.
	w.Header().Set("Link", ServiceLinks)

	// A refusal is about this request, not about the world, so it must not sit
	// in a shared cache under the same key as a real answer.
	switch {
	case cacheControl != "":
		w.Header().Set("Cache-Control", cacheControl)
	case status >= 400:
		w.Header().Set("Cache-Control", "no-store")
	default:
		w.Header().Set("Cache-Control",
			"public, s-maxage="+itoa(payload.TTL)+", stale-while-revalidate="+itoa(payload.TTL*2))
	}

	w.WriteHeader(status)
	w.Write(body)
}

// writeRateLimit publishes the quota on every answer.
//
// The counting happens in proxy.ts, which is the only layer that sees every
// /api request before it is routed. It cannot set these on the response itself
// — a header set there does not reach the client for these routes — so it
// forwards the two live numbers as request headers and they are turned into
// real response headers here.
//
// The policy is emitted whether or not those arrive. A direct hit on a Go
// function in development has no proxy in front of it, and publishing the
// ceiling without the current count is still worth more to a caller than
// publishing nothing.
func writeRateLimit(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("RateLimit-Limit", itoa(RateLimit))
	w.Header().Set("RateLimit-Policy",
		`"fixed";q=`+itoa(RateLimit)+`;w=`+itoa(RateWindowSeconds))

	if req == nil {
		return
	}
	remaining := req.Header.Get("x-dug-rate-remaining")
	reset := req.Header.Get("x-dug-rate-reset")
	if remaining == "" || reset == "" {
		return
	}

	w.Header().Set("RateLimit-Remaining", remaining)
	w.Header().Set("RateLimit-Reset", reset)
	w.Header().Set("RateLimit", `"fixed";r=`+remaining+`;t=`+reset)
}

// Fail refuses a request whose arguments are wrong, as a screen a person can
// read and a status a caller can branch on.
//
// 400, not 200. Every call site is a rejected argument: nothing was looked up,
// so there is no partial answer to protect and no reason to claim success. That
// is the opposite of an upstream that failed mid-answer, which stays 200 with
// the failure named in degraded, because the rest of that answer is real.
//
// The code is derived rather than passed: with no target the argument never
// arrived, and with one it arrived and was rejected. That covers every caller,
// and keeps the closed set in one place instead of at twenty of them.
func Fail(w http.ResponseWriter, req *http.Request, command, target, headline, detail string) {
	code := CodeInvalidArgument
	if target == "" {
		code = CodeMissingArgument
	}

	r := New(command, target)
	r.SetVerdict("warn", headline, detail)
	r.Degrade(command, detail)
	r.Add("GraphSpec", SpecProps{Title: "failed", Rows: []SpecRow{
		{Label: "result", Value: headline, Accent: true},
		{Label: "reason", Value: detail},
		{Label: "note", Value: "no partial answer is being shown as complete"},
	}}, 3)

	payload := r.Payload()
	payload.Error = &ErrorInfo{
		Code:    code,
		Message: headline,
		Hint:    detail,
	}
	WritePayloadStatus(w, req, payload, http.StatusBadRequest)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
