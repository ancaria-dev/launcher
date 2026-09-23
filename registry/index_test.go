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

// A declared conflict is a caution, not a gate. Both sides stay installable
// and both are told, because the combination is what misbehaves and the mod
// that stayed quiet is half of it.
//
// This used to hide both. One mod naming two others emptied three quarters of
// the default repository, and a player saw one mod out of four with nothing
// on the page to say why.
func TestMergeOffersBothSidesOfAConflictAndNamesIt(t *testing.T) {
	remote := Remote{URL: "u", Raw: "r"}
	indexes := map[string]*Index{"u": {Mods: []Entry{
		{ID: "self-check", Name: "Self Check", API: "3",
			Conflicts: []string{"old-huge-potions"}},
		{ID: "old-huge-potions", Name: "Old Huge Potions", API: "3"},
		{ID: "all-my-runes", Name: "All My Runes", API: "3"},
	}}}
	offers, _ := Merge(here, nil, indexes, []Remote{remote})
	if len(offers) != 3 {
		t.Fatalf("offered %+v", offers)
	}
	by := map[string]Offer{}
	for _, offer := range offers {
		by[offer.ID] = offer
	}
	for _, offer := range offers {
		if !offer.Supported {
			t.Errorf("a caution is not a refusal: %+v", offer)
		}
	}
	// Named, not identified: the sentence is read by somebody who knows the
	// mod by the name on its row.
	if !strings.Contains(by["self-check"].Caution, "Old Huge Potions") {
		t.Errorf("self-check says %q", by["self-check"].Caution)
	}
	// The other half never said anything and is told anyway.
	if !strings.Contains(by["old-huge-potions"].Caution, "Self Check") {
		t.Errorf("old-huge-potions says %q", by["old-huge-potions"].Caution)
	}
	if by["all-my-runes"].Caution != "" {
		t.Errorf("a mod in no conflict was cautioned: %q", by["all-my-runes"].Caution)
	}
}

// The sentence itself, both shapes of it. Written out in full because it is
// what a player reads, and because assembling it from a count is exactly the
// kind of code that produces "Both can all be installed".
func TestCautionReadsAsASentence(t *testing.T) {
	one := &Clash{}
	one.Add("a", "Alpha", []string{"b"})
	one.Add("b", "Beta", nil)
	want := "May not work correctly alongside Beta. Both can be installed; " +
		"turn one off if the game misbehaves."
	if got := one.Sentence("a"); got != want {
		t.Errorf("one conflict reads %q, want %q", got, want)
	}

	many := &Clash{}
	many.Add("a", "Alpha", []string{"b", "c"})
	many.Add("b", "Beta", nil)
	many.Add("c", "Gamma", nil)
	want = "May not work correctly alongside Beta and Gamma. They can all be " +
		"installed; turn one off if the game misbehaves."
	if got := many.Sentence("a"); got != want {
		t.Errorf("two conflicts read %q, want %q", got, want)
	}
}

// A mod naming something no repository offers and nobody has installed is
// naming nothing the player can act on.
func TestMergeSaysNothingAboutAConflictNobodyCanSee(t *testing.T) {
	indexes := map[string]*Index{"u": {Mods: []Entry{
		{ID: "lonely", Name: "Lonely", API: "3", Conflicts: []string{"absent"}},
	}}}
	offers, _ := Merge(here, nil, indexes, []Remote{{URL: "u"}})
	if len(offers) != 1 || offers[0].Caution != "" {
		t.Fatalf("offered %+v", offers)
	}
}

func TestMergeKeepsAwayFromWhatIsAlreadyInstalled(t *testing.T) {
	remote := Remote{URL: "u"}
	indexes := map[string]*Index{"u": {Mods: []Entry{
		{ID: "self-check", API: "3", Version: "0.2.0"},
		{ID: "old-huge-potions", API: "3"},
		{ID: "all-my-runes", API: "3"},
	}}}
	installed := []mods.Mod{
		{ID: "self-check", Version: "0.1.0", Conflicts: []string{"all-my-runes"}},
	}
	offers, known := Merge(here, installed, indexes, []Remote{remote})

	// self-check is installed, so it is not on offer. The other two are, and
	// the one it named carries the sentence rather than disappearing.
	if len(offers) != 2 {
		t.Fatalf("offered %+v", offers)
	}
	by := map[string]Offer{}
	for _, offer := range offers {
		by[offer.ID] = offer
	}
	if by["all-my-runes"].Caution == "" {
		t.Error("a mod an installed one named was offered with nothing said")
	}
	if by["old-huge-potions"].Caution != "" {
		t.Errorf("an unrelated mod was cautioned: %q", by["old-huge-potions"].Caution)
	}
	if known["self-check"].Update != "0.2.0" {
		t.Fatalf("no update was noticed: %+v", known["self-check"])
	}
}

func TestMergeSaysNothingWhenTwoRepositoriesOfferOneId(t *testing.T) {
	one := Remote{URL: "one"}
	two := Remote{URL: "two"}
	indexes := map[string]*Index{
		"one": {Mods: []Entry{{ID: "same", API: "3", Version: "1"}}},
		"two": {Mods: []Entry{{ID: "same", API: "3", Version: "2"}}},
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
		{ID: "future", API: "[3,4)", Loader: "[0.2.0,)"},
		{ID: "present", API: "[3,4)", Loader: "[0.1.0,)"},
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
