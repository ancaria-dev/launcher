package hooks

import (
	"os"
	"testing"
)

func TestParseKeepsOnlyRowsWithSomethingInThem(t *testing.T) {
	groups := parse([]byte(`[
		{"module":"core","hooks":["heroCapture"]},
		{"module":"level","hooks":[]},
		{"module":"","hooks":["orphan"]},
		{"module":"gold","hooks":["goldDelta"]}
	]`))
	if len(groups) != 2 {
		t.Fatalf("expected two rows, got %v", groups)
	}
	if groups[0].Module != "core" || groups[1].Module != "gold" {
		t.Errorf("load order is not preserved: %v", groups)
	}
}

func TestParseSurvivesRubbish(t *testing.T) {
	for _, data := range []string{"", "not json", "{}", "null"} {
		if groups := parse([]byte(data)); len(groups) != 0 {
			t.Errorf("%q produced %v", data, groups)
		}
	}
}

// The pair being tested is the pair a player gets: the host that ships inside
// this executable, asked the way the launcher asks it. A site the host does not
// name is a site nobody can switch off, so what matters is that every module
// that installs something is still in the answer.
func TestTheStagedHostNamesEveryModule(t *testing.T) {
	const exe = "../install/payload/protocol.exe"
	if _, err := os.Stat(exe); err != nil {
		t.Skip("no staged payload; run tools/build.ps1 first")
	}
	groups := Ask(exe)
	if len(groups) == 0 {
		t.Fatal("the host named no hooks; the agent moved or --hooks broke")
	}

	found := map[string][]string{}
	for _, group := range groups {
		found[group.Module] = group.Hooks
	}
	for _, want := range []struct{ module, hook string }{
		{"core", "heroCapture"},
		{"session", "worldLoad"},
		{"health", "hpDamage"}, // installed through a wrapper, not hook()
		{"position", "posCamera"},
		{"gold", "goldDelta"},
		{"exp", "expWrite"},
		{"skills", "skillWrite"},
		{"attrs", "statGrant"},
		// No level row: the level write crashes the game on save load, so that
		// module reads the field on the tick and installs nothing.
		{"items", "itemPickup"},
	} {
		if !has(found[want.module], want.hook) {
			t.Errorf("%s is missing %s, got %v", want.module, want.hook, found[want.module])
		}
	}
}

func has(names []string, name string) bool {
	for _, found := range names {
		if found == name {
			return true
		}
	}
	return false
}
