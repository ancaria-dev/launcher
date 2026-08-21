package game

import (
	"os"
	"path/filepath"
	"testing"
)

// A player's folder is not guaranteed to hold the executable this project was
// built against, nor to spell it the way this project spells it.
func TestFindPrefersTheFirstNameAndIgnoresCase(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "sacred.exe")
	if got := filepath.Base(Find(dir)); got != "sacred.exe" {
		t.Errorf("lowercase Sacred.exe was not found, got %q", got)
	}

	write(t, dir, "PUREHD.EXE")
	if got := filepath.Base(Find(dir)); got != "PUREHD.EXE" {
		t.Errorf("pureHD.exe should win over Sacred.exe, got %q", got)
	}
}

func TestFindSaysNothingWhenTheFolderIsNotTheGame(t *testing.T) {
	if got := Find(t.TempDir()); got != "" {
		t.Errorf("an empty folder is not a game folder, got %q", got)
	}
}

func write(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// The version is what decides, not the name: the folder a player actually has
// may hold a renamed copy of the wrapper or the stock game under a name we try
// second.  A file with no version resource -- which is what an empty stand-in
// is -- is not the expected build either.
func TestDescribeNamesBothBuilds(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "pureHD.exe")

	found := Describe(dir)
	if found.Exe != "pureHD.exe" {
		t.Errorf("the game in the folder is %q", found.Exe)
	}
	if found.ExpectedExe != Names[0] || found.Expected != Expected {
		t.Errorf("the expected build has to be named too, got %+v", found)
	}
	if found.Matches {
		t.Error("a file with no version resource is not the expected build")
	}
}

func TestDescribeSaysNothingAboutAFolderWithNoGame(t *testing.T) {
	found := Describe(t.TempDir())
	if found.Exe != "" || found.Matches {
		t.Errorf("an empty folder holds no build, got %+v", found)
	}
}

func TestVersionIsEmptyForAnythingWithoutOne(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Sacred.exe")
	if got := Version(filepath.Join(dir, "Sacred.exe")); got != "" {
		t.Errorf("an empty file has no version, got %q", got)
	}
	if got := Version(filepath.Join(dir, "gone.exe")); got != "" {
		t.Errorf("a file that is not there has no version, got %q", got)
	}
}
