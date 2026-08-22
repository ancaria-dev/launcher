package hooks

import (
	"path/filepath"
	"testing"
)

// The scanner reads the agent by pattern rather than from a list, so the thing
// worth testing is that the pattern still sees every module that installs
// something -- a site it misses is a site nobody can switch off.
func TestScanFindsEveryModule(t *testing.T) {
	// The staged payload rather than the coderpack checkout: this is the agent
	// that ships inside the executable, so the pair being tested is the pair a
	// player gets.
	const agent = "../install/payload/agent"
	// The directory itself is tracked and therefore always there; what a
	// clean checkout is missing is the scripts inside it. Stat would pass on
	// the empty folder and the test would fail as "no hooks found" instead of
	// skipping, which reads like a broken scanner rather than an absent build.
	staged, err := filepath.Glob(filepath.Join(agent, "*.js"))
	if err != nil || len(staged) == 0 {
		t.Skip("no staged payload; run tools/build.ps1 first")
	}
	groups := Scan(agent)
	if len(groups) == 0 {
		t.Fatal("no hooks found; the agent sources moved or the pattern broke")
	}

	found := map[string][]string{}
	for _, group := range groups {
		found[group.Module] = group.Hooks
	}
	for _, want := range []struct{ module, hook string }{
		{"core", "heroCapture"},
		{"session", "worldLoad"},
		{"health", "hpDamage"},   // installed through a wrapper, not hook()
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
