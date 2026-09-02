// PING, ROUTE and PORTS.
//
// PING and ROUTE use the unprivileged ICMP datagram socket, so they need no
// capability; where a sandbox refuses it both commands render the refusal.
// PORTS is a TCP connect scan over a bounded port set. The address guard
// applies to all three; only the port allowlist is waived, and only for PORTS.
package handler

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zaidmukaddam/dug/pkg/dnsx"
	"github.com/zaidmukaddam/dug/pkg/guard"
	"github.com/zaidmukaddam/dug/pkg/icmpx"
	"github.com/zaidmukaddam/dug/pkg/resolvers"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

const (
	maxPorts    = 64
	maxHops     = 20
	maxPings    = 10
	portTimeout = 1500 * time.Millisecond
	// The worst case, 20 silent hops at two probes each, would otherwise be 40s
	// of function time; past this the hops gathered so far are the answer.
	routeBudget = 25 * time.Second
)

// The services worth asking about by default. Named, because a bare port
// number is not an answer to "what is running here".
var commonPorts = []struct {
	port    int
	service string
}{
	{21, "ftp"}, {22, "ssh"}, {23, "telnet"}, {25, "smtp"}, {53, "dns"},
	{80, "http"}, {110, "pop3"}, {143, "imap"}, {443, "https"}, {445, "smb"},
	{465, "smtps"}, {587, "submission"}, {993, "imaps"}, {995, "pop3s"},
	{1433, "mssql"}, {3306, "mysql"}, {3389, "rdp"}, {5432, "postgres"},
	{5900, "vnc"}, {6379, "redis"}, {8080, "http alt"}, {8443, "https alt"},
	{9200, "elasticsearch"}, {27017, "mongodb"},
}

var serviceByPort = func() map[int]string {
	out := map[int]string{}
	for _, entry := range commonPorts {
		out[entry.port] = entry.service
	}
	return out
}()

func Handler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	command, target, ok := screen.Argument(w, r, "/api/probe", "PING")
	if !ok {
		return
	}

	host := target
	if _, err := netip.ParseAddr(target); err != nil {
		name, nameErr := dnsx.ToName(target)
		if nameErr != nil {
			screen.Fail(w, r, command, target, target+" is not a host or an ip address", nameErr.Error())
			return
		}
		host = name
	}

	result := screen.New(command, host)
	switch command {
	case "ROUTE":
		runRoute(r, result, host)
	case "PORTS":
		runPorts(r, result, host, query.Get("ports"))
	default:
		count := 5
		if value, err := strconv.Atoi(query.Get("count")); err == nil && value > 0 {
			count = minInt(maxPings, value)
		}
		runPing(r, result, host, count)
	}
	result.Write(w, r)
}

