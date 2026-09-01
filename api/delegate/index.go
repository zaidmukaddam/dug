// NS and DNSSEC. The delegation walk and the chain of trust.
package handler

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zaidmukaddam/dug/pkg/dnsx"
	"github.com/zaidmukaddam/dug/pkg/resolvers"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	target := query.Get("target")
	command := query.Get("command")
	if command == "" {
		command = "NS"
	}
	if target == "" {
		screen.Fail(w, r, command, "", "no domain given", "this command needs a domain name")
		return
	}
	name, err := dnsx.ToName(target)
	if err != nil {
		screen.Fail(w, r, command, target, target+" is not a domain name", err.Error())
		return
	}

	result := screen.New(command, name)
	if command == "DNSSEC" {
		runDNSSEC(r, result, name)
	} else {
		runNS(r, result, name)
	}
	result.Write(w, r)
}

func runNS(r *http.Request, result *screen.Result, name string) {
	hops := dnsx.Walk(r.Context(), name)
	result.Spend(len(hops))

	var authoritative *dnsx.Delegation
	var nameservers []string
	for i := len(hops) - 1; i >= 0; i-- {
		if hops[i].Authoritative && authoritative == nil {
			authoritative = &hops[i]
		}
		if len(hops[i].Nameservers) > 0 && nameservers == nil {
			nameservers = hops[i].Nameservers
		}
	}

	resolverNS := dnsx.Query(r.Context(), name, "NS", resolvers.Default.IP)
	result.Spend(1)
	result.HoldTTL(resolverNS.TTL, "dns")

	for _, hop := range hops {
		if hop.Err != "" {
			result.Degrade("walk "+hop.Zone, hop.Err)
		}
	}

	parentSet := map[string]bool{}
	for _, ns := range nameservers {
		parentSet[ns] = true
	}
	resolverSet := map[string]bool{}
	for _, record := range resolverNS.Records {
		resolverSet[strings.TrimSuffix(record, ".")] = true
	}
	consistent := len(parentSet) > 0 && sameSet(parentSet, resolverSet)

	switch {
	case len(nameservers) == 0:
		result.SetVerdict("none", name+" has no delegation at the parent", "the walk found no ns records")
	case consistent:
		result.SetVerdict("ok",
			fmt.Sprintf("%s is delegated to %d nameserver%s", name, len(nameservers), plural(len(nameservers))),
			"the parent and a public resolver agree on the list")
	default:
		result.SetVerdict("warn",
			fmt.Sprintf("%s is delegated to %d nameserver%s", name, len(nameservers), plural(len(nameservers))),
			"the parent and the resolver name different servers")
	}

	result.Add("GraphTree", screen.TreeProps{Title: "delegation", Nodes: treeFromWalk(hops)}, 2)

	authZone := "not reached"
	if authoritative != nil {
		authZone = authoritative.Zone
	}
	nsCount := "none"
	if len(nameservers) > 0 {
		nsCount = strconv.Itoa(len(nameservers))
	}
	result.Add("GraphSpec", screen.SpecProps{Title: "authority", Rows: []screen.SpecRow{
		{Label: "zone", Value: name, Accent: true},
		{Label: "authoritative", Value: authZone},
		{Label: "nameservers", Value: nsCount},
		{Label: "levels walked", Value: strconv.Itoa(len(hops))},
		{Label: "started at", Value: resolvers.Roots[0].Name},
	}}, 1)

	all := map[string]bool{}
	for ns := range parentSet {
		all[ns] = true
	}
	for ns := range resolverSet {
		all[ns] = true
	}
	names := make([]string, 0, len(all))
	for ns := range all {
		names = append(names, ns)
	}
	sort.Strings(names)

	rows := make([]screen.DiffRow, 0, len(names))
	for _, ns := range names {
		sign := "add"
		switch {
		case parentSet[ns] && resolverSet[ns]:
			sign = "keep"
		case parentSet[ns]:
			sign = "remove"
		}
		rows = append(rows, screen.DiffRow{Label: "ns", Value: ns, Sign: sign})
	}
	if len(rows) == 0 {
		rows = []screen.DiffRow{{Label: "ns", Value: "none published", Sign: "keep"}}
	}
	footerLabel := "mismatch"
	if consistent {
		footerLabel = "consistent"
	}
	result.Add("GraphDiff", screen.DiffProps{
		Title: "parent vs resolver",
		Rows:  rows,
		Footer: &screen.DiffRow{
			Label: footerLabel,
			Value: fmt.Sprintf("%d at parent, %d at resolver", len(parentSet), len(resolverSet)),
		},
	}, 1)

	walkRows := make([][]string, 0, len(hops))
	totalMS := 0
	for _, hop := range hops {
		kind := "referral"
		if hop.Err != "" {
			kind = "error"
		} else if hop.Authoritative {
			kind = "authoritative"
		}
		walkRows = append(walkRows, []string{hop.Zone, hop.Server, kind, strconv.Itoa(hop.MS)})
		totalMS += hop.MS
	}
	result.Add("GraphTable", screen.TableProps{
		Title: "walk", Headers: []string{"zone", "asked", "kind", "ms"},
		Align: []string{"left", "left", "left", "right"}, Rows: walkRows,
	}, 2)

	now := time.Now().UnixMilli()
	result.Add("GraphTimer", screen.TimerProps{
		Title: "walk time", Kind: "ago", At: &now,
		Caption: fmt.Sprintf("%dms across %d levels", totalMS, len(hops)),
	}, 1)

	if !consistent && len(parentSet) > 0 && len(resolverSet) > 0 {
		result.Note("the parent delegation and the resolver answer name different nameservers. during a nameserver change that resolves on its own; otherwise it’s a real inconsistency.")
	}
	result.Note("walked iteratively from " + resolvers.Roots[0].Name + ", so this is the delegation itself rather than one resolver's cached view of it.")
}

