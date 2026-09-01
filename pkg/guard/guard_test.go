package guard

import (
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

// The M0 exit gate. Every entry here must be refused.
var mustBlock = []struct{ addr, why string }{
	{"127.0.0.1", "loopback"},
	{"127.1.2.3", "loopback"},
	{"::1", "loopback"},
	{"169.254.169.254", "cloud metadata, the highest value SSRF target"},
	{"fe80::1", "link-local"},
	{"10.0.0.1", "RFC 1918"},
	{"10.255.255.255", "RFC 1918"},
	{"172.16.0.1", "RFC 1918"},
	{"172.31.255.254", "RFC 1918"},
	{"192.168.1.1", "RFC 1918"},
	{"100.64.0.1", "CGNAT, and there is no netip predicate for it"},
	{"100.127.255.255", "CGNAT"},
	{"0.0.0.0", "this network"},
	{"0.1.2.3", "this network"},
	{"255.255.255.255", "broadcast"},
	{"198.18.0.1", "benchmarking"},
	{"240.0.0.1", "reserved"},
	{"192.0.2.1", "documentation"},
	{"198.51.100.5", "documentation"},
	{"203.0.113.9", "documentation"},
	{"224.0.0.1", "multicast"},
	{"239.255.255.250", "multicast"},
	{"ff02::1", "multicast"},
	{"fc00::1", "unique local"},
	{"fd12:3456:789a::1", "unique local"},
	{"::", "unspecified"},
	{"2001:db8::1", "documentation"},

	// IPv4-mapped forms. These are the bypass the scope names explicitly:
	// IsLoopback is false on all of them until Unmap is called.
	{"::ffff:127.0.0.1", "loopback wearing a v6 costume"},
	{"::ffff:169.254.169.254", "metadata, mapped"},
	{"::ffff:10.0.0.1", "RFC 1918, mapped"},
	{"::ffff:192.168.0.1", "RFC 1918, mapped"},
	{"::ffff:7f00:1", "loopback, mapped in hex form"},
	{"::ffff:a9fe:a9fe", "metadata, mapped in hex form"},

	// 6to4 and NAT64 wrapping private space.
	{"2002:7f00:0001::", "6to4 wrapping loopback"},
	{"2002:a9fe:a9fe::", "6to4 wrapping metadata"},
	{"2002:c0a8:0001::", "6to4 wrapping RFC 1918"},
	{"64:ff9b::7f00:1", "NAT64 wrapping loopback"},
	{"64:ff9b::a9fe:a9fe", "NAT64 wrapping metadata"},
	{"64:ff9b::a00:1", "NAT64 wrapping RFC 1918"},
}

// These must be reachable, or the tool cannot do its job. The NAT64 and 6to4
// entries matter: judging them by the wrapper prefix rather than the embedded
// address would break every lookup on a NAT64-only network.
var mustAllow = []string{
	"1.1.1.1",
	"8.8.8.8",
	"9.9.9.9",
	"93.184.215.14",
	"2606:4700:4700::1111",
	"2001:4860:4860::8888",
	"64:ff9b::14cf:4952",
	"64:ff9b::5db8:d822",
	"2002:5db8:d822::",
	"::ffff:93.184.215.14",
}

func TestDenylistRefuses(t *testing.T) {
	for _, tc := range mustBlock {
		addr := netip.MustParseAddr(tc.addr)
		if err := Check(addr); err == nil {
			t.Errorf("%s was allowed, expected refusal: %s", tc.addr, tc.why)
		}
	}
}

func TestDenylistAllows(t *testing.T) {
	for _, value := range mustAllow {
		addr := netip.MustParseAddr(value)
		if err := Check(addr); err != nil {
			t.Errorf("%s was refused: %v", value, err)
		}
	}
}

func TestPortAllowlist(t *testing.T) {
	for _, port := range []int{22, 25, 3306, 6379, 8080, 0, 65535, 5432} {
		if err := CheckPort(port); err == nil {
			t.Errorf("port %d was allowed", port)
		}
	}
	for _, port := range []int{53, 43, 80, 443} {
		if err := CheckPort(port); err != nil {
			t.Errorf("port %d was refused: %v", port, err)
		}
	}
}

func TestSchemeAllowlist(t *testing.T) {
	for _, scheme := range []string{"file", "gopher", "ftp", "dict", "ldap", "jar"} {
		if err := CheckScheme(scheme); err == nil {
			t.Errorf("scheme %s was allowed", scheme)
		}
	}
	for _, scheme := range []string{"http", "https"} {
		if err := CheckScheme(scheme); err != nil {
			t.Errorf("scheme %s was refused: %v", scheme, err)
		}
	}
}

// The rebinding case. Control runs with the address the socket is about to
// connect to, so a name that resolved public a moment ago and private now is
// refused at the point of use rather than at the point of lookup. Calling
// Control directly is exactly what the dialer does.
func TestControlRefusesAtConnectTime(t *testing.T) {
	dialer := Dialer(time.Second)

	if err := dialer.Control("tcp4", "93.184.215.14:443", nil); err != nil {
		t.Fatalf("public address refused at connect time: %v", err)
	}

	// The second resolution comes back private, which is the whole attack.
	if err := dialer.Control("tcp4", "169.254.169.254:443", nil); err == nil {
		t.Fatal("metadata address was allowed at connect time")
	}
	if err := dialer.Control("tcp6", "[::ffff:127.0.0.1]:443", nil); err == nil {
		t.Fatal("mapped loopback was allowed at connect time")
	}
	if err := dialer.Control("tcp4", "10.1.2.3:443", nil); err == nil {
		t.Fatal("private address was allowed at connect time")
	}
	// The port allowlist applies on the same seam.
	if err := dialer.Control("tcp4", "93.184.215.14:22", nil); err == nil {
		t.Fatal("port 22 was allowed through the guarded dialer")
	}
}

// PORTS waives the port allowlist and nothing else.
func TestPortScanDialerStillGuardsAddresses(t *testing.T) {
	dialer := PortScanDialer(time.Second)

	if err := dialer.Control("tcp4", "93.184.215.14:22", nil); err != nil {
		t.Fatalf("port 22 refused on the scan dialer: %v", err)
	}
	for _, target := range []string{"127.0.0.1:22", "169.254.169.254:80", "10.0.0.1:3306"} {
		if err := dialer.Control("tcp4", target, nil); err == nil {
			t.Errorf("%s was allowed on the scan dialer", target)
		}
	}
}

// A real dial to a refused address must fail before any packet leaves.
func TestDialerRefusesLoopbackForReal(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen locally: %v", err)
	}
	defer listener.Close()

	conn, err := PortScanDialer(2*time.Second).Dial("tcp", listener.Addr().String())
	if err == nil {
		conn.Close()
		t.Fatal("dialed a loopback listener through the guarded dialer")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}

func TestBlockedErrorNamesTargetAndReason(t *testing.T) {
	err := Check(netip.MustParseAddr("169.254.169.254"))
	if err == nil {
		t.Fatal("expected refusal")
	}
	var blocked *Blocked
	if !errorsAs(err, &blocked) {
		t.Fatalf("expected *Blocked, got %T", err)
	}
	if blocked.Target == "" || blocked.Reason == "" {
		t.Fatalf("refusal did not name target and reason: %+v", blocked)
	}
}

func errorsAs(err error, target **Blocked) bool {
	if b, ok := err.(*Blocked); ok {
		*target = b
		return true
	}
	return false
}
