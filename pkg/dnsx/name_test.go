package dnsx

import "testing"

// ToName is the gate every domain command runs its target through, and it used
// to be a gate with no lock on it. idna runs with STD3 rules off so the
// underscore names below survive, and the side effect was that a slash, a colon
// and a space survived too. "DIG http://example.com" was answered "the name
// does not exist", which is a confident lie about a domain that plainly does.
func TestToNameRefusesWhatIsNotAName(t *testing.T) {
	for _, test := range []struct{ input, wants string }{
		{"http://example.com", "try example.com"},
		{"https://www.google.com/travel/flights", "try www.google.com"},
		{"example.com/path", "try example.com"},
		{"example.com:8080", "try example.com"},
		{"../etc", "letters, digits, hyphens and dots"},
		{"exa mple.com", "letters, digits, hyphens and dots"},
		{"", "empty name"},
	} {
		name, err := ToName(test.input)
		if err == nil {
			t.Errorf("ToName(%q) returned %q and no error", test.input, name)
			continue
		}
		if !contains(err.Error(), test.wants) {
			t.Errorf("ToName(%q) said %q, wanted it to mention %q", test.input, err, test.wants)
		}
	}
}

// The whole reason STD3 is off. Refusing these would break the records this
// tool is most often asked about.
func TestToNameKeepsTheNamesThatMatter(t *testing.T) {
	for _, input := range []string{
		"example.com",
		"_dmarc.example.com",
		"selector1._domainkey.example.com",
		"_acme-challenge.example.com",
		"xn--bcher-kva.example",
		"münchen.de",
		"localhost",
		"1.1.1.1",
		"EXAMPLE.COM",
		"example.com.",
	} {
		if _, err := ToName(input); err != nil {
			t.Errorf("ToName(%q) refused a name it should take: %v", input, err)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
