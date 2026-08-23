package install

import (
	"os"
	"path/filepath"
	"testing"
)

// The mods folder is the one thing about the layout that cannot come out of
// the payload. This launcher ships no mod, so `payload/mods` is empty, and an
// empty directory is not in an embedded tree at all -- there is nothing to walk
// and nothing would be created.
func TestUnpackAlwaysMakesTheModsFolder(t *testing.T) {
	dir := t.TempDir()
	// Say this version is installed already, so the payload is not written and
	// the only thing left is the thing that has to happen every time.
	loader := filepath.Join(dir, "launcher")
	if err := os.MkdirAll(loader, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(loader, "VERSION"), []byte(Version()), 0o644); err != nil {
		t.Fatal(err)
	}

	game := &Game{Dir: dir}
	unpacked, err := game.Unpack()
	if err != nil {
		t.Fatal(err)
	}
	if unpacked {
		t.Fatal("it wrote the payload over a folder that already had this version")
	}
	if _, err := os.Stat(game.ModsDir()); err != nil {
		t.Fatalf("no mods folder: %v", err)
	}
}
