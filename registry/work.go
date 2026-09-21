package registry

import (
	"strconv"
	"sync"

	"github.com/ancaria-dev/launcher/mods"
)

// Everything slow in here runs in a goroutine and the page polls [Work.View]
// for what to draw, for the same reason the JDK panel does: a WebView2 binding
// runs on the thread that pumps the window, so a call that spends ten seconds
// asking three repositories what they have is ten seconds of a frozen window.

// The stages a player is shown, in the order they happen.
const (
	StageIdle     = ""
	StageIndex    = "index"
	StageDownload = "download"
	StageRemove   = "remove"
	StageDone     = "done"
	StageFailed   = "failed"
)

// Progress is the one job that can be running.
type Progress struct {
	// ID is the mod this is about, so the page can say so on the row that was
	// pressed rather than only on a bar at the foot of the panel. Empty while
	// an index is being read, which is about no single mod.
	ID    string `json:"id"`
	Stage string `json:"stage"`
	Note  string `json:"note"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Error string `json:"error"`
}

// Row is one mod in the folder, with whatever the registries know about it.
type Row struct {
	mods.Mod
	Known
	Enabled bool `json:"enabled"`

	// Conflict names an installed mod this one cannot sit beside. Both are
	// already here, so hiding them is not an option and saying so is.
	Conflict string `json:"conflict"`
}

// Source is a repository as the page lists it.
type Source struct {
	URL       string `json:"url"`
	Name      string `json:"name"`
	Label     string `json:"label"`
	Mods      int    `json:"mods"`
	Token     bool   `json:"token"`
	Error     string `json:"error"`
	Removable bool   `json:"removable"`
}

// View is one poll from the page.
type View struct {
	Sources   []Source `json:"sources"`
	Offers    []Offer  `json:"offers"`
	Installed []Row    `json:"installed"`
	Progress  Progress `json:"progress"`
	Busy      bool     `json:"busy"`
	Loaded    bool     `json:"loaded"`
}

// Hooks are the things Work has to tell somebody else about.  Passed in rather
// than reached for, so this package knows nothing about the settings file.
type Hooks struct {
	// Remember persists the list of remotes, tokens and all.
	Remember func([]Remote)
	// Enable is called for a mod that was just installed. A player who asks for
	// a mod means to run it, and the enabled list is explicit once it exists.
	Enable func(id string)
	// Enabled reports whether a mod in the folder is switched on.
	Enabled func(id string) bool
}

// Work owns the remotes, what they answered, and the one job that may be
// changing the mods folder.  One job at a time: there is one folder to write
// into and one progress bar to watch it with.
type Work struct {
	loader  mods.Loader
	modsDir string
	icons   *Icons
	hooks   Hooks

	mu        sync.Mutex
	remotes   []Remote
	indexes   map[string]*Index
	failures  map[string]string
	installed []mods.Mod
	progress  Progress
	busy      bool
	loaded    bool
}

// New reads the mods folder straight away, because that answer costs nothing
// and belongs on the page before any repository has been asked anything.
func New(loader mods.Loader, loaderDir, modsDir string, remotes []Remote, hooks Hooks) *Work {
	return &Work{
		loader:    loader,
		modsDir:   modsDir,
		icons:     NewIcons(loaderDir),
		hooks:     hooks,
		remotes:   remotes,
		indexes:   map[string]*Index{},
		failures:  map[string]string{},
		installed: loader.Scan(modsDir),
	}
}

// Icon is one picture for the page, empty until there is one.
func (w *Work) Icon(id string) string { return w.icons.Get(id) }

// View is what the page draws.
func (w *Work) View() View {
	w.mu.Lock()
	defer w.mu.Unlock()

	offers, known := Merge(w.loader, w.installed, w.indexes, w.remotes)

	view := View{
		Offers:   offers,
		Progress: w.progress,
		Busy:     w.busy,
		Loaded:   w.loaded,
	}
	for _, remote := range w.remotes {
		source := Source{
			URL:       remote.URL,
			Name:      remote.Name,
			Label:     remote.Label(),
			Token:     remote.Token != "",
			Error:     w.failures[remote.URL],
			Removable: !official(remote.URL),
		}
		if index := w.indexes[remote.URL]; index != nil {
			source.Mods = len(index.Mods)
			if source.Name == "" {
				source.Name = index.Name
			}
		}
		view.Sources = append(view.Sources, source)
	}
	here := map[string]bool{}
	for _, mod := range w.installed {
		here[mod.ID] = true
	}
	for _, mod := range w.installed {
		row := Row{Mod: mod, Known: known[mod.ID]}
		if w.hooks.Enabled != nil {
			row.Enabled = mod.Supported && w.hooks.Enabled(mod.ID)
		}
		for _, other := range mod.Conflicts {
			if here[other] {
				row.Conflict = other
			}
		}
		view.Installed = append(view.Installed, row)
	}
	return view
}

// Load asks every remote what it has.  Called when the page opens, and again
// when a player presses Refresh.
func (w *Work) Load() {
	if !w.start(Progress{Stage: StageIndex, Note: "repositories"}) {
		return
	}
	go func() {
		defer w.finish()
		w.mu.Lock()
		remotes := append([]Remote(nil), w.remotes...)
		w.mu.Unlock()

		changed := false
		for _, remote := range remotes {
			w.note(Progress{Stage: StageIndex, Note: remote.Label()})

			// A remote with no shape worked out yet: the official one on a
			// fresh install, or one saved by a launcher that read it a
			// different way. Resolving is the same work as loading plus the
			// misses before it, and the answer is kept.
			var index *Index
			var dropped int
			var err error
			if remote.Raw == "" {
				var resolved Remote
				resolved, index, err = Resolve(remote.URL, remote.Token)
				if err == nil {
					remote = resolved
					w.adopt(resolved)
					changed = true
				}
			} else {
				index, dropped, err = Load(remote)
			}

			w.mu.Lock()
			if err != nil {
				w.failures[remote.URL] = err.Error()
				delete(w.indexes, remote.URL)
			} else {
				w.indexes[remote.URL] = index
				w.failures[remote.URL] = droppedNote(dropped)
			}
			w.mu.Unlock()

			if index != nil {
				w.pictures(remote, index)
			}
		}
		w.mu.Lock()
		w.loaded = true
		w.progress = Progress{Stage: StageIdle}
		kept := append([]Remote(nil), w.remotes...)
		w.mu.Unlock()
		if changed {
			w.remember(kept)
		}
	}()
}

// adopt replaces a remote with the resolved copy of itself, keeping its place
// in the list.
func (w *Work) adopt(resolved Remote) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for at, existing := range w.remotes {
		if existing.URL == resolved.URL {
			w.remotes[at] = resolved
		}
	}
}

// AddSource resolves a pasted clone URL and keeps it if it answered.
func (w *Work) AddSource(address, token string) {
	if !w.start(Progress{Stage: StageIndex, Note: address}) {
		return
	}
	go func() {
		defer w.finish()
		remote, index, err := Resolve(address, token)
		if err != nil {
			w.fail(err)
			return
		}
		w.mu.Lock()
		replaced := false
		for at, existing := range w.remotes {
			if existing.URL == remote.URL {
				w.remotes[at] = remote
				replaced = true
			}
		}
		if !replaced {
			w.remotes = append(w.remotes, remote)
		}
		w.indexes[remote.URL] = index
		delete(w.failures, remote.URL)
		remotes := append([]Remote(nil), w.remotes...)
		w.progress = Progress{Stage: StageIdle}
		w.mu.Unlock()

		w.remember(remotes)
		w.pictures(remote, index)
	}()
}

// DropSource forgets a remote.  The official one stays: a launcher with no
// repositories at all is a page with nothing on it and no way back.
func (w *Work) DropSource(address string) {
	if official(address) {
		return
	}
	w.mu.Lock()
	kept := w.remotes[:0]
	for _, remote := range w.remotes {
		if remote.URL != address {
			kept = append(kept, remote)
		}
	}
	w.remotes = append([]Remote(nil), kept...)
	delete(w.indexes, address)
	delete(w.failures, address)
	remotes := append([]Remote(nil), w.remotes...)
	w.mu.Unlock()
	w.remember(remotes)
}

// Get installs or updates one mod.
func (w *Work) Get(id string) {
	entry, remote, found := w.find(id)
	if !found {
		return
	}
	if !w.start(Progress{ID: entry.ID, Stage: StageDownload, Note: entry.Name, Total: entry.Size}) {
		return
	}
	go func() {
		defer w.finish()
		err := Install(w.loader, w.modsDir, remote, entry, func(done, total int64) {
			w.note(Progress{ID: entry.ID, Stage: StageDownload, Note: entry.Name,
				Done: done, Total: entry.Size})
		})
		if err != nil {
			w.fail(err)
			return
		}
		if w.hooks.Enable != nil {
			w.hooks.Enable(entry.ID)
		}
		w.rescan()
		w.note(Progress{ID: entry.ID, Stage: StageDone, Note: entry.Name})
	}()
}

// Drop removes an installed mod from the folder.
func (w *Work) Drop(id string) {
	// Its own stage rather than borrowing the download's. Deleting a file is
	// not a transfer, it has no length, and a bar drawn for it would be a bar
	// that is lying about something.
	if !w.start(Progress{ID: id, Stage: StageRemove, Note: id}) {
		return
	}
	go func() {
		defer w.finish()
		if err := Remove(w.loader, w.modsDir, id); err != nil {
			w.fail(err)
			return
		}
		w.rescan()
		w.note(Progress{Stage: StageIdle})
	}()
}

// Refresh throws away every cached picture and asks again.
func (w *Work) Refresh() {
	w.icons.Forget()
	w.Load()
}

func (w *Work) find(id string) (Entry, Remote, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, remote := range w.remotes {
		index := w.indexes[remote.URL]
		if index == nil {
			continue
		}
		for _, entry := range index.Mods {
			if entry.ID == id {
				return entry, remote, true
			}
		}
	}
	return Entry{}, Remote{}, false
}

// pictures fetches the icons an index named, one after another rather than all
// at once: they are decoration, and a repository does not need forty sockets
// opened at it to hand over forty thumbnails.
func (w *Work) pictures(remote Remote, index *Index) {
	for _, entry := range index.Mods {
		if entry.Icon == "" {
			continue
		}
		w.icons.Want(entry.ID, remote.File(entry.Icon), remote.Header())
	}
}

func (w *Work) rescan() {
	found := w.loader.Scan(w.modsDir)
	w.mu.Lock()
	w.installed = found
	w.mu.Unlock()
}

func (w *Work) remember(remotes []Remote) {
	if w.hooks.Remember != nil {
		w.hooks.Remember(remotes)
	}
}

// start claims the one job slot, or reports that somebody else has it.
func (w *Work) start(progress Progress) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.busy {
		return false
	}
	w.busy = true
	w.progress = progress
	return true
}

func (w *Work) finish() {
	w.mu.Lock()
	w.busy = false
	w.mu.Unlock()
}

func (w *Work) note(progress Progress) {
	w.mu.Lock()
	w.progress = progress
	w.mu.Unlock()
}

func (w *Work) fail(err error) {
	w.mu.Lock()
	w.progress = Progress{Stage: StageFailed, Error: err.Error()}
	w.mu.Unlock()
}

func droppedNote(dropped int) string {
	switch {
	case dropped == 0:
		return ""
	case dropped == 1:
		return "One unreadable entry was omitted from this index"
	default:
		return strconv.Itoa(dropped) + " unreadable entries were omitted from this index"
	}
}

func official(address string) bool {
	for _, one := range Official {
		if one == address {
			return true
		}
	}
	return false
}
