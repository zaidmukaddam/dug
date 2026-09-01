// IP, ASN and NET. Reverse DNS, autonomous systems, and address space.
//
// Team Cymru answers over DNS, so the AS lookups reuse the same resolver path
// as everything else.
//
// NET does not probe. Establishing liveness means connecting to 256
// third-party addresses from shared egress, so the grid shows which addresses
// have reverse DNS instead, which comes from our own resolver.
package handler

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"github.com/zaidmukaddam/dug/pkg/dnsx"
	"github.com/zaidmukaddam/dug/pkg/guard"
	"github.com/zaidmukaddam/dug/pkg/resolvers"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

// A /24 is 256 lookups. Anything larger is refused rather than fanned out.
const maxNetHosts = 256

func Handler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	target := strings.TrimSpace(query.Get("target"))
	command := query.Get("command")
	if command == "" {
		command = "IP"
	}
	if target == "" {
		screen.Fail(w, r, command, "", "no address given", "this command needs an address")
		return
	}

	result := screen.New(command, target)
	switch command {
	case "ASN":
		runASN(r, result, target)
	case "NET":
		runNet(r, result, target)
	default:
		runIP(r, result, target)
	}
	result.Write(w, r)
}

type cymru struct {
	ASN       string
	Prefix    string
	Country   string
	Registry  string
	Allocated string
	Name      string
}

