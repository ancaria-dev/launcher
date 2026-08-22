package mods

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// here is a launcher to measure descriptors against: this API contract, and a
// release number the loader ranges in these tests are written around.
var here = Loader{API: API, Version: "0.1.20"}

// jar writes a jar carrying nothing but a descriptor, which is all Scan reads.
func jar(t *testing.T, dir, name, toml string) {
	t.Helper()
	file, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	archive := zip.NewWriter(file)
	entry, err := archive.Create(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(toml)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestScanReadsTheApiVersion(t *testing.T) {
	dir := t.TempDir()
	jar(t, dir, "ours.jar", `
id = "ours"
name = "Ours"
version = "1.0.0"
description = "Built against this loader"
entrypoint = "demo.Ours"
api = "`+API+`"
`)
	jar(t, dir, "theirs.jar", `
id = "theirs"
name = "Theirs"
entrypoint = "demo.Theirs"
api = "99"
`)
	// No api line at all: written by hand, or by a plugin older than the field.
	// The loader refuses it, so the launcher must not offer it either.
	jar(t, dir, "ancient.jar", `
id = "ancient"
name = "Ancient"
entrypoint = "demo.Ancient"
`)

	found := here.Scan(dir)
	if len(found) != 3 {
		t.Fatalf("wanted all three mods listed, got %d: %v", len(found), found)
	}

	want := map[string]bool{"ours": true, "theirs": false, "ancient": false}
	for _, mod := range found {
		supported, known := want[mod.ID]
		if !known {
			t.Fatalf("unexpected mod %q", mod.ID)
		}
		if mod.Supported != supported {
			t.Errorf("%s: supported is %v, wanted %v (api %q)",
				mod.ID, mod.Supported, supported, mod.API)
		}
	}
}

// The incompatible ones keep their name and description: the row a player reads
// is the same row, with a reason under it.
func TestIncompatibleModStillDescribesItself(t *testing.T) {
	dir := t.TempDir()
	jar(t, dir, "theirs.jar", `
id = "theirs"
name = "Theirs"
description = "Something a player picked out"
entrypoint = "demo.Theirs"
api = "99"
`)

	found := here.Scan(dir)
	if len(found) != 1 {
		t.Fatalf("wanted one mod, got %d", len(found))
	}
	mod := found[0]
	if mod.Name != "Theirs" || mod.Description != "Something a player picked out" {
		t.Errorf("the row lost its text: %+v", mod)
	}
	if mod.API != "99" || mod.Supported {
		t.Errorf("wanted an unsupported mod declaring api 99, got %+v", mod)
	}
}

// A conflict travels in the descriptor rather than only in a registry index, so
// it still holds for a jar somebody dropped into the folder by hand.
func TestConflictsAreReadOffTheDescriptor(t *testing.T) {
	dir := t.TempDir()
	jar(t, dir, "one.jar", `
id = "one"
entrypoint = "demo.One"
api = "1"
conflicts = ["two", "three"]
`)
	jar(t, dir, "four.jar", `
id = "four"
entrypoint = "demo.Four"
api = "1"
`)

	found := here.Scan(dir)
	if len(found) != 2 {
		t.Fatalf("wanted two mods, got %d", len(found))
	}
	if got := found[1].Conflicts; len(got) != 2 || got[0] != "two" || got[1] != "three" {
		t.Errorf("read %v", got)
	}
	if found[0].Conflicts != nil {
		t.Errorf("a mod that named none has %v", found[0].Conflicts)
	}
}

// The point of a range: a mod that says it runs on this API and the next one
// keeps running when the next one arrives, and one that names a release the
// player has not got says so rather than failing silently at load time.
func TestRangesDecideWhatLoads(t *testing.T) {
	cases := []struct {
		name      string
		api       string
		loader    string
		supported bool
	}{
		{"the usual major", "[1,2)", "", true},
		{"exactly this one", "[1]", "", true},
		{"a bare number, the old spelling", "1", "", true},
		{"two majors at once", "[1,3)", "", true},
		{"the next major only", "[2,3)", "", false},
		{"everything before this one", "(,1)", "", false},
		{"a release this launcher has", "[1,2)", "[0.1.20,)", true},
		{"a release older than this one", "[1,2)", "(,0.1.0]", false},
		{"a release not out yet", "[1,2)", "[0.2.0,)", false},
		{"a range nobody can read", "[1,2", "", false},
		{"a loader range nobody can read", "[1,2)", "2,3", false},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			dir := t.TempDir()
			jar(t, dir, "one.jar", "id = \"one\"\nentrypoint = \"demo.One\"\napi = \""+
				one.api+"\"\nloader = \""+one.loader+"\"\n")
			found := here.Scan(dir)
			if len(found) != 1 {
				t.Fatalf("wanted one mod, got %d", len(found))
			}
			if found[0].Supported != one.supported {
				t.Fatalf("supported is %v, wanted %v (refusal %q)",
					found[0].Supported, one.supported, found[0].Refusal)
			}
			if !one.supported && found[0].Refusal == "" {
				t.Fatal("refused without saying why")
			}
			if one.supported && found[0].Refusal != "" {
				t.Fatalf("supported and yet %q", found[0].Refusal)
			}
		})
	}
}

// A launcher that cannot read its own version is a bug in the launcher, and
// refusing every mod over it would be exactly the wrong way round.
func TestAnUnknownLauncherVersionRefusesNothing(t *testing.T) {
	nameless := Loader{API: API}
	if refusal := nameless.Refuse("[1,2)", "[9.9.9,)"); refusal != "" {
		t.Fatalf("refused: %s", refusal)
	}
	// The API contract is not excused by it, though: that one decides whether
	// the code can run at all.
	if nameless.Refuse("[2,3)", "") == "" {
		t.Fatal("an API mismatch has nothing to do with the release number")
	}
}

// Both refusals name the range that was written, because that string is what
// somebody has to go and change.
func TestARefusalQuotesWhatTheModAskedFor(t *testing.T) {
	if got := here.Refuse("[2,3)", ""); !strings.Contains(got, "[2,3)") {
		t.Errorf("api refusal was %q", got)
	}
	if got := here.Refuse("[1,2)", "[0.2.0,)"); !strings.Contains(got, "[0.2.0,)") {
		t.Errorf("loader refusal was %q", got)
	}
	if got := here.Refuse("", ""); !strings.Contains(got, "does not declare") {
		t.Errorf("a descriptor with no api line got %q", got)
	}
}