func runPing(r *http.Request, result *screen.Result, host string, count int) {
	if ok, why := icmpx.Available(false); !ok {
		icmpUnavailable(result, "ping", why)
		return
	}

	addr, err := resolveOne(r.Context(), host)
	if err != nil {
		refused(result, err)
		return
	}

	replies := icmpx.Ping(r.Context(), net.IP(addr.AsSlice()), count, 2*time.Second)
	result.Spend(count)
	result.HoldTTL(60, "dns")

	var times []int
	answered := 0
	data := make([]int, 0, len(replies))
	statuses := make([]string, 0, len(replies))
	rows := make([][]string, 0, len(replies))

	for i, reply := range replies {
		ms := int(reply.RTT.Milliseconds())
		data = append(data, ms)
		if reply.Kind == "echo" {
			answered++
			times = append(times, ms)
			statuses = append(statuses, "ok")
		} else {
			statuses = append(statuses, "down")
		}
		rtt := "-"
		if reply.Kind == "echo" {
			rtt = fmt.Sprintf("%.2f", float64(reply.RTT.Microseconds())/1000)
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), reply.Kind, orDash(reply.Source), rtt})
	}
	sort.Ints(times)

	loss := 100 * (len(replies) - answered) / len(replies)
	median := 0
	if len(times) > 0 {
		median = times[len(times)/2]
	}

	if answered == 0 {
		result.SetVerdict("warn",
			fmt.Sprintf("%s did not answer any of %d pings", host, len(replies)),
			"a host may drop icmp by policy, so this isn’t proof it’s down")
	} else {
		state := "ok"
		if loss > 0 {
			state = "warn"
		}
		result.SetVerdict(state,
			fmt.Sprintf("%s answers in %dms with %d percent loss", host, median, loss),
			fmt.Sprintf("%d of %d echo requests returned from %s", answered, len(replies), addr))
	}

	value := "no reply"
	if answered > 0 {
		value = strconv.Itoa(median) + "ms"
	}
	result.Add("GraphKpi", screen.KpiProps{
		Title: "round trip", Value: value, Label: "median of the answered probes",
		Hint: fmt.Sprintf("%d percent loss over %d probes", loss, len(replies)), Data: data,
	}, 1)

	result.Add("GraphUptime", screen.UptimeProps{
		Title: "probes", Days: statuses, From: "first", To: "last", Columns: len(replies),
	}, 1)

	if len(times) >= 2 {
		labels := make([]string, 0, len(replies))
		for i := range replies {
			labels = append(labels, strconv.Itoa(i+1))
		}
		result.Add("GraphPlot", screen.PlotProps{
			Title: "latency", Data: data, Labels: labels, Variant: "area", Height: 7,
		}, 2)
	}

	spread := "no samples"
	if len(times) > 0 {
		spread = fmt.Sprintf("%dms to %dms", times[0], times[len(times)-1])
	}
	result.Add("GraphSpec", screen.SpecProps{Title: "target", Rows: []screen.SpecRow{
		{Label: "host", Value: host, Accent: true},
		{Label: "address", Value: addr.String()},
		{Label: "protocol", Value: "icmp echo over an unprivileged datagram socket"},
		{Label: "sent", Value: strconv.Itoa(len(replies))},
		{Label: "lost", Value: fmt.Sprintf("%d percent", loss)},
		{Label: "spread", Value: spread},
	}}, 1)

	result.Add("GraphTable", screen.TableProps{
		Title: "probes", Headers: []string{"probe", "result", "from", "ms"},
		Align: []string{"right", "left", "left", "right"}, Rows: rows,
	}, 1)

	result.Note("silence is not proof a host is down. dropping icmp is a common and deliberate configuration, and this measures one path from one region.")
}

