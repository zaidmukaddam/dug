// Package guard validates destinations before anything connects to them.
//
// The check runs in net.Dialer.Control, which fires after resolution and
// before connect with the destination address in hand. There is no window
// between the check and the connect for a second DNS answer to slip into, and
// it fires for every candidate address during Happy Eyeballs, so a host with
// several A records is covered without extra work.
package guard

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"syscall"
	"time"
)

// Ports are an allowlist, never user input: 53 DNS, 43 WHOIS, 80 and 443 HTTP.
var allowedPorts = map[int]bool{53: true, 43: true, 80: true, 443: true}

var allowedSchemes = map[string]bool{"http": true, "https": true}

// Ranges the netip predicates do not already cover. CGNAT is the notable one:
// there is no IsCGNAT, and it is routable enough to look fine.
var blocked = []struct {
	prefix netip.Prefix
	reason string
}{
	{netip.MustParsePrefix("0.0.0.0/8"), "this network"},
	{netip.MustParsePrefix("100.64.0.0/10"), "CGNAT"},
	{netip.MustParsePrefix("192.0.0.0/24"), "IETF protocol assignments"},
	{netip.MustParsePrefix("192.0.2.0/24"), "documentation"},
	{netip.MustParsePrefix("192.88.99.0/24"), "6to4 relay anycast"},
	{netip.MustParsePrefix("198.18.0.0/15"), "benchmarking"},
	{netip.MustParsePrefix("198.51.100.0/24"), "documentation"},
	{netip.MustParsePrefix("203.0.113.0/24"), "documentation"},
	{netip.MustParsePrefix("240.0.0.0/4"), "reserved"},
	{netip.MustParsePrefix("255.255.255.255/32"), "broadcast"},
	{netip.MustParsePrefix("64:ff9b:1::/48"), "local-use NAT64"},
	{netip.MustParsePrefix("100::/64"), "discard-only"},
	{netip.MustParsePrefix("2001::/23"), "IETF protocol assignments"},
	{netip.MustParsePrefix("2001:db8::/32"), "documentation"},
	{netip.MustParsePrefix("fec0::/10"), "site-local"},
	{netip.MustParsePrefix("5f00::/16"), "srv6"},
}

var nat64 = netip.MustParsePrefix("64:ff9b::/96")

// Blocked is a refused destination. Never retried, never followed.
type Blocked struct {
	Target string
	Reason string
}

func (b *Blocked) Error() string {
	return fmt.Sprintf("%s blocked: %s", b.Target, b.Reason)
}

// embeddedV4 returns the IPv4 address a v6 form carries, if it carries one.
//
// ::ffff:127.0.0.1 is loopback in a v6 costume, and so are the 6to4 and NAT64
// forms. A guard that skips this has a hole the width of private space.
func embeddedV4(addr netip.Addr) (netip.Addr, bool) {
	if addr.Is4() || !addr.Is6() {
		return netip.Addr{}, false
	}

	if addr.Is4In6() {
		return addr.Unmap(), true
	}

	bytes := addr.As16()

	// 2002::/16 carries the IPv4 in bytes 2 to 5.
	if bytes[0] == 0x20 && bytes[1] == 0x02 {
		return netip.AddrFrom4([4]byte{bytes[2], bytes[3], bytes[4], bytes[5]}), true
	}

	if nat64.Contains(addr) {
		return netip.AddrFrom4([4]byte{bytes[12], bytes[13], bytes[14], bytes[15]}), true
	}

	// ::a.b.c.d, the IPv4-compatible form: the top 96 bits are zero and the
	// v4 is the last four bytes. Deprecated, still parseable, and unseen by
	// every netip predicate. :: itself and ::1 are excluded so they keep
	// their own names (unspecified, loopback) in the reason.
	var top [12]byte
	if [12]byte(bytes[:12]) == top && !addr.IsUnspecified() && !addr.IsLoopback() {
		return netip.AddrFrom4([4]byte{bytes[12], bytes[13], bytes[14], bytes[15]}), true
	}

	return netip.Addr{}, false
}

