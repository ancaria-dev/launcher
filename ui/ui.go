// Package ui is the window: a WebView2 view over an embedded page.
//
// HTML because the launcher is the one part a player looks at, and CSS is the
// cheapest way to make it look like something rather than a dialog from 1998.
// go-webview2 is pure Go, so this still builds with `go build` and ships as one
// file.  The WebView2 runtime it drives is part of Windows 10 and 11.
package ui

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"strings"
	"syscall"
	"unsafe"

	"github.com/ancaria-dev/launcher/registry"
	"github.com/jchv/go-webview2"
)

//go:embed web
var web embed.FS

// iconResource is the RT_GROUP_ICON id `tools/rsrc` writes into
// rsrc_windows_amd64.syso.  Two places name this number and there is no shared
// source for it: the generator that puts the icon in, and the window that asks
// for it back.
const iconResource = 1

// State is everything the page needs and everything it can change.
type State struct {
	Version string      `json:"version"`
	Game    GameRow     `json:"game"`
	Java    JavaRow     `json:"java"`
	Hooks   []HookGroup `json:"hooks"`
	Flags   string      `json:"flags"`
	Debug   bool        `json:"debug"`

	// Sealed says whether a token for a private mod repository can be encrypted
	// on this machine.  The page prints one sentence or the other under the
	// field, because "stored safely" is a promise and it has to be true.
	Sealed bool `json:"sealed"`
}

// GameRow is which game is in the folder and which one the loader was made
// for.  Matches false draws a caution above everything else, because pressing
// Play is the decision it is about and nothing after that point can be undone
// by reading a log.  Version is empty when the executable carries none, which
// counts as a mismatch: it is still not a build anybody has confirmed.
type GameRow struct {
	Exe         string `json:"exe"`
	Version     string `json:"version"`
	ExpectedExe string `json:"expectedExe"`
	Expected    string `json:"expected"`
	Matches     bool   `json:"matches"`
}

// HookGroup is one agent module in the hooks block.
type HookGroup struct {
	Module string    `json:"module"`
	Hooks  []HookRow `json:"hooks"`
}

// HookRow is one place in the game the agent attaches to.  Off means the host
// is told to leave that instruction alone, which is how a crash gets bisected
// down to a single site without editing JavaScript between game restarts.
type HookRow struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// Store is the mods half of the page: what is installed, what is on offer, and
// everything that changes either.
//
// Separate from State for the same reason the Java panel is: this is the part
// that moves while the window is open, and the page polls View for it rather
// than being handed a list that was true when it opened.
type Store struct {
	View    func() registry.View
	Load    func()
	Refresh func()
	Icon    func(id string) string

	// Get installs or updates one mod, Drop removes an installed one.
	Get  func(id string)
	Drop func(id string)

	// AddSource takes a clone URL and a token for a repository that needs one.
	AddSource  func(url, token string)
	DropSource func(url string)
}

// Choice is everything the page can change: what was ticked, what was typed,
// and whether a console is wanted.  offHooks is named rather than counted
// because that is what the host's --no-hook takes.
//
// The same shape serves both callbacks: pressing Play is remembering plus
// starting a game, and writing that as two types would only invite them to
// drift apart.
type Choice func(enabled, offHooks []string, flags string, debug bool)

// Run opens the window and blocks until it is closed.
//
// remember is called on every change rather than only on Play: a player who
// ticks a mod, changes their mind about the console and then closes the window
// has still made a choice, and losing it is the kind of small betrayal that
// makes a launcher feel unreliable.
//
// jdk is the Java panel.  It is separate from State because it is the one thing
// on this page that changes while the page is open.
//
// up is the launcher upgrading itself.  Its Apply never comes back to a
// window: Terminate is handed to it, so the last thing the old launcher does
// is close after starting the new one.
func Run(state State, remember, play Choice, jdk Java, store Store, up Update) {
	// Before any window exists: afterwards Windows has already decided how to
	// treat this process and scales its output as a bitmap.
	awareOfDPI()
	view := webview2.NewWithOptions(webview2.WebViewOptions{
		WindowOptions: webview2.WindowOptions{
			Title: "Sacred Mod Loader",
			// The icon the window and the taskbar draw, out of the executable's
			// own resources. `tools/rsrc` writes that resource and gives the
			// group this id.  With no .syso linked in, LoadImage finds nothing
			// and the window falls back to the system default rather than
			// failing to open.
			IconId: iconResource,
			Width:  scale(900),
			Height: scale(620),
			Center: true,
		},
	})
	if view == nil {
		return
	}
	defer view.Destroy()

	window := view.Window()

	view.Bind("smlState", func() State { return state })

	view.Bind("smlSave", remember)

	view.Bind("smlJava", jdk.View)
	view.Bind("smlJavaLoad", jdk.Load)
	view.Bind("smlJavaGet", jdk.Get)

	view.Bind("smlUpdate", up.View)
	view.Bind("smlUpdateGet", up.Get)
	view.Bind("smlOpen", up.Open)

	// Off the message thread, because Apply renames two files and starts a
	// process.  What it is handed is how this launcher stops being the one in
	// the game folder: the swap has happened, the replacement is running, and
	// the only thing left for this window to do is go away.
	view.Bind("smlUpdateApply", func() {
		go up.Apply(func() { view.Dispatch(view.Terminate) })
	})

	view.Bind("smlMods", store.View)
	view.Bind("smlModsLoad", store.Load)
	view.Bind("smlModsRefresh", store.Refresh)
	view.Bind("smlModsGet", store.Get)
	view.Bind("smlModsDrop", store.Drop)
	view.Bind("smlIcon", store.Icon)
	view.Bind("smlSourceAdd", store.AddSource)
	view.Bind("smlSourceDrop", store.DropSource)

	view.Bind("smlPlay", func(enabled, offHooks []string, flags string, debug bool) {
		show(window, false)
		go func() {
			play(enabled, offHooks, flags, debug)
			// Back on the UI thread: the player closed the game and expects
			// the launcher where they left it.
			view.Dispatch(func() { show(window, true) })
		}()
	})

	view.SetHtml(page())
	view.Run()
}

// page inlines the stylesheet, the script and the mark so the view needs no
// server and the executable needs no files beside it.
func page() string {
	html := read("web/index.html")
	style := strings.Replace(read("web/app.css"), "/*icon*/", mark(), 1)
	html = strings.Replace(html, "/*style*/", style, 1)
	html = strings.Replace(html, "/*script*/", read("web/app.js"), 1)
	return html
}

// mark is the launcher's icon as base64, for the one place the stylesheet names
// it.  It has to travel inside the page for the same reason a mod's icon does:
// the window is loaded from a string, so it has no origin and every relative
// path in it resolves to nothing.
func mark() string {
	data, err := web.ReadFile("web/icon.png")
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}

func read(name string) string {
	data, err := web.ReadFile(name)
	if err != nil {
		return ""
	}
	return string(data)
}

var (
	user32     = syscall.NewLazyDLL("user32.dll")
	showWindow = user32.NewProc("ShowWindow")
)

const (
	swHide = 0
	swShow = 5
)

// The handle comes back from the view as an unsafe.Pointer, which is what a
// HWND is on the other side of that API.  ShowWindow wants it as a uintptr.
func show(window unsafe.Pointer, visible bool) {
	command := uintptr(swHide)
	if visible {
		command = swShow
	}
	_, _, _ = showWindow.Call(uintptr(window), command)
}

// Marshal is here so main can log what the page will be handed without
// duplicating the tags.
func Marshal(state State) string {
	data, err := json.Marshal(state)
	if err != nil {
		return "{}"
	}
	return string(data)
}
