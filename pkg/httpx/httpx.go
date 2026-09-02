// Package httpx makes HTTP requests to destinations the user influences.
//
// Every transport here is built on guard.Dialer, so the address check happens
// in Control immediately before connect, for the original request and for
// every redirect hop, with no window in between.
package httpx

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zaidmukaddam/dug/pkg/guard"
)

const (
	UserAgent    = "resolve/0.1 (network diagnostics)"
	MaxRedirects = 5
	MaxBody      = 256 << 10
	Timeout      = 8 * time.Second
)

type Hop struct {
	URL      string
	Status   int
	Location string
	IP       string
	MS       int
}

type Timing struct {
	DNSMS  int
	TCPMS  int
	TLSMS  int
	TTFBMS int
	TotalM int
}

type Response struct {
	URL     string
	Status  int
	Reason  string
	Headers http.Header
	Body    []byte
	Hops    []Hop
	Timing  Timing
	Err     string

	// A body that was cut or half-read is not a failed request: the status and
	// headers still answer the question that was asked. These say so without
	// costing the caller the rest of the response.
	BodyErr   string
	Truncated bool
}

func (r *Response) OK() bool { return r.Err == "" && r.Status > 0 && r.Status < 400 }

func (r *Response) Header(name string) string {
	if r.Headers == nil {
		return ""
	}
	return r.Headers.Get(name)
}

// Client returns a transport whose every connection is guarded.
func Client(timeout time.Duration) *http.Client {
	dialer := guard.Dialer(timeout)
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		DisableCompression:    true,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= MaxRedirects {
				return fmt.Errorf("more than %d redirects", MaxRedirects)
			}
			// The scheme is re-checked per hop; the address is re-checked by
			// the dialer's Control on the new connection.
			return guard.CheckScheme(req.URL.Scheme)
		},
	}
}

// Get follows redirects manually so each hop, its status and the address it
// resolved to can be reported.
func Get(ctx context.Context, rawURL string) *Response {
	return GetWithHeaders(ctx, rawURL, nil)
}

// GetWithHeaders is Get with request headers added, for a caller that is asking
// a question about negotiation rather than about the default representation.
// Extra headers are applied last and may override the defaults below, which is
// the point: the SEO and AEO screens ask for markdown and need that Accept to
// actually be sent.
func GetWithHeaders(ctx context.Context, rawURL string, extra map[string]string) *Response {
	return get(ctx, rawURL, extra, MaxBody)
}

// GetLimited is Get with its own body ceiling.
//
// MaxBody exists so that one hostile response cannot become one screen's worth
// of memory, and 256KB is the right size for anything meant to be read. It is
// the wrong size for looking inside a javascript bundle: they are routinely
// megabytes, and truncating one is how the WEBMCP command came to report that
// dug's own page does not reference the api it registers 26 tools with. The
// marker was 300KB past the cut.
func GetLimited(ctx context.Context, rawURL string, maxBody int) *Response {
	return get(ctx, rawURL, nil, maxBody)
}

func get(ctx context.Context, rawURL string, extra map[string]string, maxBody int) *Response {
	response := &Response{URL: rawURL}
	overall := time.Now()
	current := rawURL
	redirecting := false

	// Requests are hops plus one: following 5 redirects takes 6 requests, and
	// the sixth response is what proves the fifth redirect was not the last.
	for hop := 0; hop <= MaxRedirects; hop++ {
		parsed, err := url.Parse(current)
		if err != nil {
			response.Err = "unparseable url"
			break
		}
		if err := guard.CheckScheme(parsed.Scheme); err != nil {
			response.Err = err.Error()
			break
		}

		entry := Hop{URL: current}
		started := time.Now()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, current, nil)
		if err != nil {
			response.Err = err.Error()
			break
		}
		req.Header.Set("User-Agent", UserAgent)
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Accept-Encoding", "identity")
		for name, value := range extra {
			req.Header.Set(name, value)
		}

		// The transport is per hop because that is what keeps connectedTo
		// honest: a reused keep-alive connection never calls DialContext, so a
		// shared transport would report the previous hop's address. Its idle
		// connections are dropped at the end of the hop, otherwise each one
		// holds an fd and two goroutines for the life of the process.
		var connectedTo string
		transport := &http.Transport{
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				conn, err := guard.Dialer(Timeout).DialContext(ctx, network, address)
				if err == nil {
					connectedTo = conn.RemoteAddr().String()
				}
				return conn, err
			},
			TLSHandshakeTimeout: Timeout,
			DisableCompression:  true,
			IdleConnTimeout:     Timeout,
		}
		client := &http.Client{
			Timeout:   Timeout,
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse // handled here, one hop at a time
			},
		}

		reply, err := client.Do(req)
		entry.MS = int(time.Since(started).Milliseconds())
		if host, _, splitErr := net.SplitHostPort(connectedTo); splitErr == nil {
			entry.IP = host
		}

		if err != nil {
			transport.CloseIdleConnections()
			entry.Status = 0
			response.Hops = append(response.Hops, entry)
			response.Err = cleanErr(err)
			break
		}

		entry.Status = reply.StatusCode
		response.Status = reply.StatusCode
		response.Reason = reply.Status
		response.Headers = reply.Header
		response.URL = current
		response.Timing.TTFBMS = entry.MS

		// Each hop overwrites the last, so these describe the body kept.
		body, truncated, readErr := readBody(reply.Body, maxBody)
		reply.Body.Close()
		transport.CloseIdleConnections()
		response.Body, response.Truncated, response.BodyErr = body, truncated, ""
		if readErr != nil {
			response.BodyErr = cleanErr(readErr)
		}

		location := reply.Header.Get("Location")
		if isRedirect(reply.StatusCode) && location != "" {
			entry.Location = location
			response.Hops = append(response.Hops, entry)

			next, ok := absolute(parsed, location)
			if !ok {
				response.Err = "refused redirect target: " + location
				break
			}
			current = next
			redirecting = true
			continue
		}

		response.Hops = append(response.Hops, entry)
		redirecting = false
		break
	}

	// Running out of hops leaves Status holding a 3xx, which OK() would read as
	// a good answer and callers would render as the final response. Wording
	// matches Client()'s CheckRedirect so both paths say the same thing.
	if redirecting && response.Err == "" {
		response.Err = fmt.Sprintf("more than %d redirects", MaxRedirects)
	}

	response.Timing.TotalM = int(time.Since(overall).Milliseconds())
	return response
}