// reason returns why an address is refused, or "" if it is safe to reach.
func reason(addr netip.Addr) string {
	if !addr.IsValid() {
		return "not an IP address"
	}

	// Unmap before any predicate. IsLoopback on ::ffff:127.0.0.1 is false, and
	// forgetting this is the single most common bypass in guards of this kind.
	addr = addr.Unmap()

	// An address carrying an embedded IPv4 is judged entirely by that inner
	// address. NAT64 and 6to4 are real routes to what they wrap, so refusing
	// the wrapper prefix outright would block every public host on a
	// NAT64-only network while adding nothing.
	if inner, ok := embeddedV4(addr); ok {
		if why := reason(inner); why != "" {
			return fmt.Sprintf("%s via embedded IPv4 %s", why, inner)
		}
		return ""
	}

	switch {
	case addr.IsUnspecified():
		return "unspecified"
	case addr.IsLoopback():
		return "loopback"
	case addr.IsPrivate():
		return "RFC 1918 or unique local"
	case addr.IsLinkLocalUnicast():
		return "link-local, cloud metadata"
	case addr.IsLinkLocalMulticast(), addr.IsInterfaceLocalMulticast(), addr.IsMulticast():
		return "multicast"
	}

	for _, entry := range blocked {
		if entry.prefix.Contains(addr) {
			return entry.reason
		}
	}

	return ""
}

func Check(addr netip.Addr) error {
	if why := reason(addr); why != "" {
		return &Blocked{Target: addr.String(), Reason: why}
	}
	return nil
}

func CheckString(value string) (netip.Addr, error) {
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}, &Blocked{Target: value, Reason: "not an IP address"}
	}
	return addr, Check(addr)
}

func CheckPort(port int) error {
	if !allowedPorts[port] {
		return &Blocked{Target: fmt.Sprint(port), Reason: "port not on the allowlist"}
	}
	return nil
}

func CheckScheme(scheme string) error {
	if !allowedSchemes[scheme] {
		return &Blocked{Target: scheme, Reason: "scheme not on the allowlist"}
	}
	return nil
}

// AllowedPorts is the allowlist, for the SRC screen.
func AllowedPorts() []int {
	out := make([]int, 0, len(allowedPorts))
	for port := range allowedPorts {
		out = append(out, port)
	}
	return out
}

// Denylist is the explicit range list, for the SRC screen.
func Denylist() [][2]string {
	out := make([][2]string, 0, len(blocked))
	for _, entry := range blocked {
		out = append(out, [2]string{entry.prefix.String(), entry.reason})
	}
	return out
}

// control is the seam the whole guard rests on: it runs with the resolved
// destination, immediately before connect.
func control(enforcePort bool) func(string, string, syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		ap, err := netip.ParseAddrPort(address)
		if err != nil {
			return &Blocked{Target: address, Reason: "unparseable destination"}
		}
		if enforcePort {
			if err := CheckPort(int(ap.Port())); err != nil {
				return err
			}
		}
		return Check(ap.Addr())
	}
}

// Dialer returns a dialer that refuses any destination the guard rejects.
func Dialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{Timeout: timeout, Control: control(true)}
}

// PortScanDialer is the one exception, for the PORTS command, where the port is
// the question being asked. Only the port allowlist is waived; every address is
// still checked, so the scanner cannot reach loopback or private space.
func PortScanDialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{Timeout: timeout, Control: control(false)}
}

// Resolve returns the validated addresses for a host, for callers that need to
// report them. Connections should still go through Dialer, which re-checks.
func Resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		if err := Check(addr); err != nil {
			return nil, err
		}
		return []netip.Addr{addr}, nil
	}

	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, &Blocked{Target: host, Reason: "does not resolve: " + err.Error()}
	}
	if len(addrs) == 0 {
		return nil, &Blocked{Target: host, Reason: "resolved to no addresses"}
	}

	// Every returned address is checked, not just the first: a single private
	// answer in the set is enough to refuse the host.
	for _, addr := range addrs {
		if err := Check(addr); err != nil {
			return nil, err
		}
	}
	return addrs, nil
}