func runRoute(r *http.Request, result *screen.Result, host string) {
	if ok, why := icmpx.Available(false); !ok {
		icmpUnavailable(result, "route", why)
		return
	}

	addr, err := resolveOne(r.Context(), host)
	if err != nil {
		refused(result, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), routeBudget)
	defer cancel()

	hops := icmpx.Traceroute(ctx, net.IP(addr.AsSlice()), maxHops, time.Second, 2)
	if ctx.Err() != nil {
		result.Note("the route was cut at " + routeBudget.String() + "; hops after that were not probed")
	}
	result.Spend(len(hops) * 2)
	result.HoldTTL(300, "dns")

	// Name the hops that answered, asked of our own resolver, so this adds no
	// exposure to the target.
	jobs := make([]dnsx.Job, 0, len(hops))
	answering := make([]icmpx.Hop, 0, len(hops))
	for _, hop := range hops {
		if hop.Source == "" {
			continue
		}
		if reverse, err := dnsx.ReverseName(hop.Source); err == nil {
			jobs = append(jobs, dnsx.Job{Name: reverse, Type: "PTR", Resolver: resolvers.Default.IP})
			answering = append(answering, hop)
		}
	}
	names := map[string]string{}
	if len(jobs) > 0 {
		answers := dnsx.QueryMany(r.Context(), jobs, 8)
		result.Spend(len(answers))
		for i, answer := range answers {
			if len(answer.Records) > 0 {
				names[answering[i].Source] = strings.TrimSuffix(answer.Records[0], ".")
			}
		}
	}

	reached := len(hops) > 0 && hops[len(hops)-1].Kind == "echo"
	responding, silent := 0, 0
	for _, hop := range hops {
		if hop.Source != "" {
			responding++
		} else {
			silent++
		}
	}

	detail := fmt.Sprintf("%d hops answered and %d stayed silent", responding, silent)
	if silent > 0 {
		detail += ", which is normal because a router is free to ignore icmp"
	}
	if reached {
		result.SetVerdict("ok", fmt.Sprintf("%s is %d hops away", host, len(hops)), detail)
	} else {
		result.SetVerdict("warn", fmt.Sprintf("%s was not reached in %d hops", host, maxHops), detail)
	}

	rows := make([][]string, 0, len(hops))
	statuses := make([]string, 0, len(hops))
	rankItems := make([]screen.RankItem, 0, len(hops))
	for _, hop := range hops {
		source := "no reply"
		status := "down"
		if hop.Source != "" {
			source = hop.Source
			status = "ok"
		}
		rtt := "-"
		if hop.Source != "" {
			rtt = fmt.Sprintf("%.1f", float64(hop.RTT.Microseconds())/1000)
			rankItems = append(rankItems, screen.RankItem{
				Label:   strconv.Itoa(hop.TTL) + " " + nameOr(names, hop.Source),
				Value:   int(hop.RTT.Milliseconds()),
				Display: strconv.Itoa(int(hop.RTT.Milliseconds())) + "ms",
			})
		}
		rows = append(rows, []string{strconv.Itoa(hop.TTL), source, nameOr(names, hop.Source), rtt})
		statuses = append(statuses, status)
	}

	result.Add("GraphTable", screen.TableProps{
		Title: "path", Headers: []string{"hop", "address", "name", "ms"},
		Align: []string{"right", "left", "left", "right"}, Rows: rows,
	}, 2)

	result.Add("GraphUptime", screen.UptimeProps{
		Title: "hops answering", Days: statuses, From: "1", To: strconv.Itoa(len(hops)), Columns: len(hops),
	}, 1)

	if len(rankItems) > 0 {
		sort.SliceStable(rankItems, func(i, j int) bool { return rankItems[i].Value > rankItems[j].Value })
		if len(rankItems) > 8 {
			rankItems = rankItems[:8]
		}
		result.Add("GraphRank", screen.RankProps{Title: "slowest hops", Items: rankItems}, 2)
	}

	result.Add("GraphSpec", screen.SpecProps{Title: "route", Rows: []screen.SpecRow{
		{Label: "host", Value: host, Accent: true},
		{Label: "address", Value: addr.String()},
		{Label: "hops", Value: strconv.Itoa(len(hops))},
		{Label: "destination reached", Value: yesNo(reached)},
		{Label: "ceiling", Value: strconv.Itoa(maxHops) + " hops"},
	}}, 1)

	result.Note("a silent hop is a router that does not answer icmp, not a broken link. the path is also one direction and one moment, and the return path may differ entirely.")
}

