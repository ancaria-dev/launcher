// Package purehd fetches the build every address in the loader was found in,
// for a player whose folder has some other one.
//
// The wrapper is two files, `pureHD.exe` and `pHD.dll`, and both go beside the
// game rather than over it.  The stock Sacred.exe is never touched: game.Find
// prefers pureHD.exe once it is there, so adding it is enough, and deleting the
// two files is the whole way back.
package purehd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ancaria-dev/launcher/fetch"
	"github.com/ancaria-dev/launcher/game"
	"github.com/ancaria-dev/launcher/java"
)

// The archive, pinned.  It is served from ancaria.dev and not from a GitHub
// release, and the digest is what makes that safe: a file that changes on the
// server without a launcher release saying so is refused, not installed.
const (
	URL      = "https://ancaria.dev/files/sacred.purehd.zip"
	Checksum = "4206de65c0e3af2f99a080ce0a92e52aad3f04636808d17ec76c2c079bdf4ecd"
	Size     = 3577484
)

// The stages a player is shown, in the order they happen.
const (
	StageIdle     = ""
	StageDownload = "download"
	StageExtract  = "extract"
	StageInstall  = "install"
	StageDone     = "done"
	StageFailed   = "failed"
	StageAborted  = "aborted"
)

// Progress is the state of the one job that can be running.
type Progress struct {
	Stage string `json:"stage"`
	Note  string `json:"note"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Error string `json:"error"`
}

// Snapshot is everything the page asks for in one poll.  Build is read again
// after an install, so the strip under the header can go away without a
// restart.
type Snapshot struct {
	Build    game.Build
	Progress Progress
	Busy     bool
	// Abortable is false once the files are being moved into the game folder.
	// Stopping halfway through that is how a folder ends up with a new exe and
	// an old DLL.
	Abortable bool
}

// Work owns the one download that may be changing the game folder.
type Work struct {
	gameDir   string
	loaderDir string

	// Where the archive comes from and what it has to be.  Fields rather than
	// the constants only so a test can serve its own.
	url     string
	sum     string
	size    int64
	version func(exe string) string

	mu       sync.Mutex
	build    game.Build
	progress Progress
	busy     bool
	cancel   context.CancelFunc
}

// New reads which build is in the folder and clears what an interrupted
// attempt left in the loader folder.
func New(gameDir, loaderDir string) *Work {
	Sweep(loaderDir)
	return &Work{
		gameDir:   gameDir,
		loaderDir: loaderDir,
		url:       URL,
		sum:       Checksum,
		size:      Size,
		version:   game.Version,
		build:     game.Describe(gameDir),
	}
}

// Snapshot is one poll from the page.
func (w *Work) Snapshot() Snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	return Snapshot{
		Build:     w.build,
		Progress:  w.progress,
		Busy:      w.busy,
		Abortable: w.cancel != nil,
	}
}

// Get downloads the wrapper and puts it beside the game.  It returns at once.
// Closing the dialog does not stop it; only Abort does.
func (w *Work) Get() {
	w.mu.Lock()
	if w.busy {
		w.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.busy = true
	w.cancel = cancel
	w.progress = Progress{Stage: StageDownload, Note: "Downloading pureHD", Total: w.size}
	w.mu.Unlock()

	go func() {
		defer cancel()
		err := w.fetch(ctx)
		build := game.Describe(w.gameDir)

		w.mu.Lock()
		defer w.mu.Unlock()
		w.busy = false
		w.cancel = nil
		w.build = build
		switch {
		case err != nil && ctx.Err() != nil:
			w.progress = Progress{Stage: StageAborted, Note: "Download stopped. Nothing in the game folder changed"}
		case err != nil:
			w.progress = Progress{Stage: StageFailed, Error: err.Error()}
		default:
			w.progress = Progress{
				Stage: StageDone,
				Note:  fmt.Sprintf("pureHD %s installed in the game folder", game.Expected),
			}
		}
	}()
}

// Abort stops a download or an extraction.  Once the files are being moved
// into place it does nothing, and the page has already hidden the button.
func (w *Work) Abort() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *Work) fetch(ctx context.Context) error {
	if err := os.MkdirAll(w.loaderDir, 0o755); err != nil {
		return err
	}
	part := filepath.Join(w.loaderDir, partName)
	staging := filepath.Join(w.loaderDir, stagingName)
	defer func() { _ = os.Remove(part) }()
	defer func() { _ = os.RemoveAll(staging) }()

	sum, err := fetch.FileContext(ctx, w.url, nil, part, w.size, func(done, total int64) {
		if total <= 0 {
			total = w.size
		}
		w.stage(StageDownload, "Downloading pureHD", done, total)
	})
	if err != nil {
		return err
	}
	if !strings.EqualFold(sum, w.sum) {
		return fmt.Errorf("The download does not match the checksum this launcher expects "+
			"(expected %s, received %s), so it was deleted", short(w.sum), short(sum))
	}

	w.stage(StageExtract, "Extracting pureHD", 0, 0)
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	if err := java.Unzip(part, staging, func(done, total int64) {
		w.stage(StageExtract, "Extracting pureHD", done, total)
	}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, name := range files() {
		if info, err := os.Stat(filepath.Join(staging, name)); err != nil || info.IsDir() {
			return fmt.Errorf("The archive does not contain %s", name)
		}
	}
	if found := w.version(filepath.Join(staging, game.Names[0])); found != game.Expected {
		return fmt.Errorf("The archive contains %s %s, but the loader needs %s",
			game.Names[0], found, game.Expected)
	}

	// Past this point the folder is being changed, and half a change is worse
	// than either whole one.
	w.mu.Lock()
	if ctx.Err() != nil {
		w.mu.Unlock()
		return ctx.Err()
	}
	w.cancel = nil
	w.progress = Progress{Stage: StageInstall, Note: "Installing pureHD"}
	w.mu.Unlock()

	return place(w.gameDir, staging, filepath.Join(w.loaderDir, backupName))
}

// place moves the two files from staging into the game folder.  Anything of the
// same name already there (an older wrapper, whatever case it is spelled in)
// is moved into a timestamped folder under backups first, and put back if the
// swap fails halfway.
func place(gameDir, staging, backups string) error {
	entries, err := os.ReadDir(gameDir)
	if err != nil {
		return err
	}
	keep := filepath.Join(backups, time.Now().Format("20060102-150405"))

	type moved struct{ from, to string }
	var saved, placed []moved
	undo := func() {
		for _, m := range placed {
			_ = os.Remove(m.to)
		}
		for _, m := range saved {
			_ = os.Rename(m.to, m.from)
		}
	}

	for _, name := range files() {
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(entry.Name(), name) {
				continue
			}
			if err := os.MkdirAll(keep, 0o755); err != nil {
				undo()
				return err
			}
			from := filepath.Join(gameDir, entry.Name())
			to := filepath.Join(keep, entry.Name())
			if err := os.Rename(from, to); err != nil {
				undo()
				return fmt.Errorf("Could not move the existing %s aside. Is the game running? %w",
					entry.Name(), err)
			}
			saved = append(saved, moved{from, to})
		}
	}
	for _, name := range files() {
		to := filepath.Join(gameDir, name)
		if err := os.Rename(filepath.Join(staging, name), to); err != nil {
			undo()
			return fmt.Errorf("Could not write %s into the game folder: %w", name, err)
		}
		placed = append(placed, moved{"", to})
	}
	return nil
}

// Sweep removes what an interrupted attempt left in the loader folder.  The
// backups are the player's own files and stay.
func Sweep(loaderDir string) {
	_ = os.Remove(filepath.Join(loaderDir, partName))
	_ = os.RemoveAll(filepath.Join(loaderDir, stagingName))
}

func (w *Work) stage(stage, note string, done, total int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.progress = Progress{Stage: stage, Note: note, Done: done, Total: total}
}

// files is what the archive has to contain and all that is taken out of it.
// Nothing else in the zip decides a name in the game folder.
func files() []string {
	return []string{game.Names[0], game.Library}
}

const (
	partName    = "purehd.part"
	stagingName = "purehd.new"
	backupName  = "purehd-backup"
)

func short(digest string) string {
	if len(digest) <= 12 {
		return digest
	}
	return digest[:12] + "…"
}
