package dnsx

// The guard on the addresses a walk is handed is the one thing standing between
// this package and an SSRF: a zone's own nameserver chooses the next hop, and it
// can name anything. TestNextServerGuardsGlue covers the decision in isolation;
// this drives the whole of Walk against a server that actually tries it, so
// re-inlining an unguarded assignment into the loop cannot pass silently.

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/zaidmukaddam/dug/pkg/resolvers"
)

// hostileRoot answers every question with a referral whose glue points at the
// cloud metadata address, which is what a real attack looks like.
func hostileRoot(t *testing.T, glue string) string {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}

	server := &dns.Server{PacketConn: conn}
	server.Handler = dns.HandlerFunc(func(w dns.ResponseWriter, query *dns.Msg) {
		reply := new(dns.Msg)
		reply.SetReply(query)
		zone := query.Question[0].Name

		reply.Ns = append(reply.Ns, &dns.NS{
			Hdr: dns.RR_Header{Name: zone, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 300},
			Ns:  "ns1.attacker.example.",
		})
		reply.Extra = append(reply.Extra, &dns.A{
			Hdr: dns.RR_Header{Name: "ns1.attacker.example.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
			A:   net.ParseIP(glue),
		})
		_ = w.WriteMsg(reply)
	})

	go func() { _ = server.ActivateAndServe() }()
	t.Cleanup(func() { _ = server.Shutdown() })

	_, port, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		t.Fatalf("splitting %q: %v", conn.LocalAddr(), err)
	}
	return port
}

func TestWalkRefusesGlueItIsHanded(t *testing.T) {
	const metadata = "169.254.169.254"

	port := hostileRoot(t, metadata)

	// The walk starts at the fake server; only the addresses it learns from the
	// reply are the guard's business, which is exactly what is under test.
	previousRoot, previousPort := walkRoot, walkPort
	walkRoot = resolvers.RootServer{Name: "fake.root", IP: "127.0.0.1"}
	walkPort = port
	t.Cleanup(func() { walkRoot, walkPort = previousRoot, previousPort })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hops := Walk(ctx, "example.com")

	if len(hops) == 0 {
		t.Fatal("the walk produced no hops, so nothing was exercised")
	}

	for _, hop := range hops {
		if hop.ServerIP == metadata {
			t.Fatalf("walk queried %s, the guard did not stop it", metadata)
		}
	}

	last := hops[len(hops)-1]
	if !strings.Contains(last.Err, "refused") {
		t.Errorf("the walk did not record the refusal: hops=%d last.Err=%q", len(hops), last.Err)
	}

	// A refusal has to stop the walk rather than let it re-query the previous
	// server for every remaining label.
	if len(hops) > 2 {
		t.Errorf("walk kept going after a refusal: %d hops", len(hops))
	}
}
