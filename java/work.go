package java

import (
	"fmt"
	"os"
	"strconv"
	"sync"
)

// The page cannot wait for any of this.  A WebView2 binding runs on the thread
// that pumps the window's messages, so a call that spends thirty seconds
// downloading is thirty seconds of a frozen window -- which is exactly what a
// progress bar exists to prevent.  Everything slow therefore runs in a
// goroutine and the page polls Snapshot for what it should draw.

// The stages a player is shown, in the order they happen.
const (
	StageIdle     = ""
	StageCatalog  = "catalog"
	StageResolve  = "resolve"
	StageDownload = "download"
	StageExtract  = "extract"
	StageDone     = "done"
	StageFailed   = "failed"
)

// Progress is the state of the one job that can be running.
type Progress struct {
	Stage string `json:"stage"`
	Note  string `json:"note"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Error string `json:"error"`
}

// Catalog is what the two dropdowns are filled from.  Loaded is false until the
// index has answered, so the page can say "asking" rather than "none".
type Catalog struct {
	Vendors  []Vendor  `json:"vendors"`
	Versions []Version `json:"versions"`
	Loaded   bool      `json:"loaded"`
	Error    string    `json:"error"`
}

// Snapshot is everything the page asks for in one poll.
type Snapshot struct {
	Java     Found    `json:"-"`
	Catalog  Catalog  `json:"catalog"`
	Progress Progress `json:"progress"`
	Busy     bool     `json:"busy"`
}

// Work owns the JDK the loader will use and the one job that may be changing
// it.  One job at a time, because there is one folder to write into.
type Work struct {
	dir string

	mu       sync.Mutex
	java     Found
	catalog  Catalog
	progress Progress
	busy     bool
}

// New looks for a JDK straight away -- reading a `release` file costs nothing
// and the answer belongs on the page before the player asks a question about
// it -- and clears anything an interrupted download left behind.
func New(loaderDir string) *Work {
	Sweep(loaderDir)
	return &Work{dir: loaderDir, java: Find(loaderDir)}
}

// Java is the JDK the host should be started with, if any.
func (w *Work) Java() Found {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.java
}

// Snapshot is one poll from the page.
func (w *Work) Snapshot() Snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	return Snapshot{Java: w.java, Catalog: w.catalog, Progress: w.progress, Busy: w.busy}
}

// Load fills the dropdowns.  Called when the player opens the panel, and once
// only: the list of distributions does not change while a launcher is open.
func (w *Work) Load() {
	w.mu.Lock()
	if w.catalog.Loaded || w.busy {
		w.mu.Unlock()
		return
	}
	w.busy = true
	w.progress = Progress{Stage: StageCatalog, Note: "Loading available JDKs from foojay"}
	w.mu.Unlock()

	go func() {
		vendors, err := Vendors()
		var versions []Version
		if err == nil {
			versions, err = Versions()
		}
		w.mu.Lock()
		defer w.mu.Unlock()
		w.busy = false
		w.progress = Progress{}
		if err != nil {
			w.catalog = Catalog{Error: err.Error()}
			return
		}
		w.catalog = Catalog{Vendors: vendors, Versions: versions, Loaded: true}
	}()
}

// Get downloads one JDK and makes it the one the loader uses.  It returns
// immediately; the page follows along through Snapshot.
func (w *Work) Get(vendor string, major int) {
	w.mu.Lock()
	if w.busy {
		w.mu.Unlock()
		return
	}
	w.busy = true
	w.progress = Progress{Stage: StageResolve, Note: "Finding " + vendor + " " + strconv.Itoa(major)}
	w.mu.Unlock()

	go func() {
		found, err := w.fetch(vendor, major)
		w.mu.Lock()
		defer w.mu.Unlock()
		w.busy = false
		if err != nil {
			w.progress = Progress{Stage: StageFailed, Error: err.Error()}
			return
		}
		w.java = found
		w.progress = Progress{
			Stage: StageDone,
			Note:  fmt.Sprintf("Java %s installed in the game folder", found.Version),
		}
	}()
}

func (w *Work) fetch(vendor string, major int) (Found, error) {
	pkg, err := Resolve(vendor, major)
	if err != nil {
		return Found{}, err
	}
	link, err := Locate(pkg.ID)
	if err != nil {
		return Found{}, err
	}

	w.stage(StageDownload, pkg.Filename, 0, pkg.Size)
	archive, err := Download(w.dir, pkg, link, func(done, total int64) {
		w.stage(StageDownload, pkg.Filename, done, total)
	})
	if err != nil {
		return Found{}, err
	}
	defer func() { _ = os.Remove(archive) }()

	w.stage(StageExtract, "Extracting "+pkg.Filename, 0, 0)
	found, err := Install(w.dir, archive, func(done, total int64) {
		w.stage(StageExtract, "Extracting "+pkg.Filename, done, total)
	})
	if err != nil {
		return Found{}, err
	}
	if !found.Usable() {
		return Found{}, fmt.Errorf("The extracted JDK contains Java %s, but the loader needs %d or newer",
			found.Version, Minimum)
	}
	return found, nil
}

func (w *Work) stage(stage, note string, done, total int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.progress = Progress{Stage: stage, Note: note, Done: done, Total: total}
}
