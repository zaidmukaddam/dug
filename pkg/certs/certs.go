// Package certs retrieves and describes TLS certificate chains.
//
// This is one of the reasons the project is in Go. crypto/tls completes a
// handshake against an expired or misissued certificate and still exposes the
// whole chain through ConnectionState.PeerCertificates, so "connect anyway and
// tell me what is wrong" needs no second code path.
package certs

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/zaidmukaddam/dug/pkg/guard"
)

var Protocols = []struct {
	Label   string
	Version uint16
}{
	{"TLS 1.0", tls.VersionTLS10},
	{"TLS 1.1", tls.VersionTLS11},
	{"TLS 1.2", tls.VersionTLS12},
	{"TLS 1.3", tls.VersionTLS13},
}

// CA/Browser Forum ballot SC-081: the ceiling depends on issue date.
var lifetimeCaps = []struct {
	from time.Time
	days int
}{
	{time.Date(2029, 3, 15, 0, 0, 0, 0, time.UTC), 47},
	{time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC), 100},
	{time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), 200},
	{time.Unix(0, 0).UTC(), 398},
}

func MaxLifetimeDays(issued time.Time) int {
	for _, entry := range lifetimeCaps {
		if !issued.Before(entry.from) {
			return entry.days
		}
	}
	return 398
}

type Cert struct {
	Role string
	// Label is what a screen shows. Role stays semantic because it is compared
	// against "leaf", so the two are separate fields: a chain with two
	// intermediates needs them told apart on screen without changing what the
	// role means.
	Label      string
	Subject    string
	Issuer     string
	NotBefore  time.Time
	NotAfter   time.Time
	Serial     string
	Signature  string
	Key        string
	SANs       []string
	IsCA       bool
	SelfSigned bool
}

func (c Cert) DaysTotal() int {
	days := int(c.NotAfter.Sub(c.NotBefore).Hours() / 24)
	if days < 1 {
		return 1
	}
	return days
}

func (c Cert) DaysLeft(now time.Time) int {
	return int(c.NotAfter.Sub(now).Hours() / 24)
}

type Handshake struct {
	Host        string
	Port        int
	IP          string
	Version     string
	Cipher      string
	ALPN        string
	Verified    bool
	VerifyError string
	Chain       []Cert
	Protocols   map[string]*bool
	MS          int
	Err         string
	ChainSource string
}

func describe(cert *x509.Certificate, role string) Cert {
	return Cert{
		Role:       role,
		Label:      role,
		Subject:    commonName(cert.Subject.CommonName, cert.Subject.Organization, cert.Subject.String()),
		Issuer:     commonName(cert.Issuer.CommonName, cert.Issuer.Organization, cert.Issuer.String()),
		NotBefore:  cert.NotBefore,
		NotAfter:   cert.NotAfter,
		Serial:     fmt.Sprintf("%x", cert.SerialNumber),
		Signature:  cert.SignatureAlgorithm.String(),
		Key:        keyDescription(cert),
		SANs:       append([]string(nil), cert.DNSNames...),
		IsCA:       cert.IsCA,
		SelfSigned: cert.Subject.String() == cert.Issuer.String(),
	}
}

func commonName(cn string, org []string, fallback string) string {
	if cn != "" {
		return cn
	}
	if len(org) > 0 {
		return org[0]
	}
	return fallback
}

func keyDescription(cert *x509.Certificate) string {
	switch key := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		return fmt.Sprintf("RSA %d", key.N.BitLen())
	case *ecdsa.PublicKey:
		return "EC " + key.Curve.Params().Name
	case ed25519.PublicKey:
		return "Ed25519"
	}
	return "unknown"
}

