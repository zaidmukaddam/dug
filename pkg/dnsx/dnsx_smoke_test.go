package dnsx

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/miekg/dns"
)

// Network-touching smoke test. Skipped with -short.
func TestQueryLive(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	ctx := context.Background()

	a := Query(ctx, "example.com", "A", DefaultResolverIP())
	if a.Err != "" || len(a.Records) == 0 {
		t.Fatalf("A lookup failed: %+v", a)
	}
	t.Logf("A=%v ttl=%d ms=%d", a.Records, a.TTL, a.MS)

	mx := Query(ctx, "github.com", "MX", DefaultResolverIP())
	if len(mx.Records) == 0 {
		t.Errorf("no MX for github.com: %+v", mx)
	}
	t.Logf("MX=%v", mx.Records)

	txt := Query(ctx, "github.com", "TXT", DefaultResolverIP())
	if len(TXTStrings(txt)) == 0 {
		t.Errorf("no TXT for github.com")
	}

	key := Query(ctx, "cloudflare.com", "DNSKEY", DefaultResolverIP())
	if len(key.Records) == 0 {
		t.Errorf("no DNSKEY for cloudflare.com, DO bit may not be set")
	}
	t.Logf("DNSKEY=%d ad=%v", len(key.Records), key.Authenticated)
}

func TestIDNAndZoneCuts(t *testing.T) {
	unicode, ascii := DisplayForms("bücher.de")
	if ascii != "xn--bcher-kva.de" || unicode != "bücher.de" {
		t.Fatalf("idna round trip wrong: %q %q", unicode, ascii)
	}
	cuts := ZoneCuts("www.example.co.uk")
	want := []string{".", "uk", "co.uk", "example.co.uk", "www.example.co.uk"}
	if len(cuts) != len(want) {
		t.Fatalf("zone cuts = %v", cuts)
	}
	for i := range want {
		if cuts[i] != want[i] {
			t.Fatalf("zone cuts = %v, want %v", cuts, want)
		}
	}
}

func TestToName(t *testing.T) {
	// The underscore names are the ones this tool is asked about most.
	for _, name := range []string{"_dmarc.example.com", "_acme-challenge.example.com", "selector1._domainkey.example.com"} {
		got, err := ToName(name)
		if err != nil || got != name {
			t.Errorf("ToName(%q) = %q, %v", name, got, err)
		}
	}

	got, err := ToName("bücher.de")
	if err != nil || got != "xn--bcher-kva.de" {
		t.Errorf("ToName unicode = %q, %v", got, err)
	}

	if got, err := ToName(strings.Repeat("a.", 130) + "example.com"); err == nil {
		t.Errorf("over-long name accepted: %q", got)
	}
}

func glueReply(address string) *dns.Msg {
	reply := new(dns.Msg)
	reply.Extra = []dns.RR{&dns.A{
		Hdr: dns.RR_Header{Name: "ns1.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET},
		A:   net.ParseIP(address),
	}}
	return reply
}

func TestNextServerGuardsGlue(t *testing.T) {
	ctx := context.Background()

	for _, address := range []string{"127.0.0.1", "192.168.1.1", "169.254.169.254"} {
		ip, _, err := nextServer(ctx, glueReply(address), nil)
		if err == nil || ip != "" {
			t.Errorf("followed glue %s: ip=%q err=%v", address, ip, err)
		}
	}

	ip, name, err := nextServer(ctx, glueReply("198.41.0.4"), nil)
	if err != nil || ip != "198.41.0.4" || name != "ns1.example.com" {
		t.Fatalf("public glue not followed: ip=%q name=%q err=%v", ip, name, err)
	}
}