func cymruFields(answer dnsx.Answer) []string {
	strs := dnsx.TXTStrings(answer)
	if len(strs) == 0 {
		return nil
	}
	parts := strings.Split(strs[0], "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func cymruOrigin(ctx context.Context, ip string) cymru {
	reverse, err := dnsx.ReverseName(ip)
	if err != nil {
		return cymru{}
	}
	name := strings.NewReplacer(".in-addr.arpa", "", ".ip6.arpa", "").Replace(reverse)
	parts := cymruFields(dnsx.Query(ctx, name+".origin.asn.cymru.com", "TXT", resolvers.Default.IP))
	if len(parts) < 5 {
		return cymru{}
	}
	return cymru{ASN: parts[0], Prefix: parts[1], Country: parts[2], Registry: parts[3], Allocated: parts[4]}
}

func cymruASName(ctx context.Context, asn string) cymru {
	parts := cymruFields(dnsx.Query(ctx, "AS"+asn+".asn.cymru.com", "TXT", resolvers.Default.IP))
	if len(parts) < 5 {
		return cymru{}
	}
	return cymru{ASN: parts[0], Country: parts[1], Registry: parts[2], Allocated: parts[3], Name: parts[4]}
}

func cymruPeers(ctx context.Context, ip string) []string {
	reverse, err := dnsx.ReverseName(ip)
	if err != nil {
		return nil
	}
	name := strings.NewReplacer(".in-addr.arpa", "", ".ip6.arpa", "").Replace(reverse)
	parts := cymruFields(dnsx.Query(ctx, name+".peer.asn.cymru.com", "TXT", resolvers.Default.IP))
	if len(parts) == 0 {
		return nil
	}
	peers := strings.Fields(parts[0])
	sort.Strings(peers)
	return peers
}

// Describe fills a result with everything known about one address: reverse
// dns, origin as, prefix and neighbours.
//
// Exported for /me, which needs exactly this about the caller's own address.
// A second copy over there would be a second thing to keep correct, and the
// two would disagree the first time either changed.
func Describe(r *http.Request, result *screen.Result, address string) {
	runIP(r, result, address)
}

func runIP(r *http.Request, result *screen.Result, target string) {
	addr, err := netip.ParseAddr(target)
	if err != nil {
		screenRefuse(result, target, "not an ip address")
		return
	}
	if err := guard.Check(addr); err != nil {
		var blocked *guard.Blocked
		if b, ok := err.(*guard.Blocked); ok {
			blocked = b
		}
		reason := err.Error()
		if blocked != nil {
			reason = blocked.Reason
		}
		screenRefuse(result, addr.String(), reason)
		return
	}

	ctx := r.Context()
	ip := addr.String()
	result.Target = ip

	reverse, _ := dnsx.ReverseName(ip)
	ptr := dnsx.Query(ctx, reverse, "PTR", resolvers.Default.IP)
	origin := cymruOrigin(ctx, ip)
	peers := cymruPeers(ctx, ip)
	result.Spend(3)
	result.HoldTTL(0, "asn")

	asInfo := cymru{}
	if origin.ASN != "" {
		asInfo = cymruASName(ctx, origin.ASN)
		result.Spend(1)
	}

	rdns := "none"
	if len(ptr.Records) > 0 {
		rdns = strings.TrimSuffix(ptr.Records[0], ".")
	}
	asName := asInfo.Name
	if asName == "" {
		asName = "an unnamed network"
	}

	detail := "announced in " + orUnknown(origin.Prefix)
	if len(ptr.Records) > 0 {
		detail += ", reverse dns " + rdns
	} else {
		detail += ", no reverse dns"
	}
	state := "none"
	if origin.ASN != "" {
		state = "ok"
	}
	result.SetVerdict(state, ip+" belongs to "+asName, detail)

	asLabel := "none"
	if origin.ASN != "" {
		asLabel = "AS" + origin.ASN
	}
	result.Add("GraphKpi", screen.KpiProps{
		Title: "autonomous system", Value: asLabel,
		Label: orNotPublished(asInfo.Name), Hint: "prefix " + orUnknown(origin.Prefix),
		Data: []int{1, 1, 1},
	}, 1)

	version := "ipv4"
	if addr.Is6() && !addr.Is4In6() {
		version = "ipv6"
	}
	result.Add("GraphSpec", screen.SpecProps{Title: "address", Rows: []screen.SpecRow{
		{Label: "address", Value: ip, Accent: true},
		{Label: "version", Value: version},
		{Label: "reverse dns", Value: rdns},
		{Label: "prefix", Value: orUnknown(origin.Prefix)},
		{Label: "registry", Value: orUnknown(origin.Registry)},
		{Label: "country", Value: orUnknown(origin.Country)},
	}}, 1)

	peerCount := "-"
	if len(peers) > 0 {
		peerCount = strconv.Itoa(len(peers))
	}
	result.Add("GraphStat", screen.StatProps{Title: "allocation", Items: []screen.StatItem{
		{Value: orUnknown(origin.Registry), Label: "registry", Accent: true},
		{Value: orUnknown(origin.Country), Label: "country"},
		{Value: orDash(origin.Allocated), Label: "allocated"},
		{Value: peerCount, Label: "bgp neighbours"},
	}}, 2)

	peerRows := make([][]string, 0, 12)
	for i, peer := range peers {
		if i >= 12 {
			break
		}
		info := cymruASName(ctx, peer)
		result.Spend(1)
		peerRows = append(peerRows, []string{"AS" + peer, orNotPublished(info.Name)})
	}
	if len(peerRows) == 0 {
		peerRows = [][]string{{"-", "no peer data published for this prefix"}}
	}
	result.Add("GraphTable", screen.TableProps{
		Title: "neighbours", Headers: []string{"asn", "name"}, Rows: peerRows,
	}, 2)

	if len(ptr.Records) == 0 {
		result.Note("no reverse dns. that is common for cloud and cdn address space and is a finding rather than a failure.")
	}
	result.Note("asn and prefix come from team cymru's dns mapping. bgp neighbours are as seen by cymru's collectors, not by this tool, which doesn’t speak bgp.")
}

func runASN(r *http.Request, result *screen.Result, target string) {
	asn := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(target)), "AS")
	if _, err := strconv.Atoi(asn); err != nil {
		screenRefuse(result, target, "not an as number")
		return
	}

	ctx := r.Context()
	result.Target = "AS" + asn
	info := cymruASName(ctx, asn)
	result.Spend(1)
	result.HoldTTL(0, "asn")

	if info.Name == "" {
		result.SetVerdict("warn", "AS"+asn+" is not named in the registry",
			"team cymru returned no record for this as number")
	} else {
		result.SetVerdict("ok", "AS"+asn+" is "+info.Name,
			"allocated "+orUnknown(info.Allocated)+" under "+orUnknown(info.Registry))
	}

	result.Add("GraphKpi", screen.KpiProps{
		Title: "autonomous system", Value: "AS" + asn,
		Label: orNotPublished(info.Name), Hint: "registry " + orUnknown(info.Registry),
		Data: []int{1, 1, 1},
	}, 1)

	result.Add("GraphSpec", screen.SpecProps{Title: "autonomous system", Rows: []screen.SpecRow{
		{Label: "number", Value: "AS" + asn, Accent: true},
		{Label: "name", Value: orNotPublished(info.Name)},
		{Label: "registry", Value: orUnknown(info.Registry)},
		{Label: "country", Value: orUnknown(info.Country)},
		{Label: "allocated", Value: orUnknown(info.Allocated)},
	}}, 2)

	// The prefix list needs a routing table. Cymru's DNS interface maps an
	// address to its origin, not an origin to its addresses, so this screen
	// says what it does not have rather than implying an empty network.
	result.Add("GraphSpec", screen.SpecProps{Title: "prefixes", Rows: []screen.SpecRow{
		{Label: "announced prefixes", Value: "not listed here"},
		{Label: "reason", Value: "enumerating them needs a bgp table, and this tool carries none"},
		{Label: "what works", Value: "IP <address> reports the prefix and origin for one address"},
	}}, 2)

	result.Note("cymru's dns interface maps an address to its origin as, not an as to its prefixes. rather than show an empty table, this screen names the limit.")
}

