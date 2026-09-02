package screen

import (
	"net/http"
	"strings"

	"github.com/zaidmukaddam/dug/pkg/commands"
)

// Argument reads the command and target of a request and refuses what the
// grammar refuses, so a handler starts from a command it actually serves and
// a non-empty target. The wording comes from pkg/commands, the same table
// llms.txt and the OpenAPI document are generated from.
func Argument(w http.ResponseWriter, req *http.Request, endpoint, fallback string) (command, target string, ok bool) {
	query := req.URL.Query()

	command = strings.ToUpper(strings.TrimSpace(query.Get("command")))
	if command == "" {
		command = fallback
	}
	target = strings.TrimSpace(query.Get("target"))

	spec, found := commands.ByName(command)
	if !found || spec.Endpoint != endpoint {
		var names []string
		for _, s := range commands.List {
			if s.Endpoint == endpoint {
				names = append(names, s.Name)
			}
		}
		Fail(w, req, fallback, target, command+" is not a command this endpoint answers", "one of "+strings.Join(names, ", "))
		return "", "", false
	}

	if target == "" {
		// spec.Argument is the grammar's kind name; "endpoint" and "pair" are
		// not words a person would type in a refusal, so they get read as nouns.
		noun := argumentNoun[spec.Argument]
		if noun == "" {
			noun = spec.Argument
		}
		Fail(w, req, command, "", "no "+noun+" given", "this command needs "+spec.TargetAbout())
		return "", "", false
	}

	return command, target, true
}

var argumentNoun = map[string]string{"endpoint": "host", "pair": "domain"}
