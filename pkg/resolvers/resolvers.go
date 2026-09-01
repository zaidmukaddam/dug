// Package resolvers holds the fixed public resolver list and the root servers.
//
// Both are constants, not parameters, which is what bounds the fan-out.
// lib/resolvers.ts mirrors this list; the wiring test fails if they drift.
package resolvers

type Resolver struct {
	ID    string
	Name  string
	Short string // axis label, chosen rather than sliced off Name
	IP    string
}

// Six. Five leaves no room for one operator being down without the Uptime row
// looking broken, and past about eight the glyph row stops being scannable.
var List = []Resolver{
	{"cloudflare", "Cloudflare", "cf", "1.1.1.1"},
	{"google", "Google", "goog", "8.8.8.8"},
	{"quad9", "Quad9", "quad9", "9.9.9.9"},
	{"opendns", "OpenDNS", "odns", "208.67.222.222"},
	{"adguard", "AdGuard", "adg", "94.140.14.14"},
	{"controld", "Control D", "ctrld", "76.76.2.0"},
}

var Default = List[0]

type RootServer struct {
	Name string
	IP   string
}

var Roots = []RootServer{
	{"a.root-servers.net", "198.41.0.4"},
	{"b.root-servers.net", "170.247.170.2"},
	{"c.root-servers.net", "192.33.4.12"},
	{"d.root-servers.net", "199.7.91.13"},
	{"e.root-servers.net", "192.203.230.10"},
	{"f.root-servers.net", "192.5.5.241"},
	{"g.root-servers.net", "192.112.36.4"},
	{"h.root-servers.net", "198.97.190.53"},
	{"i.root-servers.net", "192.36.148.17"},
	{"j.root-servers.net", "192.58.128.30"},
	{"k.root-servers.net", "193.0.14.129"},
	{"l.root-servers.net", "199.7.83.42"},
	{"m.root-servers.net", "202.12.27.33"},
}

// ByID looks a resolver up by id, name or address. An unrecognised value
// returns the default rather than being used as a destination.
func ByID(value string) Resolver {
	if value == "" {
		return Default
	}
	for _, entry := range List {
		if value == entry.ID || value == entry.IP || value == entry.Name {
			return entry
		}
	}
	return Default
}
