// MAIL and SPF. Whether mail from this domain will authenticate.
package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/zaidmukaddam/dug/pkg/dnsx"
	"github.com/zaidmukaddam/dug/pkg/mailx"
	"github.com/zaidmukaddam/dug/pkg/resolvers"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	target := query.Get("target")
	command := query.Get("command")
	if command == "" {
		command = "MAIL"
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
	if command == "SPF" {
		runSPF(r, result, name)
	} else {
		runMail(r, result, name)
	}
	result.Write(w, r)
}

func runMail(r *http.Request, result *screen.Result, name string) {
	ctx := r.Context()
	resolverIP := resolvers.Default.IP

	base := dnsx.QueryMany(ctx, []dnsx.Job{
		{Name: name, Type: "MX", Resolver: resolverIP},
		{Name: name, Type: "TXT", Resolver: resolverIP},
		{Name: "_dmarc." + name, Type: "TXT", Resolver: resolverIP},
	}, 3)
	result.Spend(3)
	result.HoldTTL(dnsx.MinTTL(base), "dns")
	mx, txt, dmarcAnswer := base[0], base[1], base[2]

	spfRecord := mailx.ParseSPF(dnsx.TXTStrings(txt))
	dmarc := mailx.ParseDMARC(dnsx.TXTStrings(dmarcAnswer))
	selectors := mailx.ProbeDKIM(ctx, name, resolverIP)
	result.Spend(len(mailx.Selectors))

	// Still expanded, because the lookup count and the limit verdict depend on
	// it. Only the rendered tree is left to the SPF screen.
	walk := mailx.NewWalk(resolverIP)
	if spfRecord != "" {
		walk.Expand(ctx, name, 0)
	}
	result.Spend(walk.Queries)

	hasMX := len(mx.Records) > 0 && !(len(mx.Records) == 1 && strings.TrimSpace(mx.Records[0]) == "0 .")
	hasSPF := spfRecord != ""
	hasDKIM := len(selectors) > 0
	policy := dmarc["p"]
	hasDMARC := len(dmarc) > 0
	enforcing := policy == "quarantine" || policy == "reject"
	withinLimit := walk.Lookups <= mailx.LookupLimit
	aspf := valueOr(dmarc, "aspf", "r")
	adkim := valueOr(dmarc, "adkim", "r")

	switch {
	case !hasMX:
		result.SetVerdict("none", name+" does not receive mail",
			"no mx records, so the checks below describe sending only")
	case hasSPF && hasDMARC && enforcing && withinLimit:
		result.SetVerdict("ok", "mail claiming to be from "+name+" is protected",
			"spf and dmarc are published and dmarc is set to "+policy)
	case !hasSPF && !hasDMARC:
		result.SetVerdict("warn", "anyone can send mail as "+name,
			"neither spf nor dmarc is published")
	case !withinLimit:
		result.SetVerdict("warn", name+" has an spf record receivers cannot finish evaluating",
			fmt.Sprintf("%d dns lookups against a limit of %d", walk.Lookups, mailx.LookupLimit))
	default:
		detail := "spf is missing"
		if hasSPF {
			detail = "dmarc is missing"
			if hasDMARC && !enforcing {
				detail = "dmarc is set to monitor only"
			}
		}
		result.SetVerdict("warn", name+" is only partly protected", detail)
	}

	result.Add("GraphFlow", screen.FlowProps{Title: "authentication", Rows: []screen.FlowRow{
		{Nodes: []screen.FlowNode{{Label: mxLabel(hasMX, len(mx.Records)), Tone: tone(hasMX), Stretch: true}}},
		{Nodes: []screen.FlowNode{
			{Label: labelOr(hasSPF, "spf", "no spf"), Tone: tone(hasSPF)},
			{Label: dkimLabel(hasDKIM, len(selectors)), Tone: tone(hasDKIM)},
		}},
		{Nodes: []screen.FlowNode{
			{Label: lookupLabel(hasSPF, walk.Lookups), Tone: tone(withinLimit && hasSPF)},
			{Label: dmarcLabel(hasDMARC, policy), Tone: tone(hasDMARC)},
		}},
		{Nodes: []screen.FlowNode{{Label: alignmentLabel(hasDMARC, aspf, adkim), Tone: tone(hasDMARC), Stretch: true}}},
		{Nodes: []screen.FlowNode{{Label: enforcedLabel(enforcing, hasDMARC), Tone: tone(enforcing), Stretch: true}}},
	}}, 2)

	result.Add("GraphCheck", screen.CheckProps{Title: "punch list", Items: []screen.CheckItem{
		{Label: "mx records", Done: hasMX, Note: noteOr(hasMX,
			fmt.Sprintf("%d hosts", len(mx.Records)), "none, this domain receives no mail")},
		// Whether it exists, not what it says. The SPF screen has the record.
		{Label: "spf record", Done: hasSPF, Note: noteOr(hasSPF,
			fmt.Sprintf("published, %d mechanisms", len(mailx.Terms(spfRecord))), "none published")},
		{Label: "spf within ten lookups", Done: hasSPF && withinLimit, Note: noteOr(hasSPF,
			fmt.Sprintf("%d of %d", walk.Lookups, mailx.LookupLimit), "no record to evaluate")},
		{Label: "spf ends in a fail", Done: hasSPF && isFail(mailx.Terminal(spfRecord)), Note: noteOr(hasSPF,
			mailx.AllMeaning(mailx.Terminal(spfRecord)), "none published")},
		{Label: "dkim selector found", Done: hasDKIM, Note: noteOr(hasDKIM,
			selectorNames(selectors), fmt.Sprintf("none of %d common selectors answered", len(mailx.Selectors)))},
		{Label: "dmarc record", Done: hasDMARC, Note: noteOr(hasDMARC,
			"p="+policy, "none, so nothing tells receivers what to do")},
		{Label: "dmarc enforces", Done: enforcing, Note: noteOr(hasDMARC, "p="+policy, "none published")},
		{Label: "reporting address", Done: dmarc["rua"] != "", Note: noteOr(dmarc["rua"] != "",
			dmarc["rua"], "none, so failures are invisible to the owner")},
	}}, 1)

	const reach = 100
	result.Add("GraphFunnel", screen.FunnelProps{
		Title: "deliverability",
		Steps: []screen.FunnelStep{
			// Fixed track ellipsises longer labels; punch list has the wording.
			{Label: "mx", Value: step(hasMX, reach), Display: yesNo(hasMX)},
			{Label: "spf", Value: step(hasMX && hasSPF, reach), Display: yesNo(hasMX && hasSPF)},
			{Label: "evaluable", Value: step(hasMX && hasSPF && withinLimit, reach), Display: yesNo(hasMX && hasSPF && withinLimit)},
			{Label: "dmarc", Value: step(hasMX && hasSPF && hasDMARC, reach), Display: yesNo(hasMX && hasSPF && hasDMARC)},
			// yes/no like the steps above it. The Funnel display track is 8
			// characters wide, so "quarantine" breaks mid-word; the policy
			// word is already on the flow, the punch list and the dmarc spec.
			{Label: "enforced", Value: step(hasMX && hasSPF && hasDMARC && enforcing, reach), Display: yesNo(enforcing)},
		},
		Stage: "each step needs the one above it",
	}, 1)

	// The full include tree is not here. For a large sender it runs to two
	// thousand pixels, half this screen, to say something the lookup count
	// above already says. SPF owns the expansion; scope section 10 assigns
	// Tree to NS and SPF, never to MAIL.
	result.Add("GraphBullet", screen.BulletProps{Title: "spf lookups", Items: []screen.BulletItem{
		{Label: "querying mechanisms", Value: walk.Lookups, Target: mailx.LookupLimit,
			Max:     maxInt(mailx.LookupLimit+2, walk.Lookups),
			Display: fmt.Sprintf("%d of %d", walk.Lookups, mailx.LookupLimit)},
		{Label: "void lookups", Value: walk.Void, Target: 2, Max: maxInt(4, walk.Void),
			Display: fmt.Sprintf("%d of 2", walk.Void)},
	}}, 1)

	mxRows := make([][]string, 0, len(mx.Records))
	for _, record := range mx.Records {
		fields := strings.Fields(record)
		if len(fields) == 2 {
			mxRows = append(mxRows, []string{fields[0], strings.TrimSuffix(fields[1], ".")})
		}
	}
	if len(mxRows) == 0 {
		mxRows = [][]string{{"-", "none published"}}
	}
	result.Add("GraphTable", screen.TableProps{
		Title: "mx", Headers: []string{"priority", "host"},
		Align: []string{"right", "left"}, Rows: mxRows,
	}, 1)

	result.Add("GraphSpec", screen.SpecProps{Title: "dmarc", Rows: []screen.SpecRow{
		{Label: "policy", Value: orNone(policy), Accent: true},
		{Label: "subdomain policy", Value: valueOr(dmarc, "sp", "inherits the policy above")},
		{Label: "percentage", Value: valueOr(dmarc, "pct", "100")},
		{Label: "spf alignment", Value: alignment(hasDMARC, aspf)},
		{Label: "dkim alignment", Value: alignment(hasDMARC, adkim)},
		{Label: "aggregate reports", Value: valueOr(dmarc, "rua", "none")},
	}}, 2)

	if !withinLimit {
		result.Note(fmt.Sprintf(
			"the spf record needs %d dns lookups and the limit is %d. receivers stop evaluating at the limit and return permerror, which means spf fails for senders past that point even though the record looks correct.",
			walk.Lookups, mailx.LookupLimit))
	}
	if !hasDKIM {
		result.Note(fmt.Sprintf(
			"no dkim selector answered out of %d common ones. selectors can’t be enumerated from dns, so this means not found, not absent.",
			len(mailx.Selectors)))
	}
	if hasSPF {
		result.Note(fmt.Sprintf(
			"the spf record expands across %d domains and %d lookups. SPF %s draws the include tree.",
			walk.Domains(), walk.Lookups, name))
	}
	result.Note("alignment here is the published policy, not an observed result. whether a given message aligns depends on the message, and there’s no message in this query.")
}

