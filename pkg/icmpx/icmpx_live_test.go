package icmpx

import (
	"net"
	"testing"
	"time"
)

func TestPingAndTraceroute(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	ok, why := Available(false)
	t.Logf("icmp available=%v (%s)", ok, why)
	if !ok {
		t.Skip("no unprivileged icmp socket here")
	}

	target := net.ParseIP("1.1.1.1")
	replies := Ping(target, 4, 2*time.Second)
	answered := 0
	for _, r := range replies {
		if r.Kind == "echo" {
			answered++
		}
	}
	t.Logf("ping: %d/%d answered, first rtt %v", answered, len(replies), replies[0].RTT)
	if answered == 0 {
		t.Error("no echo replies from 1.1.1.1")
	}

	hops := Traceroute(target, 12, time.Second, 2)
	reached := false
	for _, h := range hops {
		t.Logf("  ttl %2d %-18s %-14s %v", h.TTL, orStar(h.Source), h.Kind, h.RTT.Truncate(time.Millisecond))
		if h.Kind == "echo" {
			reached = true
		}
	}
	if !reached {
		t.Error("traceroute never reached the destination")
	}
}

func orStar(s string) string {
	if s == "" {
		return "*"
	}
	return s
}
