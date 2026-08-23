package registry

import (
	"strings"
	"testing"

	"github.com/ancaria-dev/launcher/mods"
)

// here is the launcher these tests measure an index against: this API contract,
// and a release number the loader ranges below are written around.
var here = mods.Loader{API: mods.API, Version: "0.1.20"}

const good = `{
  "srml": 1,
  "name": "Ancaria",
  "url": "https://github.com/ancaria-dev/mods",
  "mods": [
    {"id": "old-huge-potions", "name": "Old Huge Potions", "version": "0.1.0",
     "api": "1", "icon": "old-huge-potions/icon.png",
     "file": "old-huge-potions-0.1.0.jar", "size": 4292,
     "sha256": "c6c88b1f47c9bb5760d943403c32f2f8b6ed432b123a9dde002685ae31c6e45e",
     "url": "https://example.invalid/old-huge-potions-0.1.0.jar"}
  ]
}`

func TestParseReadsAnIndex(t *testing.T) {
	index, dropped, err := Parse([]byte(good))
	if err != nil || dropped != 0 {
		t.Fatalf("Parse gave %v, %d dropped", err, dropped)
	}
	if index.Name != "Ancaria" || len(index.Mods) != 1 {
		t.Fatalf("read %+v", index)
	}
	if got := index.Mods[0].Jar(); got != "old-huge-potions-0.1.0.jar" {
		t.Fatalf("the jar would be saved as %q", got)
	}
}