func runSPF(r *http.Request, result *screen.Result, name string) {
	ctx := r.Context()
	resolverIP := resolvers.Default.IP

	txt := dnsx.Query(ctx, name, "TXT", resolverIP)
	result.Spend(1)
	result.HoldTTL(txt.TTL, "dns")
	record := mailx.ParseSPF(dnsx.TXTStrings(txt))

	if record == "" {
		result.SetVerdict("none", name+" publishes no spf record", "that is a finding, not an error")
		result.Add("GraphSpec", screen.SpecProps{Title: "spf", Rows: []screen.SpecRow{
			{Label: "record", Value: "none published"},
			{Label: "effect", Value: "receivers have no sender policy to check against"},
			{Label: "txt records", Value: strconv.Itoa(len(txt.Records))},
		}}, 3)
		result.Note("no spf record. that is a finding, not an error.")
		return
	}

	walk := mailx.NewWalk(resolverIP)
	tree := walk.Expand(ctx, name, 0)
	result.Spend(walk.Queries)
	within := walk.Lookups <= mailx.LookupLimit

	if within {
		result.SetVerdict("ok",
			fmt.Sprintf("%s uses %d of the %d dns lookups spf allows", name, walk.Lookups, mailx.LookupLimit),
			fmt.Sprintf("expanded across %d domains", walk.Domains()))
	} else {
		result.SetVerdict("warn",
			fmt.Sprintf("%s needs %d dns lookups and spf allows %d", name, walk.Lookups, mailx.LookupLimit),
			"receivers stop at the limit and return permerror, so spf fails")
	}

	result.Add("GraphKpi", screen.KpiProps{
		Title: "lookups",
		Value: fmt.Sprintf("%d/%d", walk.Lookups, mailx.LookupLimit),
		Label: "querying mechanisms",
		Hint:  "over the limit means permerror, not a warning",
		Data:  []int{walk.Lookups, mailx.LookupLimit},
	}, 1)

	result.Add("GraphBullet", screen.BulletProps{Title: "against the limit", Items: []screen.BulletItem{
		{Label: "dns lookups", Value: walk.Lookups, Target: mailx.LookupLimit,
			Max:     maxInt(mailx.LookupLimit+2, walk.Lookups),
			Display: fmt.Sprintf("%d of %d", walk.Lookups, mailx.LookupLimit)},
		{Label: "void lookups", Value: walk.Void, Target: 2, Max: maxInt(4, walk.Void),
			Display: fmt.Sprintf("%d of 2", walk.Void)},
	}}, 1)

	result.Add("GraphTree", screen.TreeProps{Title: "include tree", Nodes: []screen.TreeNode{toTree(tree)}}, 3)

	termRows := make([][]string, 0)
	for _, term := range mailx.Terms(record) {
		lookup := "no"
		if mailx.Querying[term.Mechanism] {
			lookup = "yes"
		}
		value := term.Value
		if value == "" {
			value = "-"
		}
		termRows = append(termRows, []string{term.Qualifier, term.Mechanism, value, lookup})
	}
	result.Add("GraphTable", screen.TableProps{
		Title: "mechanisms", Headers: []string{"qualifier", "mechanism", "value", "lookup"}, Rows: termRows,
	}, 2)

	withinText := "yes"
	if !within {
		withinText = fmt.Sprintf("no, %d lookups", walk.Lookups)
	}
	result.Add("GraphSpec", screen.SpecProps{Title: "record", Rows: []screen.SpecRow{
		{Label: "raw", Value: record, Accent: true},
		{Label: "terminal", Value: mailx.AllMeaning(mailx.Terminal(record))},
		{Label: "within limit", Value: withinText},
		{Label: "domains expanded", Value: strconv.Itoa(walk.Domains())},
	}}, 2)

	if !within {
		result.Note(fmt.Sprintf(
			"%d lookups against a limit of %d. receivers return permerror at that point, so this record fails for senders that fall past the cut rather than degrading gracefully.",
			walk.Lookups, mailx.LookupLimit))
	}
}

