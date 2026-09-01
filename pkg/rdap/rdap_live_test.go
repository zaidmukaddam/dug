package rdap

import (
	"context"
	"testing"
)

func TestLoadSpread(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	cases := []struct{ name, shape string }{
		{"example.com", "gtld thick"},
		{"github.com", "gtld thin, needs a referral hop"},
		{"afnic.fr", "cctld with rdap"},
		{"google.co.uk", "cctld with rdap"},
		{"nic.ir", "cctld, whois fallback"},
		{"nic.ch", "cctld, whois refuses this client"},
	}
	for _, tc := range cases {
		reg, problems := Load(context.Background(), tc.name)
		t.Logf("%-14s proto=%-5s thin=%-5v registrar=%-28q dates=%d ns=%d status=%d problems=%v",
			tc.name, reg.Protocol, reg.Thin, reg.Registrar, len(reg.Events), len(reg.Nameservers), len(reg.Statuses), problems)
		if reg.Protocol == "none" && len(problems) == 0 {
			t.Errorf("%s: no record and nothing reported as degraded", tc.name)
		}
	}
}
