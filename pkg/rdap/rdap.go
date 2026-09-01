// Package rdap fetches registration data, with WHOIS as the ccTLD fallback.
//
// Four things from scope section 7 are handled here:
//
//   - Discovery goes through the IANA bootstrap registry. No endpoint is
//     hardcoded.
//   - Thin registries hand back a referral and the full record needs the second
//     hop. A record whose referral could not be followed is marked partial.
//   - ccTLDs are not covered by ICANN contracts and roughly forty percent have
//     no RDAP service, so port 43 stays live.
//   - 429 is expected on shared egress and lands as a degraded state.
//
// RDAP is a specified format, so it is decoded into real types rather than
// walked as generic maps.
package rdap

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/zaidmukaddam/dug/pkg/httpx"
)

const BootstrapURL = "https://data.iana.org/rdap/dns.json"

// Wire types, matching RFC 9083.

type bootstrapFile struct {
	Services [][][]string `json:"services"`
}

type Link struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
	Type string `json:"type"`
}

type Event struct {
	Action string `json:"eventAction"`
	Date   string `json:"eventDate"`
}

type Nameserver struct {
	LDHName string `json:"ldhName"`
}

type SecureDNS struct {
	DelegationSigned bool `json:"delegationSigned"`
	DSData           []struct {
		KeyTag int `json:"keyTag"`
	} `json:"dsData"`
}

type Entity struct {
	Handle     string   `json:"handle"`
	Roles      []string `json:"roles"`
	VCardArray []any    `json:"vcardArray"`
	Entities   []Entity `json:"entities"`
}

// Name pulls the formatted name out of the jCard, falling back to the handle.
func (e Entity) Name() string {
	if len(e.VCardArray) > 1 {
		if properties, ok := e.VCardArray[1].([]any); ok {
			for _, raw := range properties {
				property, ok := raw.([]any)
				if !ok || len(property) < 4 {
					continue
				}
				if key, ok := property[0].(string); ok && key == "fn" {
					if value, ok := property[3].(string); ok && value != "" {
						return value
					}
				}
			}
		}
	}
	return e.Handle
}

type Domain struct {
	ObjectClassName string       `json:"objectClassName"`
	Handle          string       `json:"handle"`
	LDHName         string       `json:"ldhName"`
	UnicodeName     string       `json:"unicodeName"`
	Status          []string     `json:"status"`
	Events          []Event      `json:"events"`
	Entities        []Entity     `json:"entities"`
	Nameservers     []Nameserver `json:"nameservers"`
	SecureDNS       *SecureDNS   `json:"secureDNS"`
	Links           []Link       `json:"links"`
	Port43          string       `json:"port43"`
}

var eventLabels = map[string]string{
	"registration":                 "registered",
	"expiration":                   "expires",
	"last changed":                 "last changed",
	"last update of rdap database": "record refreshed",
	"transfer":                     "transferred",
	"deletion":                     "deleted",
	"reregistration":               "re-registered",
	"last re-registration":         "last re-registration",
}

// Registration is one normalised view over whichever protocol answered.
type Registration struct {
	Protocol    string // rdap, whois, none
	Endpoint    string
	Name        string
	Handle      string
	Registrar   string
	Registrant  string
	Abuse       string
	Statuses    []string
	Nameservers []string
	Events      map[string]time.Time
	DNSSEC      bool
	Thin        bool
	Partial     string
}

var (
	bootstrapMap map[string][]string
	bootstrapAt  time.Time
	bootstrapMu  sync.Mutex
)

// Bootstrap loads the IANA registry, cached for the life of the instance and
// again at the edge by the response Cache-Control.
func Bootstrap(ctx context.Context) (map[string][]string, error) {
	bootstrapMu.Lock()
	cached := bootstrapMap
	fresh := cached != nil && time.Since(bootstrapAt) < 24*time.Hour
	bootstrapMu.Unlock()
	if fresh {
		return cached, nil
	}

	var file bootstrapFile
	if _, err := httpx.GetJSON(ctx, BootstrapURL, &file); err != nil {
		return nil, fmt.Errorf("iana bootstrap unavailable: %w", err)
	}

	mapping := map[string][]string{}
	for _, service := range file.Services {
		if len(service) != 2 {
			continue
		}
		for _, tld := range service[0] {
			urls := make([]string, 0, len(service[1]))
			for _, url := range service[1] {
				urls = append(urls, strings.TrimSuffix(url, "/"))
			}
			mapping[strings.ToLower(strings.Trim(tld, "."))] = urls
		}
	}

	bootstrapMu.Lock()
	bootstrapMap, bootstrapAt = mapping, time.Now()
	bootstrapMu.Unlock()
	return mapping, nil
}

