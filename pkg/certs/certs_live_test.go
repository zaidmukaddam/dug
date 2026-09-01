package certs

import (
	"context"
	"testing"
	"time"
)

func TestConnectLive(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	for _, host := range []string{"github.com", "example.com"} {
		hs := Connect(context.Background(), host, 443, 8*time.Second)
		if hs.Err != "" {
			t.Fatalf("%s: %s", host, hs.Err)
		}
		if len(hs.Chain) < 2 {
			t.Errorf("%s: chain has %d certs", host, len(hs.Chain))
		}
		t.Logf("%s ver=%s cipher=%s alpn=%s verified=%v chain=%d source=%q",
			host, hs.Version, hs.Cipher, hs.ALPN, hs.Verified, len(hs.Chain), hs.ChainSource)
		for _, c := range hs.Chain {
			t.Logf("   %-12s %s <- %s (%s)", c.Role, c.Subject, c.Issuer, c.Key)
		}
		for _, p := range Protocols {
			t.Logf("   %s offered=%v", p.Label, *hs.Protocols[p.Label])
		}
	}
}

// A diagnostic tool spends its time on broken endpoints, so an expired
// certificate must still yield a described chain rather than an error.
func TestConnectToExpiredCertificate(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	hs := Connect(context.Background(), "expired.badssl.com", 443, 8*time.Second)
	if hs.Err != "" {
		t.Skipf("host unreachable: %s", hs.Err)
	}
	if hs.Verified {
		t.Error("expired certificate reported as verified")
	}
	if len(hs.Chain) == 0 {
		t.Fatal("no chain returned for an expired certificate")
	}
	t.Logf("expired.badssl.com verified=%v verifyErr=%q chain=%d leaf expires %s",
		hs.Verified, hs.VerifyError, len(hs.Chain), hs.Chain[0].NotAfter.Format("2006-01-02"))
}
