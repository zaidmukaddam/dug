package certs

import "testing"

// Every chain a real host serves has two or more intermediates, so the
// single-intermediate case cannot be reached from a live handshake. It is the
// branch that decides whether a number appears at all, so it is checked here.
func TestLabelChain(t *testing.T) {
	roles := func(in ...string) []Cert {
		out := make([]Cert, 0, len(in))
		for _, role := range in {
			out = append(out, Cert{Role: role, Label: role})
		}
		return out
	}

	labels := func(chain []Cert) []string {
		out := make([]string, 0, len(chain))
		for _, cert := range chain {
			out = append(out, cert.Label)
		}
		return out
	}

	cases := []struct {
		name  string
		chain []Cert
		want  []string
	}{
		{
			name:  "one intermediate is not numbered",
			chain: roles("leaf", "intermediate", "root"),
			want:  []string{"leaf", "intermediate", "root"},
		},
		{
			name:  "two intermediates are numbered from the leaf",
			chain: roles("leaf", "intermediate", "intermediate"),
			want:  []string{"leaf", "intermediate 1", "intermediate 2"},
		},
		{
			name:  "numbering skips the leaf and the root",
			chain: roles("leaf", "intermediate", "intermediate", "intermediate", "root"),
			want:  []string{"leaf", "intermediate 1", "intermediate 2", "intermediate 3", "root"},
		},
		{
			name:  "a leaf on its own is untouched",
			chain: roles("leaf"),
			want:  []string{"leaf"},
		},
		{
			name:  "an empty chain does not panic",
			chain: nil,
			want:  []string{},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			labelChain(test.chain)
			got := labels(test.chain)
			if len(got) != len(test.want) {
				t.Fatalf("got %v, want %v", got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Errorf("position %d: got %q, want %q", i, got[i], test.want[i])
				}
			}
		})
	}
}

// The screen renders "name matches" from this, so anything it accepts that a
// TLS client would reject reads as a certificate that is fine when it is not.
func TestCovers(t *testing.T) {
	cases := []struct {
		name    string
		sans    []string
		subject string
		host    string
		want    bool
	}{
		{name: "exact", sans: []string{"example.com"}, host: "example.com", want: true},
		{name: "case differs", sans: []string{"Example.COM"}, host: "example.com", want: true},
		{name: "wildcard covers one label", sans: []string{"*.example.com"}, host: "a.example.com", want: true},
		{name: "wildcard case differs", sans: []string{"*.Example.com"}, host: "A.example.com", want: true},
		{name: "wildcard does not cover two labels", sans: []string{"*.example.com"}, host: "a.b.example.com"},
		{name: "wildcard does not cover the apex", sans: []string{"*.example.com"}, host: "example.com"},
		{name: "partial label is not a wildcard", sans: []string{"fo*.example.com"}, host: "foo.example.com"},
		{name: "wildcard is not a bare suffix", sans: []string{"*.example.com"}, host: "notexample.com"},
		{name: "the subject is considered too", subject: "example.com", host: "example.com", want: true},
		{name: "no name matches", sans: []string{"other.com"}, host: "example.com"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			leaf := Cert{SANs: test.sans, Subject: test.subject}
			if got := Covers(leaf, test.host); got != test.want {
				t.Errorf("Covers(%v, %q) = %v, want %v", append(test.sans, test.subject), test.host, got, test.want)
			}
		})
	}
}