func runNet(r *http.Request, result *screen.Result, target string) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(target))
	if err != nil {
		screenRefuse(result, target, "not a network in cidr form")
		return
	}
	prefix = prefix.Masked()

	hosts := hostCount(prefix)
	if hosts > maxNetHosts {
		screenRefuse(result, prefix.String(), fmt.Sprintf(
			"holds %d addresses and the grid is capped at %d, so use a /24 or smaller", hosts, maxNetHosts))
		return
	}
	if err := guard.Check(prefix.Addr()); err != nil {
		reason := err.Error()
		if blocked, ok := err.(*guard.Blocked); ok {
			reason = blocked.Reason
		}
		screenRefuse(result, prefix.String(), reason)
		return
	}

	ctx := r.Context()
	result.Target = prefix.String()
	result.Budget = maxNetHosts + 32

	addresses := make([]netip.Addr, 0, hosts)
	jobs := make([]dnsx.Job, 0, hosts)
	for addr := prefix.Addr(); prefix.Contains(addr); addr = addr.Next() {
		reverse, err := dnsx.ReverseName(addr.String())
		if err != nil {
			continue
		}
		addresses = append(addresses, addr)
		jobs = append(jobs, dnsx.Job{Name: reverse, Type: "PTR", Resolver: resolvers.Default.IP})
		if len(jobs) >= maxNetHosts {
			break
		}
	}

	answers := dnsx.QueryMany(ctx, jobs, 32)
	result.Spend(len(answers))
	result.HoldTTL(dnsx.MinTTL(answers), "dns")

	named := 0
	cells := [][]int{}
	columns := 16
	if len(answers) < columns {
		columns = len(answers)
	}
	row := []int{}
	namedRows := [][]string{}
	for i, answer := range answers {
		value := 0
		if len(answer.Records) > 0 {
			value = 4
			named++
			if len(namedRows) < 20 {
				namedRows = append(namedRows, []string{
					addresses[i].String(), strings.TrimSuffix(answer.Records[0], "."),
				})
			}
		}
		row = append(row, value)
		if columns > 0 && len(row) == columns {
			cells = append(cells, row)
			row = []int{}
		}
	}
	if len(row) > 0 {
		cells = append(cells, row)
	}

	origin := cymruOrigin(ctx, prefix.Addr().String())
	result.Spend(1)

	originText := "no origin as published"
	if origin.ASN != "" {
		originText = "announced by AS" + origin.ASN
	}
	state := "none"
	if named > 0 {
		state = "ok"
	}
	result.SetVerdict(state,
		fmt.Sprintf("%d of %d addresses in %s have reverse dns", named, len(answers), prefix),
		originText)

	result.Add("GraphCells", screen.CellsProps{
		Title: "reverse dns", Items: []screen.CellGrid{{Label: prefix.String(), Cells: cells}},
	}, 2)

	result.Add("GraphSpec", screen.SpecProps{Title: "network", Rows: []screen.SpecRow{
		{Label: "network", Value: prefix.String(), Accent: true},
		{Label: "addresses", Value: strconv.Itoa(len(answers))},
		{Label: "with reverse dns", Value: fmt.Sprintf("%d of %d", named, len(answers))},
		{Label: "announced prefix", Value: orUnknown(origin.Prefix)},
		{Label: "origin", Value: asOrUnknown(origin.ASN)},
		{Label: "registry", Value: orUnknown(origin.Registry)},
	}}, 1)

	share := 0.0
	if len(answers) > 0 {
		share = float64(named) / float64(len(answers))
	}
	result.Add("GraphWaffle", screen.WaffleProps{
		Title: "named share", Value: round4(share),
		Caption: fmt.Sprintf("%d of %d addresses resolve backwards", named, len(answers)),
	}, 1)

	if len(namedRows) == 0 {
		namedRows = [][]string{{"-", "no address in this range has reverse dns"}}
	}
	result.Add("GraphTable", screen.TableProps{
		Title: "named addresses", Headers: []string{"address", "name"}, Rows: namedRows,
	}, 2)

	result.Note("a filled cell means the address has reverse dns, not that it is live. nothing in this range was contacted. establishing liveness means connecting to 256 third-party addresses from shared egress, which is the probe laundering this tool is built not to do.")
}

func hostCount(prefix netip.Prefix) int {
	bits := prefix.Addr().BitLen() - prefix.Bits()
	if bits > 20 {
		return 1 << 20
	}
	count := new(big.Int).Lsh(big.NewInt(1), uint(bits))
	if !count.IsInt64() {
		return 1 << 20
	}
	return int(count.Int64())
}

func screenRefuse(result *screen.Result, target, reason string) {
	result.Degrade("guard", reason)
	result.SetVerdict("warn", target+" is not a permitted destination", reason)
	result.Add("GraphSpec", screen.SpecProps{Title: "refused", Rows: []screen.SpecRow{
		{Label: "target", Value: target, Accent: true},
		{Label: "reason", Value: reason},
		{Label: "policy", Value: "resolved addresses are validated, never the name"},
	}}, 3)
	result.HoldTTL(0, "asn")
}

func orUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func orNotPublished(value string) string {
	if value == "" {
		return "not published"
	}
	return value
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func asOrUnknown(asn string) string {
	if asn == "" {
		return "unknown"
	}
	return "AS" + asn
}

func round4(value float64) float64 {
	return float64(int(value*10000+0.5)) / 10000
}