func toTree(node mailx.Node) screen.TreeNode {
	out := screen.TreeNode{Label: node.Label, Meta: node.Meta, Accent: node.Accent}
	for _, child := range node.Children {
		out.Children = append(out.Children, toTree(child))
	}
	return out
}

func tone(ok bool) string {
	if ok {
		return "accent"
	}
	return "muted"
}

func labelOr(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func mxLabel(has bool, count int) string {
	if has {
		return fmt.Sprintf("mx %d", count)
	}
	return "no mx"
}

func dkimLabel(has bool, count int) string {
	if has {
		return fmt.Sprintf("dkim %d", count)
	}
	return "no dkim found"
}

func lookupLabel(hasSPF bool, lookups int) string {
	if !hasSPF {
		return "no lookups"
	}
	return fmt.Sprintf("%d/%d lookups", lookups, mailx.LookupLimit)
}

func dmarcLabel(has bool, policy string) string {
	if has {
		return "dmarc p=" + policy
	}
	return "no dmarc"
}

func alignmentLabel(has bool, aspf, adkim string) string {
	if !has {
		return "no alignment policy"
	}
	return "alignment spf " + aspf + " dkim " + adkim
}

func enforcedLabel(enforcing, hasDMARC bool) string {
	switch {
	case enforcing:
		return "enforced"
	case hasDMARC:
		return "monitored only"
	}
	return "unprotected"
}

func alignment(has bool, mode string) string {
	if !has {
		return "no policy"
	}
	if text, ok := mailx.AlignmentModes[mode]; ok {
		return text
	}
	return mode
}

func isFail(qualifier string) bool { return qualifier == "-" || qualifier == "~" }

func selectorNames(selectors []mailx.Selector) string {
	names := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		names = append(names, selector.Name)
	}
	return strings.Join(names, ", ")
}

func noteOr(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func yesNo(ok bool) string {
	if ok {
		return "yes"
	}
	return "no"
}

func step(ok bool, value int) int {
	if ok {
		return value
	}
	return 0
}

func orNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func valueOr(tags map[string]string, key, fallback string) string {
	if value, ok := tags[key]; ok && value != "" {
		return value
	}
	return fallback
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
