// The live address policy, so SRC can show it rather than a copy that drifts.
package handler

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/zaidmukaddam/dug/pkg/guard"
	"github.com/zaidmukaddam/dug/pkg/screen"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	result := screen.New("GUARD", "policy")
	result.HoldTTL(86400, "asn")

	ports := guard.AllowedPorts()
	sort.Ints(ports)
	portText := ""
	for i, port := range ports {
		if i > 0 {
			portText += " "
		}
		portText += strconv.Itoa(port)
	}

	denylist := guard.Denylist()

	result.SetVerdict("ok",
		"every destination is validated in the dialer, immediately before connect",
		strconv.Itoa(len(denylist))+" explicit ranges plus the netip predicates")

	result.Add("GraphSpec", screen.SpecProps{Title: "policy", Rows: []screen.SpecRow{
		{Label: "allowed ports", Value: portText},
		{Label: "allowed schemes", Value: "http https"},
		{Label: "validated", Value: "resolved addresses, in net.Dialer.Control"},
		{Label: "explicit ranges", Value: strconv.Itoa(len(denylist))},
		{Label: "also refused", Value: "loopback, private, link-local, multicast, unspecified, via netip predicates"},
		{Label: "ipv4-mapped", Value: "unmapped before every predicate, and judged by the embedded address"},
	}}, 1)

	rows := make([][]string, 0, len(denylist))
	for _, entry := range denylist {
		rows = append(rows, []string{entry[0], entry[1]})
	}
	result.Add("GraphTable", screen.TableProps{
		Title: "explicit denylist", Headers: []string{"range", "reason"}, Rows: rows,
	}, 2)

	result.Note("netip covers loopback, private, link-local, multicast and unspecified as predicates. the ranges listed here are the ones it has no predicate for, notably cgnat.")
	result.Write(w, r)
}
