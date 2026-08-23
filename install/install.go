// Package install carries the whole loader inside the executable and lays it
// out in the game folder.
//
// The player copies one file into Sacred Gold and runs it. Everything the
// loader needs -- the host, the agent scripts, the jars, the stock mods -- is
// embedded here and written out on first run, and again whenever the version
// changes. Nothing has to be installed in the right order, and there is no
// second file to forget.
package install

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ancaria-dev/launcher/game"
)

// payload is assembled by tools/build.ps1.  Everything under coderpack/ goes to
// <game>/launcher and everything under mods/ to <game>/mods.
//
//go:embed all:payload
var payload embed.FS

const root = "payload"

// Game is a Sacred Gold folder: the one the executable sits in.
type Game struct {
	Dir string
}

// Find locates the game around the executable.  Being in the wrong folder is
// the one mistake a player will actually make, so it is worth a clear error
// rather than a mysterious failure to attach later on.
func Find() (*Game, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(exe)
	if game.Find(dir) == "" {
		return nil, errors.New(
			"No game was found next to this file (" + game.Missing() + "). Move " +
				"Sacred Mod Loader.exe into the Sacred Gold folder that contains " +
				"the game executable")
	}
	return &Game{Dir: dir}, nil
}

// Version is the build stamped into the payload.
func Version() string {
	data, err := payload.ReadFile(root + "/VERSION")
	if err != nil {
		return "dev"
	}
	return strings.TrimSpace(string(data))
}

// Installed is the version already in the game folder, empty if none.
func (g *Game) Installed() string {
	data, err := os.ReadFile(filepath.Join(g.Dir, "launcher", "VERSION"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// Unpack writes the payload out when the installed version is not this one.
//
// Mod jars are overwritten like everything else, but files a mod created next
// to them -- a rune table somebody filled in by hand -- are not touched: only
// what is in the payload is written, never a whole directory replaced.
func (g *Game) Unpack() (bool, error) {
	// Made here rather than carried in the payload. This launcher ships no mod,
	// so payload/mods is empty, and an empty directory does not survive
	// `go:embed` -- there is nothing in the embedded tree to walk. The folder
	// still has to exist: it is where the loader looks and where the game folder
	// tells a player their mods live, whether or not any have been installed.
	if err := os.MkdirAll(g.ModsDir(), 0o755); err != nil {
		return false, err
	}
	if g.Installed() == Version() {
		return false, nil
	}
	err := fs.WalkDir(payload, root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		target := filepath.Join(g.Dir, "launcher", relative)
		// mods/ belongs to the game folder, not inside coderpack/: a player looking
		// for their mods should find them without opening ours.
		if head, rest, found := strings.Cut(filepath.ToSlash(relative), "/"); found && head == "mods" {
			target = filepath.Join(g.Dir, "mods", rest)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		// The payload is a staged copy of a build directory and picks up the
		// odd .gitkeep on the way; a game folder should not collect them.
		if strings.HasPrefix(entry.Name(), ".") {
			return nil
		}
		data, err := payload.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return write(target, data)
	})
	return err == nil, err
}

// write replaces a file, tolerating the case where it is currently running:
// the old one is renamed out of the way first, which Windows allows even for a
// locked executable, and cleaned up on the next launch.
func write(target string, data []byte) error {
	if err := os.WriteFile(target, data, 0o755); err == nil {
		return nil
	}
	stale := target + ".old"
	_ = os.Remove(stale)
	if err := os.Rename(target, stale); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o755)
}

// LoaderDir is where the host, the agent and the jars live: `<game>/launcher`.
func (g *Game) LoaderDir() string {
	return filepath.Join(g.Dir, "launcher")
}

// ModsDir is where mod jars live.
func (g *Game) ModsDir() string {
	return filepath.Join(g.Dir, "mods")
}