// Connect handshakes and describes what came back. A verification failure is a
// finding, so the handshake is not abandoned when it happens.
func Connect(ctx context.Context, host string, port int, timeout time.Duration) Handshake {
	out := Handshake{Host: host, Port: port, Protocols: map[string]*bool{}}
	started := time.Now()

	addrs, err := guard.Resolve(ctx, host)
	if err != nil {
		out.Err = err.Error()
		return out
	}
	out.IP = addrs[0].String()

	dialer := &tls.Dialer{
		NetDialer: guard.Dialer(timeout),
		Config:    tlsConfig(host, true),
	}

	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, itoa(port)))
	if err != nil {
		// Retry without verification so a broken certificate is still
		// described rather than merely refused.
		out.VerifyError = cleanTLSError(err)
		dialer.Config = tlsConfig(host, false)
		conn, err = dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, itoa(port)))
		if err != nil {
			out.Err = cleanTLSError(err)
			return out
		}
		out.Verified = false
	} else {
		out.Verified = true
	}

	tlsConn := conn.(*tls.Conn)
	state := tlsConn.ConnectionState()
	out.Version = versionName(state.Version)
	out.Cipher = tls.CipherSuiteName(state.CipherSuite)
	out.ALPN = state.NegotiatedProtocol

	// Servers do not send the root, but a successful verification built the
	// full path including it, so VerifiedChains is a better source than the
	// wire chain when it is available.
	source := state.PeerCertificates
	out.ChainSource = "served by the host"
	if len(state.VerifiedChains) > 0 && len(state.VerifiedChains[0]) > len(source) {
		source = state.VerifiedChains[0]
		out.ChainSource = "served by the host, root from the verified path"
	}

	for i, cert := range source {
		role := "intermediate"
		switch {
		case i == 0:
			role = "leaf"
		case i == len(source)-1 && cert.Subject.String() == cert.Issuer.String():
			role = "root"
		}
		out.Chain = append(out.Chain, describe(cert, role))
	}
	conn.Close()

	// An unverified handshake has no built path, so fall back to resolving the
	// issuer of the topmost served certificate against the system pool.
	if out.ChainSource == "served by the host" && len(source) > 0 {
		top := source[len(source)-1]
		if top.Subject.String() != top.Issuer.String() {
			if root := findRoot(top); root != nil {
				out.Chain = append(out.Chain, describe(root, "root"))
				out.ChainSource += ", root from the system trust store"
			}
		}
	}

	labelChain(out.Chain)

	out.MS = int(time.Since(started).Milliseconds())
	out.Protocols = probeProtocols(ctx, host, port, timeout)
	return out
}

// labelChain numbers the intermediates when there is more than one, counting
// out from the leaf. Chains of two intermediates are ordinary, and both
// rendering as "intermediate" leaves no way to tell which row is which. A
// single intermediate needs no number.
func labelChain(chain []Cert) {
	total := 0
	for _, cert := range chain {
		if cert.Role == "intermediate" {
			total++
		}
	}
	if total < 2 {
		return
	}

	seen := 0
	for i := range chain {
		if chain[i].Role == "intermediate" {
			seen++
			chain[i].Label = fmt.Sprintf("intermediate %d", seen)
		}
	}
}

func findRoot(top *x509.Certificate) *x509.Certificate {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		return nil
	}
	opts := x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}
	chains, err := top.Verify(opts)
	if err != nil || len(chains) == 0 {
		return nil
	}
	chain := chains[0]
	candidate := chain[len(chain)-1]
	if candidate.Subject.String() == top.Subject.String() {
		return nil
	}
	return candidate
}

// probeProtocols runs one handshake per version. A nil result means the client
// could not offer that version at all, which is a different statement from the
// server refusing it.
func probeProtocols(ctx context.Context, host string, port int, timeout time.Duration) map[string]*bool {
	// Four independent handshakes, one per protocol version, run at once: a
	// host that silently drops TLS 1.0/1.1 otherwise costs the full timeout
	// per version, one after another.
	offered := make([]bool, len(Protocols))
	var group errgroup.Group
	for i, entry := range Protocols {
		group.Go(func() error {
			config := tlsConfig(host, false)
			config.MinVersion = entry.Version
			config.MaxVersion = entry.Version

			dialer := &tls.Dialer{NetDialer: guard.Dialer(timeout), Config: config}
			conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, itoa(port)))
			if err == nil {
				// A server that quietly negotiates up would otherwise be
				// reported as supporting the floor it was offered.
				offered[i] = conn.(*tls.Conn).ConnectionState().Version == entry.Version
				conn.Close()
			}
			return nil
		})
	}
	_ = group.Wait()

	results := map[string]*bool{}
	for i, entry := range Protocols {
		value := offered[i]
		results[entry.Label] = &value
	}
	return results
}