// ServiceFor returns the RDAP base URLs for a name. Longest matching suffix
// wins, so a.b.example under .example still maps.
func ServiceFor(ctx context.Context, name string) ([]string, error) {
	mapping, err := Bootstrap(ctx)
	if err != nil {
		return nil, err
	}
	labels := strings.Split(name, ".")
	for i := 0; i < len(labels); i++ {
		suffix := strings.Join(labels[i:], ".")
		if urls, ok := mapping[suffix]; ok {
			return urls, nil
		}
	}
	return nil, fmt.Errorf("no rdap service published for .%s", labels[len(labels)-1])
}

// FetchRDAP gets the domain object, following one referral hop for thin
// registries and merging both responses.
func FetchRDAP(ctx context.Context, name string) (*Registration, error) {
	bases, err := ServiceFor(ctx, name)
	if err != nil {
		return nil, err
	}

	var domain *Domain
	var endpoint string
	var lastErr error

	for i, base := range bases {
		if i >= 2 {
			break
		}
		url := base + "/domain/" + name
		var candidate Domain
		response, err := httpx.GetJSON(ctx, url, &candidate)
		if err != nil {
			switch {
			case response != nil && response.Status == 429:
				lastErr = fmt.Errorf("%s returned 429, rate limited", base)
			case response != nil && response.Status == 404:
				lastErr = fmt.Errorf("%s has no record for %s", base, name)
			default:
				lastErr = fmt.Errorf("%s: %w", base, err)
			}
			continue
		}
		domain, endpoint, lastErr = &candidate, url, nil
		break
	}

	if domain == nil {
		if lastErr == nil {
			lastErr = fmt.Errorf("no rdap server answered")
		}
		return nil, lastErr
	}

	registration := fromDomain(domain, name, endpoint)

	// Thin registry: the registry answer carries a pointer to the registrar's
	// own RDAP server, and the full record only exists there.
	if referral := referralOf(domain, name); referral != "" {
		registration.Thin = true
		var merged Domain
		if _, err := httpx.GetJSON(ctx, referral, &merged); err == nil {
			second := fromDomain(&merged, name, referral)
			registration = mergeRegistration(registration, second)
			registration.Thin = true
			registration.Endpoint = endpoint
		} else {
			registration.Partial = "referral to " + referral + " did not answer, the record is partial"
		}
	}

	return &registration, nil
}

func fromDomain(domain *Domain, name, endpoint string) Registration {
	out := Registration{
		Protocol: "rdap",
		Endpoint: endpoint,
		Name:     firstNonEmpty(domain.LDHName, domain.UnicodeName, name),
		Handle:   domain.Handle,
		Statuses: domain.Status,
		Events:   map[string]time.Time{},
	}

	for _, entity := range domain.Entities {
		for _, role := range entity.Roles {
			switch strings.ToLower(role) {
			case "registrar":
				if out.Registrar == "" {
					out.Registrar = entity.Name()
				}
			case "registrant":
				if out.Registrant == "" {
					out.Registrant = entity.Name()
				}
			case "abuse":
				if out.Abuse == "" {
					out.Abuse = entity.Name()
				}
			}
		}
		// Abuse contacts are commonly nested under the registrar.
		for _, nested := range entity.Entities {
			for _, role := range nested.Roles {
				if strings.EqualFold(role, "abuse") && out.Abuse == "" {
					out.Abuse = nested.Name()
				}
			}
		}
	}

	for _, ns := range domain.Nameservers {
		if ns.LDHName != "" {
			out.Nameservers = append(out.Nameservers, strings.ToLower(strings.TrimSuffix(ns.LDHName, ".")))
		}
	}

	for _, event := range domain.Events {
		label, ok := eventLabels[strings.ToLower(event.Action)]
		if !ok {
			label = strings.ToLower(event.Action)
		}
		if stamp, err := parseDate(event.Date); err == nil {
			out.Events[label] = stamp
		}
	}

	if domain.SecureDNS != nil {
		out.DNSSEC = domain.SecureDNS.DelegationSigned
	}
	return out
}

func mergeRegistration(base, extra Registration) Registration {
	out := base
	if extra.Registrar != "" {
		out.Registrar = extra.Registrar
	}
	if extra.Registrant != "" {
		out.Registrant = extra.Registrant
	}
	if extra.Abuse != "" {
		out.Abuse = extra.Abuse
	}
	if len(extra.Nameservers) > 0 {
		out.Nameservers = extra.Nameservers
	}
	if len(extra.Statuses) > 0 {
		out.Statuses = extra.Statuses
	}
	for label, stamp := range extra.Events {
		out.Events[label] = stamp
	}
	if extra.DNSSEC {
		out.DNSSEC = true
	}
	return out
}