func treeFromWalk(hops []dnsx.Delegation) []screen.TreeNode {
	rootLabel := "no answer"
	if len(hops) > 0 {
		rootLabel = hops[0].Server
	}
	root := screen.TreeNode{Label: ".", Meta: "root, " + rootLabel, Accent: true}

	// Build from the deepest hop upward so each level nests inside its parent.
	var build func(index int) []screen.TreeNode
	build = func(index int) []screen.TreeNode {
		if index >= len(hops) {
			return nil
		}
		hop := hops[index]
		node := screen.TreeNode{Label: hop.Zone, Meta: hopMeta(hop), Accent: hop.Authoritative}
		for _, ns := range hop.Nameservers {
			node.Children = append(node.Children, screen.TreeNode{Label: ns, Meta: "nameserver"})
		}
		if len(hop.Nameservers) == 0 {
			node.Children = append(node.Children, screen.TreeNode{Label: "none", Meta: "no ns records at this level"})
		}
		node.Children = append(node.Children, build(index+1)...)
		return []screen.TreeNode{node}
	}

	root.Children = build(0)
	return []screen.TreeNode{root}
}

func hopMeta(hop dnsx.Delegation) string {
	if hop.Err != "" {
		return "error: " + hop.Err
	}
	kind := "referral"
	if hop.Authoritative {
		kind = "authoritative"
	}
	meta := strconv.Itoa(hop.MS) + "ms, " + kind
	if len(hop.DS) > 0 {
		meta += fmt.Sprintf(", %d ds", len(hop.DS))
	}
	return meta
}

