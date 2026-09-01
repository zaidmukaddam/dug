// ME. The address this request came from.
//
// The one command whose subject is the caller rather than a target. Everything
// else here answers a question about the world and two people asking get the
// same answer, which is why those are publicly cacheable. This one is true for
// exactly one requester, so it is written with no-store and never shares a
// cache entry.
//
// There is no argument. `curl dug.sh/me` is the whole interface.
package handler

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	addrapi "github.com/zaidmukaddam/dug/api/addr"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	result := screen.New("ME", "")

	address, source := clientAddr(r)
	if address == "" {
		result.SetVerdict("warn", "the address this request came from is not visible",
			"no forwarded-for header and no usable remote address")
		result.Add("GraphSpec", screen.SpecProps{Title: "request", Rows: []screen.SpecRow{
			{Label: "address", Value: "unknown", Accent: true},
			{Label: "reason", Value: "nothing in the request named an address"},
		}}, 3)
		result.WritePrivate(w, r)
		return
	}

	result.Target = address

	// Enrichment goes through the address guard, which refuses private space on
	// purpose. That is right for a target someone named and wrong as an error
	// here: running the project locally, the caller genuinely is 127.0.0.1, and
	// showing a refusal would read as a fault rather than as the answer.
	if parsed, err := netip.ParseAddr(address); err == nil && !isPublic(parsed) {
		result.SetVerdict("ok", "this request came from "+address,
			"a private address, so there is no origin network to look up. "+
				"running locally, this is your own machine.")
		result.Add("GraphSpec", screen.SpecProps{Title: "request", Rows: []screen.SpecRow{
			{Label: "address", Value: address, Accent: true},
			{Label: "version", Value: version(parsed)},
			{Label: "seen via", Value: source},
			{Label: "scope", Value: "private, not routable on the public internet"},
		}}, 3)
		result.WritePrivate(w, r)
		return
	}

	// Reverse dns, origin as, prefix and neighbours, from the same code IP uses.
	addrapi.Describe(r, result, address)

	// Describe holds an asn ttl, which is a day. That is right for a fact about
	// an address and wrong on the line this prints, where "held 86400s" sits
	// above a response that is no-store. HoldTTL keeps the minimum, so this
	// lowers it rather than fighting it.
	result.HoldTTL(screen.TTLFloor, "http")

	// Set after Describe, which writes the verdict for a named address. The
	// question here was "what is my address", so that is what the sentence
	// answers; the evidence underneath is unchanged.
	parsed, _ := netip.ParseAddr(address)
	result.SetVerdict("ok", "this request came from "+address,
		version(parsed)+", seen via "+source+". this is the address a server you connect to sees.")

	result.WritePrivate(w, r)
}

// clientAddr returns the caller's address and which header it came from.
//
// X-Forwarded-For first, because in production this runs behind Vercel's edge
// and RemoteAddr is the proxy rather than the caller. The leftmost entry is the
// original client: the header is appended to hop by hop, and everything to the
// right of the first entry is infrastructure. It is client-settable and worth
// nothing as a security control, which is fine, because nothing here is
// authorised by it: the value is reported back to the person who sent it.
func clientAddr(r *http.Request) (address string, source string) {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
		if ip := normalise(first); ip != "" {
			return ip, "x-forwarded-for"
		}
	}
	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
		if ip := normalise(real); ip != "" {
			return ip, "x-real-ip"
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := normalise(host); ip != "" {
		return ip, "socket"
	}
	return "", ""
}

// normalise parses and re-prints, so a v6 address arrives in one spelling and
// an ipv4-in-ipv6 wrapper is unwrapped to the address inside.
func normalise(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "[]")
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return ""
	}
	return addr.Unmap().String()
}

func isPublic(addr netip.Addr) bool {
	addr = addr.Unmap()
	return !addr.IsLoopback() && !addr.IsPrivate() && !addr.IsLinkLocalUnicast() &&
		!addr.IsLinkLocalMulticast() && !addr.IsUnspecified() && !addr.IsMulticast()
}

func version(addr netip.Addr) string {
	if addr.Is4() || addr.Is4In6() {
		return "ipv4"
	}
	if addr.IsValid() {
		return "ipv6"
	}
	return "unknown"
}