func referralOf(domain *Domain, name string) string {
	self := map[string]bool{}
	for _, link := range domain.Links {
		if link.Rel == "self" {
			self[strings.TrimSuffix(link.Href, "/")] = true
		}
	}
	for _, link := range domain.Links {
		if link.Rel != "related" {
			continue
		}
		href := strings.TrimSuffix(link.Href, "/")
		if href == "" || self[href] {
			continue
		}
		if strings.Contains(strings.ToLower(link.Type), "rdap") ||
			strings.HasSuffix(strings.ToLower(href), "/domain/"+strings.ToLower(name)) {
			return href
		}
	}
	return ""
}

// WHOIS fallback. Ask IANA which server is authoritative for the TLD, then ask
// that server, because a guessed hostname is wrong often enough on ccTLDs.

const ianaWhois = "whois.iana.org"

var whoisServerRE = regexp.MustCompile(`(?mi)^whois:\s*(\S+)`)

var whoisFields = []struct {
	label   string
	pattern *regexp.Regexp
}{
	{"registrar", regexp.MustCompile(`(?mi)^\s*(?:registrar|sponsoring registrar)\s*:\s*(.+)$`)},
	{"registered", regexp.MustCompile(`(?mi)^\s*(?:creation date|created|registered on|domain registration date|record created)\s*:\s*(.+)$`)},
	{"expires", regexp.MustCompile(`(?mi)^\s*(?:registry expiry date|expiry date|expires|paid-till|renewal date|expiration date)\s*:\s*(.+)$`)},
	{"last changed", regexp.MustCompile(`(?mi)^\s*(?:updated date|last modified|changed|last-update)\s*:\s*(.+)$`)},
}

var (
	whoisStatusRE = regexp.MustCompile(`(?mi)^\s*(?:domain )?status\s*:\s*(.+)$`)
	whoisNSRE     = regexp.MustCompile(`(?mi)^\s*(?:name server|nserver|nameserver)\s*:\s*(\S+)`)
)

func FetchWHOIS(ctx context.Context, name string) (*Registration, error) {
	labels := strings.Split(name, ".")
	tld := labels[len(labels)-1]

	referral, err := httpx.Whois43(ctx, ianaWhois, tld)
	if err != nil {
		return nil, fmt.Errorf("%s did not answer for .%s: %w", ianaWhois, tld, err)
	}
	match := whoisServerRE.FindStringSubmatch(referral)
	if match == nil {
		return nil, fmt.Errorf(".%s publishes no whois server either", tld)
	}
	server := match[1]

	body, err := httpx.Whois43(ctx, server, name)
	if err != nil {
		return nil, fmt.Errorf("%s did not answer: %w", server, err)
	}

	out := Registration{Protocol: "whois", Endpoint: server, Name: name, Events: map[string]time.Time{}}
	for _, field := range whoisFields {
		if found := field.pattern.FindStringSubmatch(body); found != nil {
			value := strings.TrimSpace(found[1])
			if field.label == "registrar" {
				out.Registrar = value
				continue
			}
			if stamp, err := parseDate(value); err == nil {
				out.Events[field.label] = stamp
			}
		}
	}

	for _, found := range whoisStatusRE.FindAllStringSubmatch(body, -1) {
		out.Statuses = append(out.Statuses, strings.TrimSpace(found[1]))
	}
	seen := map[string]bool{}
	for _, found := range whoisNSRE.FindAllStringSubmatch(body, -1) {
		ns := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(found[1]), "."))
		if ns != "" && !seen[ns] {
			seen[ns] = true
			out.Nameservers = append(out.Nameservers, ns)
		}
	}

	if out.Registrar == "" && len(out.Events) == 0 && len(out.Nameservers) == 0 {
		head := strings.TrimSpace(body)
		if i := strings.IndexByte(head, '\n'); i > 0 {
			head = head[:i]
		}
		return nil, fmt.Errorf("whois answered but nothing parsed: %s", head)
	}
	return &out, nil
}

// Load tries RDAP first and port 43 second. The caller reports which answered.
func Load(ctx context.Context, name string) (Registration, []string) {
	var problems []string

	registration, err := FetchRDAP(ctx, name)
	if err == nil {
		if registration.Partial != "" {
			problems = append(problems, registration.Partial)
		}
		return *registration, problems
	}
	problems = append(problems, err.Error())

	fallback, whoisErr := FetchWHOIS(ctx, name)
	if whoisErr != nil {
		problems = append(problems, whoisErr.Error())
		return Registration{Protocol: "none", Name: name, Events: map[string]time.Time{}}, problems
	}
	return *fallback, problems
}

var dateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05Z0700",
	"2006-01-02 15:04:05",
	"2006-01-02",
	"02-Jan-2006",
	"2006.01.02",
	"20060102",
}

func parseDate(value string) (time.Time, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	for _, layout := range dateLayouts {
		if stamp, err := time.Parse(layout, text); err == nil {
			return stamp.UTC(), nil
		}
	}
	if len(text) >= 19 {
		if stamp, err := time.Parse("2006-01-02 15:04:05", text[:19]); err == nil {
			return stamp.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable date %q", value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