func runPorts(r *http.Request, result *screen.Result, host, raw string) {
	ports, warning := parsePorts(raw)
	if len(ports) == 0 {
		screenFailPorts(result, warning)
		return
	}

	addr, err := resolveOne(r.Context(), host)
	if err != nil {
		refused(result, err)
		return
	}

	result.Budget = maxPorts + 16
	findings := scan(r.Context(), addr.String(), ports)
	result.Spend(len(findings))
	result.HoldTTL(300, "http")

	if warning != "" {
		result.Degrade("ports", warning)
	}

	var open, closed, filtered []portResult
	for _, finding := range findings {
		switch finding.state {
		case "open":
			open = append(open, finding)
		case "closed":
			closed = append(closed, finding)
		default:
			filtered = append(filtered, finding)
		}
	}

	if len(open) > 0 {
		names := make([]string, 0, len(open))
		for i, finding := range open {
			if i >= 6 {
				break
			}
			names = append(names, strconv.Itoa(finding.port)+" "+finding.service)
		}
		result.SetVerdict("ok",
			fmt.Sprintf("%d of %d scanned ports are open on %s", len(open), len(findings), host),
			strings.Join(names, ", "))
	} else {
		result.SetVerdict("none",
			fmt.Sprintf("none of the %d scanned ports answered on %s", len(findings), host),
			"closed means refused, filtered means silent")
	}

	result.Add("GraphStat", screen.StatProps{Title: "result", Items: []screen.StatItem{
		{Value: strconv.Itoa(len(open)), Label: "open", Accent: true},
		{Value: strconv.Itoa(len(closed)), Label: "closed, refused"},
		{Value: strconv.Itoa(len(filtered)), Label: "filtered, silent"},
		{Value: strconv.Itoa(len(findings)), Label: "scanned"},
	}}, 3)

	checkItems := make([]screen.CheckItem, 0, len(findings))
	for _, finding := range findings {
		if len(checkItems) >= 24 {
			break
		}
		note := finding.state
		if finding.state != "open" && finding.detail != "" {
			note = finding.state + ", " + finding.detail
		}
		checkItems = append(checkItems, screen.CheckItem{
			Label: strconv.Itoa(finding.port) + " " + finding.service,
			Done:  finding.state == "open",
			Note:  note,
		})
	}
	result.Add("GraphCheck", screen.CheckProps{Title: "open ports", Items: checkItems}, 1)

	rows := make([][]string, 0, len(findings))
	for _, finding := range findings {
		rows = append(rows, []string{
			strconv.Itoa(finding.port), finding.service, finding.state, strconv.Itoa(finding.ms),
		})
	}
	result.Add("GraphTable", screen.TableProps{
		Title: "ports", Headers: []string{"port", "service", "state", "ms"},
		Align: []string{"right", "left", "left", "right"}, Rows: rows,
	}, 2)

	result.Add("GraphSpec", screen.SpecProps{Title: "target", Rows: []screen.SpecRow{
		{Label: "host", Value: host, Accent: true},
		{Label: "address", Value: addr.String()},
		{Label: "method", Value: "tcp connect, no raw packets"},
		{Label: "ports", Value: strconv.Itoa(len(findings))},
		{Label: "timeout", Value: portTimeout.String() + " each"},
	}}, 1)

	result.Note("a tcp connect scan completes the handshake, so it appears in the target's logs as an ordinary connection from this deployment's egress address. scan hosts you are responsible for.")
	result.Note(fmt.Sprintf(
		"the common service list is scanned by default and a request is capped at %d ports. pass your own with PORTS host 22,80,8000-8010.", maxPorts))
}

type portResult struct {
	port    int
	service string
	state   string
	detail  string
	ms      int
}

// scan connects with the guard's port-scan dialer: the port allowlist is
// waived, every address is still checked.
func scan(ctx context.Context, address string, ports []int) []portResult {
	results := make([]portResult, len(ports))
	dialer := guard.PortScanDialer(portTimeout)

	var wait sync.WaitGroup
	slots := make(chan struct{}, 16)

	for i, port := range ports {
		wait.Add(1)
		go func(i, port int) {
			defer wait.Done()
			slots <- struct{}{}
			defer func() { <-slots }()

			started := time.Now()
			conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(address, strconv.Itoa(port)))
			state, detail := "open", ""
			if err != nil {
				text := err.Error()
				switch {
				case strings.Contains(text, "refused"):
					state, detail = "closed", "actively refused"
				case strings.Contains(text, "timeout") || strings.Contains(text, "deadline"):
					state, detail = "filtered", "no response before the timeout"
				default:
					state, detail = "filtered", text
				}
			} else {
				conn.Close()
			}

			results[i] = portResult{
				port:    port,
				service: serviceOr(port),
				state:   state,
				detail:  detail,
				ms:      int(time.Since(started).Milliseconds()),
			}
		}(i, port)
	}
	wait.Wait()
	return results
}

