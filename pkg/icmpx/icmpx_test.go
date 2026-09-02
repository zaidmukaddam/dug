package icmpx

import (
	"context"
	"net"
	"testing"
	"time"
)

// A cancelled context must stop Ping before it ever waits on a socket, so a
// disconnected client's request does not keep burning function time.
func TestPingStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	replies := Ping(ctx, net.IPv4(127, 0, 0, 1), 3, time.Second)
	elapsed := time.Since(started)

	if elapsed >= 200*time.Millisecond {
		t.Fatalf("Ping took %v with an already-cancelled context, want < 200ms", elapsed)
	}
	for _, reply := range replies {
		if reply.Answered() {
			t.Fatalf("got an answered reply from a cancelled context: %+v", reply)
		}
	}
}
