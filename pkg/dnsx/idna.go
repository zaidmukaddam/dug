package dnsx

import "golang.org/x/net/idna"

// Lookup is the stricter profile and the right one for a name about to be put
// on the wire; Display is the lenient one for rendering it back.
//
// STD3 rules are off, because they forbid underscores and the underscore names
// are exactly what this tool is asked to look up: _dmarc, _acme-challenge,
// selector._domainkey. Mapping and the bidi rule stay on.
var (
	toASCII   = idna.New(idna.MapForLookup(), idna.StrictDomainName(false), idna.BidiRule(), idna.Transitional(false))
	toUnicode = idna.New(idna.ValidateForRegistration())
)

func idnaToASCII(name string) (string, error) {
	return toASCII.ToASCII(name)
}

func idnaToUnicode(name string) (string, error) {
	return toUnicode.ToUnicode(name)
}
