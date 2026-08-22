package pin

import "testing"

// The corpus below is the specification, and it is deliberately the same set of
// cases the Java side checks: `Ranges` in coderpack's zygote and `Ranges` in the
// build repository's linter answer the same questions, and there is no shared
// source any of the three could read. A case added here belongs in both of them.

func TestCompare(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
	}{
		{"1", "1", 0},
		{"1", "1.0.0", 0},
		{"1.0", "1.0.0.0", 0},
		{"1.2.3", "1.2.4", -1},
		{"1.10", "1.9", 1},
		{"2", "10", -1},
		{"0.1.20", "0.1.9", 1},
		{"25.0.4.1", "25.0.4", 1},
		// A qualifier is a release that has not happened yet.
		{"1.0.0-rc1", "1.0.0", -1},
		{"1.0.0-rc1", "1.0.0-rc2", -1},
		{"1.0.0+1", "1.0.0", -1},
	}
	for _, one := range cases {
		got := Parse(one.left).Compare(Parse(one.right))
		if got != one.want {
			t.Errorf("Parse(%q).Compare(%q) = %d, wanted %d", one.left, one.right, got, one.want)
		}
		if back := Parse(one.right).Compare(Parse(one.left)); back != -one.want {
			t.Errorf("Parse(%q).Compare(%q) = %d, wanted %d", one.right, one.left, back, -one.want)
		}
	}
}

func TestUnknownVersions(t *testing.T) {
	for _, text := range []string{"", "  ", "x", "1.x", "1..2", "-1", ".", "1.-2", "v1"} {
		if version := Parse(text); version.Known() {
			t.Errorf("Parse(%q) read as %+v, wanted nothing", text, version)
		}
	}
	for _, text := range []string{"1", "0.1.20", "1.0.0-rc1", "25.0.4.1+1", `"1"`, " 2.0 "} {
		if !Parse(text).Known() {
			t.Errorf("Parse(%q) read as nothing", text)
		}
	}
}

func TestRanges(t *testing.T) {
	cases := []struct {
		rang, version string
		want          bool
	}{
		// The usual one: a whole major.
		{"[1,2)", "1", true},
		{"[1,2)", "1.4.0", true},
		{"[1,2)", "2", false},
		{"[1,2)", "0.9", false},
		// No upper end.
		{"[0.1.20,)", "0.1.20", true},
		{"[0.1.20,)", "0.2.0", true},
		{"[0.1.20,)", "0.1.19", false},
		// No lower end.
		{"(,1.5]", "1.5", true},
		{"(,1.5]", "1.5.1", false},
		{"(,1.5)", "1.5", false},
		// Exactly one, both ways of writing it.
		{"[1.2]", "1.2", true},
		{"[1.2]", "1.2.0", true},
		{"[1.2]", "1.2.1", false},
		{"1", "1", true},
		{"1", "1.0", true},
		{"1", "2", false},
		// A union, and the hole in the middle of it.
		{"[1,2),[3,4)", "1.5", true},
		{"[1,2),[3,4)", "3.0", true},
		{"[1,2),[3,4)", "2.5", false},
		{"[1,2), [3,4)", "3.9", true},
		// Nothing said allows everything; nothing readable allows nothing.
		{"", "1", true},
		{"[1,2)", "", false},
		{"[1,2)", "x", false},
	}
	for _, one := range cases {
		got := Allows(one.rang, one.version)
		if got != one.want {
			t.Errorf("Allows(%q, %q) = %v, wanted %v", one.rang, one.version, got, one.want)
		}
	}
}

func TestBadRanges(t *testing.T) {
	for _, text := range []string{
		"[1,2",       // never closed
		"1,2",        // a comma outside brackets
		"[2,1)",      // backwards
		"(,)",        // everything, written at length
		"[1,2)[3,4)", // no comma between them
		"[1,2),",     // a comma with nothing after it
		"[x,2)",      // not a version
		"(1.2)",      // one version needs square brackets
		"[]",         // nothing at all
		"[1,2}",      // not a bracket this notation knows
	} {
		if _, err := ParseRange(text); err == nil {
			t.Errorf("ParseRange(%q) was accepted", text)
		}
	}
}

// An empty range is not the same as a range that matched, and the two have to
// stay distinguishable: one of them is a line somebody left out.
func TestAnyIsNotTheSameAsMatching(t *testing.T) {
	empty, err := ParseRange("")
	if err != nil || !empty.Any() {
		t.Fatalf("empty range: %+v, %v", empty, err)
	}
	if !empty.Has(Parse("1")) || !empty.Has(Version{}) {
		t.Fatal("an empty range should hold everything, an unknown version included")
	}
	full, err := ParseRange("[1,2)")
	if err != nil || full.Any() {
		t.Fatalf("[1,2): %+v, %v", full, err)
	}
}

// The text comes back as it was written, because every message built out of one
// of these quotes what somebody put in their build file.
func TestTextIsKept(t *testing.T) {
	rang, err := ParseRange(" [1,2) ")
	if err != nil {
		t.Fatal(err)
	}
	if rang.String() != "[1,2)" {
		t.Errorf("String() = %q", rang.String())
	}
	if got := Parse(" 1.0.0-rc1 ").String(); got != "1.0.0-rc1" {
		t.Errorf("String() = %q", got)
	}
}