func tlsConfig(host string, verify bool) *tls.Config {
	return &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: !verify,
		MinVersion:         tls.VersionTLS10,
		NextProtos:         []string{"h2", "http/1.1"},
	}
}

func versionName(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	}
	return "unknown"
}

// GanttItems normalises validity spans to fractions along one shared track,
// which is what the component takes.
func GanttItems(chain []Cert) (items []struct {
	Label  string
	Start  float64
	End    float64
	Accent bool
}, ticks []string) {
	if len(chain) == 0 {
		return nil, nil
	}

	origin, finish := chain[0].NotBefore, chain[0].NotAfter
	for _, cert := range chain {
		if cert.NotBefore.Before(origin) {
			origin = cert.NotBefore
		}
		if cert.NotAfter.After(finish) {
			finish = cert.NotAfter
		}
	}
	span := finish.Sub(origin).Seconds()
	if span <= 0 {
		span = 1
	}

	fraction := func(moment time.Time) float64 {
		value := moment.Sub(origin).Seconds() / span
		if value < 0 {
			return 0
		}
		if value > 1 {
			return 1
		}
		return value
	}

	for _, cert := range chain {
		start := fraction(cert.NotBefore)
		end := fraction(cert.NotAfter)
		if end < start+0.01 {
			end = start + 0.01
		}
		items = append(items, struct {
			Label  string
			Start  float64
			End    float64
			Accent bool
		}{cert.Label, round4(start), round4(end), cert.Role == "leaf"})
	}

	for i := 0; i < 5; i++ {
		moment := origin.Add(time.Duration(span*float64(i)/4) * time.Second)
		ticks = append(ticks, moment.Format("2006"))
	}
	return items, ticks
}

// NowFraction is the marker position for the current moment on that track.
func NowFraction(chain []Cert, now time.Time) float64 {
	if len(chain) == 0 {
		return 0
	}
	origin, finish := chain[0].NotBefore, chain[0].NotAfter
	for _, cert := range chain {
		if cert.NotBefore.Before(origin) {
			origin = cert.NotBefore
		}
		if cert.NotAfter.After(finish) {
			finish = cert.NotAfter
		}
	}
	span := finish.Sub(origin).Seconds()
	if span <= 0 {
		return 0
	}
	value := now.Sub(origin).Seconds() / span
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return round4(value)
}

// Covers reports whether the leaf actually covers the name asked for.
func Covers(leaf Cert, name string) bool {
	names := append([]string(nil), leaf.SANs...)
	names = append(names, leaf.Subject)
	for _, candidate := range names {
		if matchName(candidate, name) {
			return true
		}
	}
	return false
}

// matchName applies the rule a TLS client applies (RFC 6125): DNS names compare
// case-insensitively, and a wildcard is only the whole leftmost label and stands
// for exactly one label, so *.example.com covers a.example.com but neither
// a.b.example.com nor example.com itself.
func matchName(candidate, name string) bool {
	if strings.EqualFold(candidate, name) {
		return true
	}
	suffix, ok := strings.CutPrefix(candidate, "*.")
	if !ok || suffix == "" {
		return false
	}
	label, rest, ok := strings.Cut(name, ".")
	return ok && label != "" && strings.EqualFold(rest, suffix)
}

func SortedSANs(leaf Cert) []string {
	out := append([]string(nil), leaf.SANs...)
	sort.Strings(out)
	return out
}

func round4(value float64) float64 {
	return float64(int(value*10000+0.5)) / 10000
}

func itoa(n int) string { return fmt.Sprint(n) }

func cleanTLSError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	for _, prefix := range []string{"tls: ", "x509: "} {
		if i := indexOf(text, prefix); i >= 0 {
			return text[i:]
		}
	}
	return text
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
