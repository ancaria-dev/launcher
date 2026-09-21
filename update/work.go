package update

import (
	"sync"
)

// Same arrangement as java.Work and registry.Work, and for the same reason: a
// WebView2 binding runs on the thread that pumps the window, so anything that
// touches the network has to be started and then polled. A binding that
// downloads is a window frozen with a progress bar painted on it.

// The stages, in the order they happen.
const (
	StageIdle     = ""
	StageCheck    = "check"
	StageDownload = "download"
	StageReady    = "ready"
	StageInstall  = "install"
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

// Snapshot is one poll from the page.
type Snapshot struct {
	Current  string   `json:"current"`
	Release  Release  `json:"release"`
	Found    bool     `json:"found"`
	Progress Progress `json:"progress"`
	Busy     bool     `json:"busy"`
	// Staged is a download that finished and is waiting to be applied, which
	// is what turns the dialog's button into Install & Restart.
	Staged bool `json:"staged"`
}

// Work owns the check and the one download that may follow it.
type Work struct {
	dir     string
	current string

	mu       sync.Mutex
	release  Release
	found    bool
	progress Progress
	busy     bool
	staged   bool
}

// New clears anything an interrupted update left in the game folder and
// returns the half the page polls.
//
// current is this launcher's own version. Nothing is asked of the network
// here: Check does that, and it is started separately so that opening the
// window never waits on github.com being reachable.
func New(loaderDir, current string) *Work {
	Sweep(loaderDir)
	return &Work{dir: loaderDir, current: current}
}

// Snapshot is one poll.
func (w *Work) Snapshot() Snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	return Snapshot{
		Current:  w.current,
		Release:  w.release,
		Found:    w.found,
		Progress: w.progress,
		Busy:     w.busy,
		Staged:   w.staged,
	}
}

// Check asks GitHub what the newest release is and returns at once.
//
// A failure here is silent on purpose. Not being able to reach github.com is
// the ordinary condition of a machine that is offline, and a launcher that
// answers it with a red banner above the mods has turned a non-event into an
// alarm about something the player did not ask for. What they asked for is to
// play Sacred.
func (w *Work) Check() {
	w.mu.Lock()
	if w.busy || w.found {
		w.mu.Unlock()
		return
	}
	w.busy = true
	w.progress = Progress{Stage: StageCheck}
	w.mu.Unlock()

	go func() {
		release, err := Latest()
		w.mu.Lock()
		defer w.mu.Unlock()
		w.busy = false
		w.progress = Progress{}
		if err != nil || !Newer(w.current, release.Version) {
			return
		}
		w.release = release
		w.found = true
	}()
}

// Get downloads the release the check found. It returns immediately and the
// page follows it through Snapshot.
func (w *Work) Get() {
	w.mu.Lock()
	if w.busy || !w.found {
		w.mu.Unlock()
		return
	}
	release := w.release
	w.busy = true
	w.staged = false
	w.progress = Progress{Stage: StageDownload, Total: release.Size}
	w.mu.Unlock()

	go func() {
		_, err := Download(w.dir, release, func(done, total int64) {
			// The release says how big it is, and that number is the one the
			// download is checked against, so it is also the one the bar is
			// drawn from: a server that omits Content-Length would otherwise
			// leave the bar sweeping through a download of a known length.
			if total <= 0 {
				total = release.Size
			}
			w.stage(Progress{Stage: StageDownload, Done: done, Total: total})
		})
		w.mu.Lock()
		defer w.mu.Unlock()
		w.busy = false
		if err != nil {
			w.progress = Progress{Stage: StageFailed, Error: err.Error()}
			return
		}
		w.staged = true
		w.progress = Progress{
			Stage: StageReady,
			Note:  "Sacred Mod Loader " + release.Version + " is ready to install",
			Done:  release.Size,
			Total: release.Size,
		}
	}()
}

// Apply swaps the launcher and starts the new one.
//
// Unlike the other two this one blocks, because it is over in the time two
// renames take and because there is nothing for the page to poll afterwards:
// either the window is about to close or the update did not happen. done is
// called on success, and closing the window is its job.
func (w *Work) Apply(done func()) {
	w.mu.Lock()
	if w.busy || !w.staged {
		w.mu.Unlock()
		return
	}
	w.busy = true
	w.progress = Progress{Stage: StageInstall, Note: "Installing"}
	w.mu.Unlock()

	err := Apply(w.dir)

	w.mu.Lock()
	w.busy = false
	if err != nil {
		// Still staged: the file is where it was, so Install can be pressed
		// again once whatever refused the rename has let go of it.
		w.progress = Progress{Stage: StageFailed, Error: err.Error()}
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()
	done()
}

func (w *Work) stage(progress Progress) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.progress = progress
}
