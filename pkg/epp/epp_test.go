package epp

import "testing"

// RDAP sends the spaced spelling and WHOIS the camel-case one; both have to
// land on the same entry or the same domain reads differently by protocol.
func TestBothSpellingsDecodeTheSame(t *testing.T) {
	pairs := [][2]string{
		{"clientTransferProhibited", "client transfer prohibited"},
		{"serverDeleteProhibited", "server delete prohibited"},
		{"redemptionPeriod", "redemption period"},
		{"pendingDelete", "pending delete"},
	}
	for _, pair := range pairs {
		camel, spaced := Decode(pair[0]), Decode(pair[1])
		if camel != spaced {
			t.Errorf("%q and %q decoded differently: %+v vs %+v", pair[0], pair[1], camel, spaced)
		}
		if !camel.Locked {
			t.Errorf("%q should be a locking status", pair[0])
		}
	}
}

func TestUnlockedStatuses(t *testing.T) {
	for _, code := range []string{"ok", "active", "pending renew", "add period"} {
		if Decode(code).Locked {
			t.Errorf("%q should not be a locking status", code)
		}
	}
}

func TestUnknownCodeIsNamedNotDropped(t *testing.T) {
	status := Decode("bogusCode")
	if status.Title != "bogus code" {
		t.Errorf("title = %q", status.Title)
	}
	if status.Meaning == "" {
		t.Error("an unrecognised code should still say so")
	}
}

// WHOIS commonly appends the ICANN explanatory URL to the code.
func TestTrailingURLIsIgnored(t *testing.T) {
	status := Decode("clientTransferProhibited https://icann.org/epp#clientTransferProhibited")
	if !status.Locked {
		t.Errorf("code with a trailing url did not decode: %+v", status)
	}
}

// Absence is rendered, not omitted.
func TestNoStatusesStillRendersARow(t *testing.T) {
	items := CheckItems(nil)
	if len(items) != 1 || items[0].Done {
		t.Fatalf("expected one unmarked row, got %+v", items)
	}
}

// Done means permitted, so a locking code reads as an unmarked box.
func TestCheckItemsInvertLockedSense(t *testing.T) {
	items := CheckItems([]string{"ok", "clientTransferProhibited"})
	if !items[0].Done {
		t.Error("ok should render as done")
	}
	if items[1].Done {
		t.Error("a prohibition should render as not done")
	}
	if LockedCount([]string{"ok", "clientTransferProhibited"}) != 1 {
		t.Error("LockedCount miscounted")
	}
}
