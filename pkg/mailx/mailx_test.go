package mailx

import (
	"context"
	"testing"
)

// The lookup counter is the part of the MAIL screen most likely to be silently
// wrong, and no major sender exceeds the limit in the wild because exceeding it
// breaks their own mail. So it is tested directly rather than against a live
// broken third party.
func TestLookupCounting(t *testing.T) {
	record := "v=spf1"
	for i := 0; i < 12; i++ {
		record += " include:host" + string(rune('a'+i)) + ".example"
	}
	record += " -all"

	querying := 0
	for _, term := range Terms(record) {
		if Querying[term.Mechanism] {
			querying++
		}
	}
	if querying != 12 {
		t.Errorf("counted %d querying mechanisms, want 12", querying)
	}
	if querying <= LookupLimit {
		t.Errorf("12 mechanisms should exceed the limit of %d", LookupLimit)
	}
}

// Past the limit the walk must stop querying, not merely say it is over. A
// domain whose every subdomain answers with ten fresh includes would otherwise
// turn one request into millions of queries. Both guards are checked before
// any DNS is touched, so this test never reaches the network.
func TestExpandStopsQueryingPastTheLimit(t *testing.T) {
	over := NewWalk("1.1.1.1")
	over.Lookups = LookupLimit + 2
	node := over.Expand(context.Background(), "over.example", 0)

	if over.Queries != 0 {
		t.Errorf("issued %d queries past the lookup limit, want 0", over.Queries)
	}
	if len(node.Children) != 0 {
		t.Errorf("expanded %d children past the lookup limit, want 0", len(node.Children))
	}
	if over.Domains() != 0 {
		t.Errorf("counted %d domains expanded past the limit, want 0", over.Domains())
	}
	if node.Meta == "" {
		t.Error("a node stopped by the limit says nothing about why")
	}
	if over.Lookups != LookupLimit+2 {
		t.Errorf("Lookups = %d, the count must survive for the punch list", over.Lookups)
	}

	ceiling := NewWalk("1.1.1.1")
	ceiling.Queries = QueryCeiling
	ceiling.Expand(context.Background(), "ceiling.example", 0)
	if ceiling.Queries != QueryCeiling {
		t.Errorf("Queries = %d, the ceiling did not stop the walk", ceiling.Queries)
	}
}

// ip4 and ip6 resolve nothing, so counting them would overstate every record.
func TestAddressMechanismsCostNoLookup(t *testing.T) {
	for _, term := range Terms("v=spf1 ip4:1.2.3.0/24 ip6:2001:db8::/32 -all") {
		if Querying[term.Mechanism] {
			t.Errorf("%s was counted as a dns lookup", term.Mechanism)
		}
	}
}

// The terminal qualifier is the last `all`, not the first mechanism.
func TestTerminalReadFromTheEnd(t *testing.T) {
	cases := map[string]string{
		"v=spf1 include:a.example -all": "-",
		"v=spf1 include:a.example ~all": "~",
		"v=spf1 include:a.example ?all": "?",
		"v=spf1 include:a.example":      "?",
	}
	for record, want := range cases {
		if got := Terminal(record); got != want {
			t.Errorf("Terminal(%q) = %q, want %q", record, got, want)
		}
	}
}

func TestParseDMARCTags(t *testing.T) {
	tags := ParseDMARC([]string{"v=DMARC1; p=reject; sp=quarantine; aspf=s; rua=mailto:a@b.com"})
	for key, want := range map[string]string{
		"p": "reject", "sp": "quarantine", "aspf": "s", "rua": "mailto:a@b.com",
	} {
		if tags[key] != want {
			t.Errorf("tag %s = %q, want %q", key, tags[key], want)
		}
	}
	if len(ParseDMARC([]string{"v=spf1 -all"})) != 0 {
		t.Error("an spf record was parsed as dmarc")
	}
}

func TestParseSPFPicksTheSPFRecord(t *testing.T) {
	records := []string{"google-site-verification=abc", "v=spf1 include:x.example ~all", "other"}
	if got := ParseSPF(records); got != "v=spf1 include:x.example ~all" {
		t.Errorf("ParseSPF = %q", got)
	}
	if ParseSPF([]string{"nothing here"}) != "" {
		t.Error("found an spf record where there is none")
	}
}
