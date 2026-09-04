// Package epp decodes registration status codes.
//
// RDAP publishes the spelled-out RFC 9083 form, WHOIS the camel-case EPP form;
// both map to the same entry. Locked marks the codes that bar a change, which
// is what the screen is actually read for.
package epp

import (
	"strings"
	"unicode"
)

type Status struct {
	Title   string
	Meaning string
	Locked  bool
}

var table = map[string]Status{
	"active":                     {"active", "no restrictions, the registry will honour requests", false},
	"inactive":                   {"inactive", "no nameservers delegated, the domain does not resolve", false},
	"ok":                         {"ok", "standard status, nothing pending or prohibited", false},
	"client delete prohibited":   {"client delete prohibited", "the registrar blocks deletion requests", true},
	"client hold":                {"client hold", "the registrar has pulled the domain from the zone, it does not resolve", true},
	"client renew prohibited":    {"client renew prohibited", "the registrar blocks renewal requests", true},
	"client transfer prohibited": {"client transfer prohibited", "the registrar blocks transfer to another registrar", true},
	"client update prohibited":   {"client update prohibited", "the registrar blocks changes to the record", true},
	"server delete prohibited":   {"server delete prohibited", "the registry blocks deletion, used for disputes and high value names", true},
	"server hold":                {"server hold", "the registry has pulled the domain from the zone, it does not resolve", true},
	"server renew prohibited":    {"server renew prohibited", "the registry blocks renewal", true},
	"server transfer prohibited": {"server transfer prohibited", "the registry blocks transfer, commonly for sixty days after registration", true},
	"server update prohibited":   {"server update prohibited", "the registry blocks changes to the record", true},
	"pending create":             {"pending create", "a create request is being processed", false},
	"pending delete":             {"pending delete", "scheduled for removal, five days before the name drops", true},
	"pending renew":              {"pending renew", "a renewal request is being processed", false},
	"pending restore":            {"pending restore", "a redemption request is being processed", false},
	"pending transfer":           {"pending transfer", "a transfer to another registrar is in progress", true},
	"pending update":             {"pending update", "an update request is being processed", false},
	"add period":                 {"add period", "within five days of registration, a delete is fully refunded", false},
	"auto renew period":          {"auto renew period", "within forty five days of an automatic renewal, still refundable", false},
	"renew period":               {"renew period", "within five days of a renewal, still refundable", false},
	"transfer period":            {"transfer period", "within five days of a transfer", false},
	"redemption period":          {"redemption period", "deleted but recoverable for thirty days, restoring costs a fee", true},
	"associated":                 {"associated", "linked to a registrant account", false},
	"validated":                  {"validated", "registrant identity has been verified", false},
	"locked":                     {"locked", "changes are barred", true},
	"proxy":                      {"proxy", "registered through a proxy service", false},
	"private":                    {"private", "registrant details are behind a privacy service", false},
	"removed":                    {"removed", "the object has been removed from the registry", true},
	"obscured":                   {"obscured", "some fields are deliberately masked", false},
}

// Normalize folds camelCase EPP spellings onto the RDAP spaced spelling.
func Normalize(code string) string {
	text := strings.TrimSpace(code)
	if text == "" {
		return ""
	}

	if !strings.ContainsAny(text, " -_") {
		var out strings.Builder
		for i, ch := range text {
			if unicode.IsUpper(ch) && i > 0 {
				out.WriteByte(' ')
			}
			out.WriteRune(unicode.ToLower(ch))
		}
		text = out.String()
	}

	text = strings.ReplaceAll(strings.ReplaceAll(text, "_", " "), "-", " ")
	return strings.Join(strings.Fields(strings.ToLower(text)), " ")
}

func Decode(code string) Status {
	// WHOIS often appends the ICANN explanatory URL to the code. Strip it
	// before normalising: leaving it on introduces a space, which suppresses
	// the camelCase split and leaves `clientTransferProhibited https://...`
	// undecodable.
	trimmed := code
	if head, _, found := strings.Cut(trimmed, " http"); found {
		trimmed = strings.TrimSpace(head)
	}

	key := Normalize(trimmed)
	if entry, ok := table[key]; ok {
		return entry
	}
	if key == "" {
		key = code
	}
	return Status{Title: key, Meaning: "not a recognised status code"}
}

// CheckItem is the shape the GraphCheck component takes.
type CheckItem struct {
	Label string `json:"label"`
	Done  bool   `json:"done"`
	Note  string `json:"note,omitempty"`
}

// CheckItems inverts the sense on purpose: an unmarked box is a restriction,
// which is what a reader scanning for what is barred wants to land on.
func CheckItems(codes []string) []CheckItem {
	if len(codes) == 0 {
		// Absence is rendered, not omitted.
		return []CheckItem{{
			Label: "none published",
			Done:  false,
			Note:  "the registry returned no status codes",
		}}
	}

	items := make([]CheckItem, 0, len(codes))
	for _, code := range codes {
		status := Decode(code)
		items = append(items, CheckItem{Label: status.Title, Done: !status.Locked, Note: status.Meaning})
	}
	return items
}

func LockedCount(codes []string) int {
	locked := 0
	for _, code := range codes {
		if Decode(code).Locked {
			locked++
		}
	}
	return locked
}
