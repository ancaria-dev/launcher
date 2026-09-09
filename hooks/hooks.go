// Package hooks lists the sites the agent will attach to, read off the agent
// itself.
//
// Every site is installed through one helper that takes its own name first and
// its address second, `hook("goldDelta", RVA.goldDelta, ...)`, and the host
// already accepts `--no-hook goldDelta`.  So the list a player sees comes from
// the scripts in the game folder rather than from a table kept beside them: a
// table would be one more thing to forget, and a hook missing from the list is
// a hook nobody can turn off.
//
// This reads the copy in the game folder, not the embedded payload, because
// that is the copy the host loads, including one somebody edited by hand
// between two runs.
package hooks

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Group is one agent module and the sites it installs.
type Group struct {
	Module string   `json:"module"`
	Hooks  []string `json:"hooks"`
}

// site matches any call whose first argument is a name and whose second is an
// address out of the generated table.  Written this way rather than as a list
// of helper names so that a new wrapper (health already has one) does not
// quietly drop its hooks out of the list.
var site = regexp.MustCompile(`\b\w+\(\s*"([A-Za-z_][A-Za-z0-9_]*)"\s*,\s*RVA\.`)

// Scan reads <coderpack>/agent in load order, which is the order the file names
// already encode.
func Scan(dir string) []Group {
	paths, err := filepath.Glob(filepath.Join(dir, "*.js"))
	if err != nil {
		return nil
	}
	sort.Strings(paths)

	groups := make([]Group, 0, len(paths))
	for _, path := range paths {
		found := read(path)
		if len(found) == 0 {
			continue
		}
		groups = append(groups, Group{Module: module(path), Hooks: found})
	}
	return groups
}

func read(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	for _, match := range site.FindAllStringSubmatch(string(data), -1) {
		name := match[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

// module is the file name without its ordering prefix: 50-health.js is health,
// the same name the host's --skip takes.
func module(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), ".js")
	return strings.TrimLeft(name, "0123456789-")
}