// parsePorts accepts a comma list and dash ranges, or the common set.
func parsePorts(raw string) ([]int, string) {
	text := strings.ReplaceAll(strings.TrimSpace(raw), " ", "")
	if text == "" {
		ports := make([]int, 0, len(commonPorts))
		for _, entry := range commonPorts {
			ports = append(ports, entry.port)
		}
		return ports, ""
	}

	seen := map[int]bool{}
	var ports []int
	add := func(port int) {
		if port >= 1 && port <= 65535 && !seen[port] {
			seen[port] = true
			ports = append(ports, port)
		}
	}

	for _, chunk := range strings.Split(text, ",") {
		if chunk == "" {
			continue
		}
		if low, high, found := strings.Cut(chunk, "-"); found {
			start, err1 := strconv.Atoi(low)
			end, err2 := strconv.Atoi(high)
			if err1 != nil || err2 != nil || start < 1 || end > 65535 || start > end {
				return nil, chunk + " is not a valid port range"
			}
			for port := start; port <= end; port++ {
				add(port)
			}
			continue
		}
		port, err := strconv.Atoi(chunk)
		if err != nil {
			return nil, chunk + " is not a port"
		}
		add(port)
	}

	sort.Ints(ports)
	if len(ports) > maxPorts {
		return ports[:maxPorts], fmt.Sprintf(
			"%d ports asked for and %d is the per request cap, so the rest were not scanned", len(ports), maxPorts)
	}
	return ports, ""
}

func resolveOne(ctx context.Context, host string) (netip.Addr, error) {
	addrs, err := guard.Resolve(ctx, host)
	if err != nil {
		return netip.Addr{}, err
	}
	// ICMP here speaks v4; prefer a v4 answer when the host has one.
	for _, addr := range addrs {
		if addr.Unmap().Is4() {
			return addr.Unmap(), nil
		}
	}
	return addrs[0], nil
}

func icmpUnavailable(result *screen.Result, command, why string) {
	result.Degrade("icmp", why)
	result.SetVerdict("warn", "icmp is not available in this runtime",
		"the kernel refused an unprivileged icmp socket, which sandboxed serverless runtimes do")
	result.Add("GraphSpec", screen.SpecProps{Title: command, Rows: []screen.SpecRow{
		{Label: "result", Value: "no probe was sent", Accent: true},
		{Label: "reason", Value: why},
		{Label: "why it can work", Value: "linux and macos expose icmp over a datagram socket with no capability"},
		{Label: "why it may not", Value: "a sandbox can still refuse the socket, and this one did"},
		{Label: "alternative", Value: "PORTS reaches a host over tcp instead"},
	}}, 3)
	result.HoldTTL(300, "dns")
}

func refused(result *screen.Result, err error) {
	var blocked *guard.Blocked
	target, reason := "destination", err.Error()
	if asBlocked(err, &blocked) {
		target, reason = blocked.Target, blocked.Reason
	}
	result.Degrade("guard", reason)
	result.SetVerdict("warn", target+" is not a permitted destination", reason)
	result.Add("GraphSpec", screen.SpecProps{Title: "refused", Rows: []screen.SpecRow{
		{Label: "target", Value: target, Accent: true},
		{Label: "reason", Value: reason},
		{Label: "policy", Value: "resolved addresses are validated on every command, including these"},
	}}, 3)
	result.HoldTTL(300, "dns")
}

func screenFailPorts(result *screen.Result, warning string) {
	if warning == "" {
		warning = "no ports to scan"
	}
	result.Degrade("ports", warning)
	result.SetVerdict("warn", "the port list could not be read", warning)
	result.Add("GraphSpec", screen.SpecProps{Title: "ports", Rows: []screen.SpecRow{
		{Label: "result", Value: "nothing scanned", Accent: true},
		{Label: "reason", Value: warning},
		{Label: "example", Value: "PORTS example.com 22,80,8000-8010"},
	}}, 3)
	result.HoldTTL(300, "http")
}

func asBlocked(err error, target **guard.Blocked) bool {
	if blocked, ok := err.(*guard.Blocked); ok {
		*target = blocked
		return true
	}
	return false
}

func serviceOr(port int) string {
	if service, ok := serviceByPort[port]; ok {
		return service
	}
	return "unassigned"
}

func nameOr(names map[string]string, source string) string {
	if name, ok := names[source]; ok {
		return name
	}
	return "-"
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func yesNo(ok bool) string {
	if ok {
		return "yes"
	}
	return "no"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
