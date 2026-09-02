// Package icmpx sends ICMP echo over unprivileged datagram sockets.
//
// The premise that ICMP needs CAP_NET_RAW is only true of raw sockets. Linux
// and macOS both expose ICMP over SOCK_DGRAM, which needs no capability, and
// x/net/icmp speaks it directly. Availability is still reported as a value
// rather than assumed, because a sandbox can refuse the socket.
package icmpx

import (
	"context"
	"errors"
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

type Reply struct {
	Source string
	RTT    time.Duration
	Kind   string // echo, time_exceeded, unreachable, timeout, error
	Detail string
}

func (r Reply) Answered() bool {
	return r.Kind == "echo" || r.Kind == "time_exceeded" || r.Kind == "unreachable"
}

func network(v6 bool) (string, int) {
	if v6 {
		return "udp6", ipv6.ICMPTypeEchoRequest.Protocol()
	}
	return "udp4", ipv4.ICMPTypeEcho.Protocol()
}

// Available reports whether an unprivileged ICMP socket can be opened here.
func Available(v6 bool) (bool, string) {
	proto, _ := network(v6)
	listen := "0.0.0.0"
	if v6 {
		listen = "::"
	}
	conn, err := icmp.ListenPacket(proto, listen)
	if err != nil {
		return false, err.Error()
	}
	conn.Close()
	return true, "unprivileged icmp datagram socket"
}

// Probe sends one echo request and waits for the first message about it. A low
// ttl is what makes traceroute work: the probe expires in transit and the
// router that dropped it announces itself with a time exceeded.
func Probe(ctx context.Context, target net.IP, sequence, ttl int, timeout time.Duration) Reply {
	if err := ctx.Err(); err != nil {
		return Reply{Kind: "error", Detail: err.Error()}
	}

	v6 := target.To4() == nil
	proto, protoNum := network(v6)
	listen := "0.0.0.0"
	if v6 {
		listen = "::"
	}

	conn, err := icmp.ListenPacket(proto, listen)
	if err != nil {
		return Reply{Kind: "error", Detail: err.Error()}
	}
	defer conn.Close()

	if ttl > 0 {
		if v6 {
			_ = conn.IPv6PacketConn().SetHopLimit(ttl)
		} else {
			_ = conn.IPv4PacketConn().SetTTL(ttl)
		}
	}

	var messageType icmp.Type = ipv4.ICMPTypeEcho
	if v6 {
		messageType = ipv6.ICMPTypeEchoRequest
	}

	body := &icmp.Echo{
		// The kernel owns the identifier on this socket type, so replies are
		// matched on the sequence number.
		ID:   os.Getpid() & 0xffff,
		Seq:  sequence,
		Data: make([]byte, 32),
	}
	message := icmp.Message{Type: messageType, Code: 0, Body: body}
	encoded, err := message.Marshal(nil)
	if err != nil {
		return Reply{Kind: "error", Detail: err.Error()}
	}

	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)

	started := time.Now()
	if _, err := conn.WriteTo(encoded, &net.UDPAddr{IP: target}); err != nil {
		return Reply{Kind: "error", Detail: err.Error()}
	}

	buffer := make([]byte, 1500)
	for {
		if time.Now().After(deadline) {
			return Reply{Kind: "timeout"}
		}

		count, peer, err := conn.ReadFrom(buffer)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				return Reply{Kind: "timeout"}
			}
			return Reply{Kind: "error", Detail: err.Error()}
		}
		elapsed := time.Since(started)

		parsed, err := icmp.ParseMessage(protoNum, buffer[:count])
		if err != nil {
			continue
		}

		source := peer.String()
		if addr, ok := peer.(*net.UDPAddr); ok {
			source = addr.IP.String()
		}

		switch parsed.Type {
		case ipv4.ICMPTypeEchoReply, ipv6.ICMPTypeEchoReply:
			if echo, ok := parsed.Body.(*icmp.Echo); ok && echo.Seq != sequence {
				continue // another probe's reply on this socket
			}
			return Reply{Source: source, RTT: elapsed, Kind: "echo"}
		case ipv4.ICMPTypeTimeExceeded, ipv6.ICMPTypeTimeExceeded:
			return Reply{Source: source, RTT: elapsed, Kind: "time_exceeded"}
		case ipv4.ICMPTypeDestinationUnreachable, ipv6.ICMPTypeDestinationUnreachable:
			return Reply{Source: source, RTT: elapsed, Kind: "unreachable"}
		}
	}
}

// Ping sends sequential echo requests, so the round trip times are independent
// samples rather than a burst competing with itself.
func Ping(ctx context.Context, target net.IP, count int, timeout time.Duration) []Reply {
	replies := make([]Reply, 0, count)
	for i := 0; i < count; i++ {
		if ctx.Err() != nil {
			return replies
		}
		replies = append(replies, Probe(ctx, target, (os.Getpid()+i)&0xffff, 0, timeout))
		if i+1 < count {
			time.Sleep(120 * time.Millisecond)
		}
	}
	return replies
}

type Hop struct {
	TTL    int
	Source string
	RTT    time.Duration
	Kind   string
}

// Traceroute walks the TTL up until the destination answers or the ceiling is
// hit. Each hop keeps its fastest answering probe, because a single lost packet
// is normal and should not render the hop unreachable.
func Traceroute(ctx context.Context, target net.IP, maxHops int, timeout time.Duration, probesPerHop int) []Hop {
	hops := make([]Hop, 0, maxHops)

	for ttl := 1; ttl <= maxHops; ttl++ {
		if ctx.Err() != nil {
			return hops
		}
		best := Hop{TTL: ttl, Kind: "timeout"}

		for attempt := 0; attempt < probesPerHop; attempt++ {
			reply := Probe(ctx, target, (os.Getpid()+ttl*16+attempt)&0xffff, ttl, timeout)
			if reply.Answered() && (best.Source == "" || reply.RTT < best.RTT) {
				best = Hop{TTL: ttl, Source: reply.Source, RTT: reply.RTT, Kind: reply.Kind}
			}
		}

		hops = append(hops, best)
		if best.Kind == "echo" || best.Kind == "unreachable" {
			break
		}
	}
	return hops
}
