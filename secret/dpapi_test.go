package secret

import "testing"

func TestRoundTrip(t *testing.T) {
	if !Works() {
		t.Skip("no DPAPI here")
	}
	sealed := Seal("ghp_example_token")
	if sealed == "ghp_example_token" {
		t.Fatal("the token was written down as it was typed")
	}
	if got := Open(sealed); got != "ghp_example_token" {
		t.Fatalf("Open gave %q", got)
	}
}

func TestPlainValueSurvives(t *testing.T) {
	if got := Open("typed-by-hand"); got != "typed-by-hand" {
		t.Fatalf("a value with no marker came back as %q", got)
	}
	if Seal("") != "" || Open("") != "" {
		t.Fatal("an empty token became something")
	}
}