func runDNSSEC(r *http.Request, result *screen.Result, name string) {
	ctx := r.Context()
	hops := dnsx.Walk(ctx, name)
	result.Spend(len(hops))

	// A walk that stopped early, because a hop failed or because it named a
	// server the address guard refused, leaves a short chain of trust. Without
	// this the flow below just renders fewer levels and looks like a finding
	// about the zone rather than a gap in what could be measured.
	for _, hop := range hops {
		if hop.Err != "" {
			result.Degrade("walk "+hop.Zone, hop.Err)
		}
	}

	answers := dnsx.QueryMany(ctx, []dnsx.Job{
		{Name: name, Type: "DNSKEY", Resolver: resolvers.Default.IP},
		{Name: name, Type: "DS", Resolver: resolvers.Default.IP},
		{Name: name, Type: "SOA", Resolver: resolvers.Default.IP},
		{Name: name, Type: "A", Resolver: resolvers.Default.IP},
	}, 4)
	result.Spend(len(answers))
	dnskey, ds, soa, aRecord := answers[0], answers[1], answers[2], answers[3]
	result.HoldTTL(dnsx.MinTTL(answers), "dns")

	keys := dnsx.ParseDNSKEYs(name, dnskey.Records)
	ksk, zsk := 0, 0
	for _, key := range keys {
		if key.Flags == 257 {
			ksk++
		} else {
			zsk++
		}
	}

	// only a ds for this exact name in its parent makes the name signed. a ds
	// higher up the walk says the tld is signed, which is true for almost every
	// name and says nothing about this one.
	dsAtParent := len(ds.Records) > 0
	hasKeys := len(keys) > 0
	validated := aRecord.Authenticated

	switch {
	case dsAtParent && hasKeys && validated:
		result.SetVerdict("ok", name+" is signed and answers validate",
			fmt.Sprintf("%d key signing and %d zone signing keys", ksk, zsk))
	case dsAtParent:
		result.SetVerdict("none", name+" is signed but answers are not validating",
			fmt.Sprintf("%d key signing and %d zone signing keys", ksk, zsk))
	case hasKeys:
		result.SetVerdict("none", name+" publishes keys but the parent holds no ds",
			"an island of trust, so validating resolvers have no path to these keys")
	default:
		result.SetVerdict("none", name+" is not signed",
			"most of the namespace is unsigned, so this is a choice rather than a fault")
	}

	flowRows := []screen.FlowRow{{Nodes: []screen.FlowNode{{Label: "root ksk", Tone: "accent"}}}}

	parentNodes := make([]screen.FlowNode, 0, len(hops))
	for _, hop := range hops {
		if hop.Zone == name {
			continue
		}
		if len(hop.DS) > 0 {
			parentNodes = append(parentNodes, screen.FlowNode{Label: hop.Zone + " ds", Tone: "accent"})
		} else {
			parentNodes = append(parentNodes, screen.FlowNode{Label: hop.Zone + " unsigned", Tone: "muted"})
		}
	}
	if len(parentNodes) == 0 {
		parentNodes = []screen.FlowNode{{Label: "no parent level", Tone: "muted"}}
	}
	flowRows = append(flowRows, screen.FlowRow{Nodes: parentNodes})

	dsLabel, dsTone := "ds at parent missing", "muted"
	if dsAtParent {
		dsLabel, dsTone = "ds at parent found", "accent"
	}
	flowRows = append(flowRows, screen.FlowRow{Nodes: []screen.FlowNode{{Label: dsLabel, Tone: dsTone, Stretch: true}}})

	kskTone, zskTone := "muted", "muted"
	if ksk > 0 {
		kskTone = "accent"
	}
	if zsk > 0 {
		zskTone = "accent"
	}
	flowRows = append(flowRows, screen.FlowRow{Nodes: []screen.FlowNode{
		{Label: fmt.Sprintf("ksk %d", ksk), Tone: kskTone},
		{Label: fmt.Sprintf("zsk %d", zsk), Tone: zskTone},
	}})

	validLabel, validTone := "no ad flag, answers are not validated", "muted"
	if validated {
		validLabel, validTone = "ad flag set, answers validate", "accent"
	}
	flowRows = append(flowRows, screen.FlowRow{Nodes: []screen.FlowNode{{Label: validLabel, Tone: validTone, Stretch: true}}})

	result.Add("GraphFlow", screen.FlowProps{Title: "chain of trust", Rows: flowRows}, 2)

	dsNote := "none, so the zone is unsigned from the parent's view"
	if dsAtParent {
		dsNote = fmt.Sprintf("%d ds records", len(ds.Records))
	}
	keyNote := "none published"
	if hasKeys {
		keyNote = fmt.Sprintf("%d key signing, %d zone signing", ksk, zsk)
	}
	validNote := "no ad flag, which is expected when the zone is unsigned"
	switch {
	case validated:
		validNote = "the ad flag was set on the answer"
	case dsAtParent:
		validNote = "no ad flag, even though the parent holds a ds record"
	}
	soaNote := "none"
	if len(soa.Records) > 0 {
		soaNote = soa.Records[0]
	}
	result.Add("GraphCheck", screen.CheckProps{Title: "chain", Items: []screen.CheckItem{
		{Label: "parent holds a ds record", Done: dsAtParent, Note: dsNote},
		{Label: "zone publishes dnskey", Done: hasKeys, Note: keyNote},
		{Label: "resolver validates", Done: validated, Note: validNote},
		{Label: "soa answered", Done: len(soa.Records) > 0, Note: soaNote},
	}}, 1)

	keyRows := make([][]string, 0, len(keys))
	for _, key := range keys {
		algorithm, ok := dnsx.DNSKEYAlgorithms[key.Algorithm]
		if !ok {
			algorithm = "algorithm " + strconv.Itoa(key.Algorithm)
		}
		keyRows = append(keyRows, []string{strconv.Itoa(key.Flags), key.Role, algorithm, strconv.Itoa(key.Tag)})
	}
	if len(keyRows) == 0 {
		keyRows = [][]string{{"-", "none", "the zone publishes no dnskey", "-"}}
	}
	result.Add("GraphTable", screen.TableProps{
		Title: "dnskey", Headers: []string{"flags", "role", "algorithm", "tag"},
		Align: []string{"right", "left", "left", "right"}, Rows: keyRows,
	}, 2)

	result.Add("GraphTable", screen.TableProps{
		Title: "ds", Headers: []string{"tag", "algorithm", "digest"},
		Align: []string{"right", "left", "left"}, Rows: dsRows(ds.Records),
	}, 1)

	levelNodes := make([]screen.TreeNode, 0, len(hops))
	for _, hop := range hops {
		meta := "unsigned at this level"
		if len(hop.DS) > 0 {
			meta = fmt.Sprintf("%d ds", len(hop.DS))
		}
		levelNodes = append(levelNodes, screen.TreeNode{Label: hop.Zone, Meta: meta, Accent: len(hop.DS) > 0})
	}
	if len(levelNodes) == 0 {
		levelNodes = []screen.TreeNode{{Label: "walk failed", Meta: "no levels reached"}}
	}
	result.Add("GraphTree", screen.TreeProps{Title: "signed levels", Nodes: levelNodes}, 1)

	switch {
	case !dsAtParent && hasKeys:
		result.Note("the zone publishes dnskey records but the parent holds no ds for it, so the keys are an island of trust. resolvers can’t reach them from the root and answer as if the zone were unsigned.")
	case !dsAtParent:
		result.Note("no ds record at the parent means the zone is unsigned. that’s a configuration choice, not a fault, and most of the namespace is in the same position.")
	}
}

func dsRows(records []string) [][]string {
	rows := make([][]string, 0, len(records))
	for _, record := range records {
		fields := strings.Fields(record)
		if len(fields) < 4 {
			continue
		}
		algorithm := fields[1]
		if n, err := strconv.Atoi(fields[1]); err == nil {
			if named, ok := dnsx.DNSKEYAlgorithms[n]; ok {
				algorithm = named
			}
		}
		digest := fields[2]
		if n, err := strconv.Atoi(fields[2]); err == nil {
			if named, ok := dnsx.DSDigests[n]; ok {
				digest = named
			}
		}
		rows = append(rows, []string{fields[0], algorithm, digest})
	}
	if len(rows) == 0 {
		return [][]string{{"-", "none", "no ds record"}}
	}
	return rows
}

func sameSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for key := range a {
		if !b[key] {
			return false
		}
	}
	return true
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