// readBody caps the body at limit, reading one byte past the cap so an
// oversized body is known to be cut rather than turning up later as content
// that will not parse.
func readBody(from io.Reader, limit int) (body []byte, truncated bool, err error) {
	body, err = io.ReadAll(io.LimitReader(from, int64(limit)+1))
	if len(body) > limit {
		return body[:limit], true, err
	}
	return body, false, err
}

func isRedirect(status int) bool {
	switch status {
	case 301, 302, 303, 307, 308:
		return true
	}
	return false
}

// absolute resolves a Location header, refusing anything off the allowlist.
func absolute(base *url.URL, location string) (string, bool) {
	ref, err := url.Parse(location)
	if err != nil {
		return "", false
	}
	next := base.ResolveReference(ref)
	if guard.CheckScheme(next.Scheme) != nil || next.Host == "" {
		return "", false
	}
	if port := next.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || guard.CheckPort(number) != nil {
			return "", false
		}
	}
	return next.String(), true
}

// GetJSON fetches and decodes, for RDAP and the IANA bootstrap.
func GetJSON(ctx context.Context, rawURL string, into any) (*Response, error) {
	response := Get(ctx, rawURL)
	if response.Err != "" {
		return response, errors.New(response.Err)
	}
	if response.Status >= 400 {
		return response, fmt.Errorf("status %d", response.Status)
	}
	return response, response.decode(into)
}

// decode reports a short body as short. A cut or half-read record is rarely
// valid json, and calling it unparseable sends the reader hunting for a
// malformed record that the registry never served.
func (r *Response) decode(into any) error {
	if r.Truncated {
		return fmt.Errorf("body truncated at %d bytes", MaxBody)
	}
	if r.BodyErr != "" {
		return fmt.Errorf("incomplete body: %s", r.BodyErr)
	}
	if err := json.Unmarshal(r.Body, into); err != nil {
		return fmt.Errorf("unparseable json: %w", err)
	}
	return nil
}

// Whois43 is the ccTLD fallback: plain text over port 43, guarded like
// everything else.
func Whois43(ctx context.Context, server, query string) (string, error) {
	dialer := guard.Dialer(Timeout)
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(server, "43"))
	if err != nil {
		return "", err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(Timeout))
	if _, err := conn.Write([]byte(query + "\r\n")); err != nil {
		return "", err
	}

	body, err := io.ReadAll(io.LimitReader(conn, MaxBody))
	if err != nil && len(body) == 0 {
		return "", err
	}
	return string(body), nil
}

// TLSConfig is the shared config for probes. Verification is requested so a
// failure is reported, not so the connection is abandoned.
func TLSConfig(host string, verify bool) *tls.Config {
	return &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: !verify,
		MinVersion:         tls.VersionTLS10,
		NextProtos:         []string{"h2", "http/1.1"},
	}
}

func cleanErr(err error) string {
	text := err.Error()
	if i := strings.Index(text, ": "); i > 0 && strings.Contains(text[:i], "Get \"") {
		text = text[i+2:]
	}
	return strings.TrimSpace(text)
}