func TestParseRefusesWhatItCannotTrust(t *testing.T) {
	for name, body := range map[string]string{
		"not an index":   `{"name": "x", "mods": []}`,
		"a newer layout": `{"srml": 2, "name": "x", "mods": []}`,
		"no name":        `{"srml": 1, "mods": []}`,
		"not even json":  `<html>404</html>`,
	} {
		if _, _, err := Parse([]byte(body)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestParseDropsEntriesThatCouldNotBeInstalled(t *testing.T) {
	// One of each: an id that would escape the mods folder, a version that
	// would, a checksum that cannot be one, a URL that is not https, and the
	// same id twice.
	body := `{"srml": 1, "name": "x", "mods": [
	  {"id": "../evil", "version": "1", "sha256": "` + strings.Repeat("a", 64) + `",
	   "size": 1, "url": "https://e.invalid/a.jar"},
	  {"id": "ok-one", "version": "../../1", "sha256": "` + strings.Repeat("a", 64) + `",
	   "size": 1, "url": "https://e.invalid/a.jar"},
	  {"id": "ok-two", "version": "1", "sha256": "nope", "size": 1,
	   "url": "https://e.invalid/a.jar"},
	  {"id": "ok-three", "version": "1", "sha256": "` + strings.Repeat("a", 64) + `",
	   "size": 1, "url": "http://e.invalid/a.jar"},
	  {"id": "ok-four", "version": "1", "sha256": "` + strings.Repeat("a", 64) + `",
	   "size": 1, "url": "https://e.invalid/a.jar"},
	  {"id": "ok-four", "version": "2", "sha256": "` + strings.Repeat("b", 64) + `",
	   "size": 1, "url": "https://e.invalid/b.jar"}
	]}`
	index, dropped, err := Parse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 5 || len(index.Mods) != 1 || index.Mods[0].ID != "ok-four" {
		t.Fatalf("kept %+v, dropped %d", index.Mods, dropped)
	}
}

func TestParseThrowsAwayAPathThatLeavesTheRepository(t *testing.T) {
	body := `{"srml": 1, "name": "x", "icon": "../../secrets.png", "mods": [
	  {"id": "one", "version": "1", "icon": "/etc/passwd", "source": "..\\up",
	   "sha256": "` + strings.Repeat("a", 64) + `", "size": 1,
	   "url": "https://e.invalid/a.jar"}]}`
	index, _, err := Parse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if index.Icon != "" || index.Mods[0].Icon != "" || index.Mods[0].Source != "" {
		t.Fatalf("a path out of the repository survived: %+v", index)
	}
}

func TestNormalizeWantsACloneURL(t *testing.T) {
	url, err := Normalize("  https://github.com/ancaria-dev/mods.git/ ")
	if err != nil || url != "https://github.com/ancaria-dev/mods.git" {
		t.Fatalf("got %q, %v", url, err)
	}
	for _, bad := range []string{
		"",
		"github.com/ancaria-dev/mods.git",
		"http://github.com/ancaria-dev/mods.git",
		"git@github.com:ancaria-dev/mods.git",
		"https://github.com/ancaria-dev/mods",
		"https://github.com/mods.git",
	} {
		if _, err := Normalize(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestCandidatesKnowGitHubAndGuessTheRest(t *testing.T) {
	github := Candidates("https://github.com/ancaria-dev/mods.git")
	if len(github) != 2 ||
		!strings.HasPrefix(github[0], "https://raw.githubusercontent.com/ancaria-dev/mods/HEAD/") ||
		!strings.Contains(github[1], "api.github.com") {
		t.Fatalf("github shapes are %v", github)
	}
	other := Candidates("https://codeberg.org/someone/mods.git")
	if len(other) != 3 || !strings.Contains(other[1], "/raw/branch/main/") {
		t.Fatalf("other shapes are %v", other)
	}
}

func TestLabelIsShortEnoughToPrintUnderAName(t *testing.T) {
	remote := Remote{URL: "https://github.com/ancaria-dev/mods.git"}
	if got := remote.Label(); got != "github.com/ancaria-dev/mods" {
		t.Fatalf("label is %q", got)
	}
}

// The rule this whole feature turns on: a pair that cannot work together stops
// being offered at all, rather than being offered with a warning nobody reads.
func TestMergeHidesBothSidesOfAConflict(t *testing.T) {
	remote := Remote{URL: "u", Raw: "r"}
	indexes := map[string]*Index{"u": {Mods: []Entry{
		{ID: "self-check", API: "1", Conflicts: []string{"old-huge-potions"}},
		{ID: "old-huge-potions", API: "1"},
		{ID: "all-my-runes", API: "1"},
	}}}
	offers, _ := Merge(here, nil, indexes, []Remote{remote})
	if len(offers) != 1 || offers[0].ID != "all-my-runes" {
		t.Fatalf("offered %+v", offers)
	}
}

func TestMergeKeepsAwayFromWhatIsAlreadyInstalled(t *testing.T) {
	remote := Remote{URL: "u"}
	indexes := map[string]*Index{"u": {Mods: []Entry{
		{ID: "self-check", API: "1", Version: "0.2.0"},
		{ID: "old-huge-potions", API: "1"},
		{ID: "all-my-runes", API: "1"},
	}}}
	installed := []mods.Mod{
		{ID: "self-check", Version: "0.1.0", Conflicts: []string{"all-my-runes"}},
	}
	offers, known := Merge(here, installed, indexes, []Remote{remote})

	// self-check is installed, so it is not on offer; all-my-runes is refused by
	// what is installed; old-huge-potions is the only thing left.
	if len(offers) != 1 || offers[0].ID != "old-huge-potions" {
		t.Fatalf("offered %+v", offers)
	}
	if known["self-check"].Update != "0.2.0" {
		t.Fatalf("no update was noticed: %+v", known["self-check"])
	}
}

func TestMergeSaysNothingWhenTwoRepositoriesOfferOneId(t *testing.T) {
	one := Remote{URL: "one"}
	two := Remote{URL: "two"}
	indexes := map[string]*Index{
		"one": {Mods: []Entry{{ID: "same", API: "1", Version: "1"}}},
		"two": {Mods: []Entry{{ID: "same", API: "1", Version: "2"}}},
	}
	offers, _ := Merge(here, nil, indexes, []Remote{one, two})
	if len(offers) != 0 {
		t.Fatalf("offered %+v", offers)
	}
	installed := []mods.Mod{{ID: "same", Version: "1"}}
	_, known := Merge(here, installed, indexes, []Remote{one, two})
	if known["same"].Update != "" {
		t.Fatalf("an update was offered from an ambiguous pair: %+v", known["same"])
	}
}

func TestMergeShowsAModBuiltForAnotherLoader(t *testing.T) {
	indexes := map[string]*Index{"u": {Mods: []Entry{{ID: "old", API: "0"}}}}
	offers, _ := Merge(here, nil, indexes, []Remote{{URL: "u"}})
	if len(offers) != 1 || offers[0].Supported {
		t.Fatalf("offered %+v", offers)
	}
}

// The other half of a pin: a mod that names a launcher release rather than an
// API contract. It is offered and it is not installable, with the range it
// asked for in the sentence under it.
func TestMergeRefusesAnOfferPinnedToANewerLauncher(t *testing.T) {
	indexes := map[string]*Index{"u": {Mods: []Entry{
		{ID: "future", API: "[1,2)", Loader: "[0.2.0,)"},
		{ID: "present", API: "[1,2)", Loader: "[0.1.0,)"},
	}}}
	offers, _ := Merge(here, nil, indexes, []Remote{{URL: "u"}})
	if len(offers) != 2 {
		t.Fatalf("offered %+v", offers)
	}
	by := map[string]Offer{}
	for _, offer := range offers {
		by[offer.ID] = offer
	}
	if !by["present"].Supported || by["present"].Refusal != "" {
		t.Errorf("a mod this launcher satisfies was refused: %+v", by["present"])
	}
	if by["future"].Supported {
		t.Errorf("a mod pinned above this launcher was offered: %+v", by["future"])
	}
	if !strings.Contains(by["future"].Refusal, "[0.2.0,)") {
		t.Errorf("the refusal does not quote the range: %q", by["future"].Refusal)
	}
}
