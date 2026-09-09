// Sacred Mod Loader: one file a player drops into their Sacred Gold folder.
//
// Everything else (the host, the agent, the loader jars, the stock mods)
// travels inside this executable and is written into the game folder on first
// run. What a player has to know is "put this next to the game and run it".
//
//	Sacred Mod Loader.exe            the launcher
//	Sacred Mod Loader.exe --debug    the same, with a console showing the host
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/ancaria-dev/launcher/conf"
	"github.com/ancaria-dev/launcher/game"
	"github.com/ancaria-dev/launcher/hooks"
	"github.com/ancaria-dev/launcher/install"
	"github.com/ancaria-dev/launcher/java"
	"github.com/ancaria-dev/launcher/mods"
	"github.com/ancaria-dev/launcher/registry"
	"github.com/ancaria-dev/launcher/secret"
	"github.com/ancaria-dev/launcher/ui"
)

func main() {
	debug := hasFlag("--debug")
	if debug {
		openConsole()
	}

	found, err := install.Find()
	if err != nil {
		fail(err.Error())
		return
	}
	if _, err := found.Unpack(); err != nil {
		fail("Could not write to the game folder: " + err.Error() +
			"\n\nIf Sacred Gold is under Program Files, run this as Administrator.")
		return
	}

	settings := conf.Load(found.LoaderDir())
	settings.Debug = settings.Debug || debug

	// Which build is in this folder, before anything is started. The addresses
	// belong to one of the three executables the lookup accepts, and a player
	// running either of the others is owed that fact while they can still act
	// on it.
	build := game.Describe(found.Dir)

	// Which java the loader will be started with, decided here rather than left
	// to the host: without one the zygote never runs and no mod ever loads, and
	// that is worth saying on the page instead of leaving as a silent nothing.
	jdk := java.New(found.LoaderDir())

	state := ui.State{
		Version: install.Version(),
		Java:    javaRow(jdk.Java()),
		Game: ui.GameRow{
			Exe:         build.Exe,
			Version:     build.Version,
			ExpectedExe: build.ExpectedExe,
			Expected:    build.Expected,
			Matches:     build.Matches,
		},
		Flags:  strings.Join(settings.Flags, " "),
		Debug:  settings.Debug,
		Sealed: secret.Works(),
	}
	for _, group := range hooks.Ask(filepath.Join(found.LoaderDir(), "protocol.exe")) {
		row := ui.HookGroup{Module: group.Module}
		for _, name := range group.Hooks {
			row.Hooks = append(row.Hooks, ui.HookRow{
				Name:    name,
				Enabled: settings.IsHookOn(name),
			})
		}
		state.Hooks = append(state.Hooks, row)
	}
	// Where mods come from, and what is already here. The page polls this the
	// way it polls the Java panel: installing one changes the folder while the
	// window is open, so a list handed over once would be wrong a minute later.
	// What this launcher is, as a mod's descriptor sees it: the API contract it
	// implements and the release it is. Both are ranges on the mod's side, and
	// this pair is what those ranges are asked about.
	loader := mods.Here(install.Version())
	store := registry.New(loader, found.LoaderDir(), found.ModsDir(), settings.Sources(),
		registry.Hooks{
			Remember: settings.SetSources,
			Enable:   settings.Enable,
			// A mod built against another API is never on, whatever the saved
			// list says: the loader would refuse it a second later anyway.
			Enabled: settings.IsEnabled,
		})
	// Asked once, on the way in. A player who opens the launcher with nothing
	// installed should be looking at what they could install, not at a button
	// that fetches the list.
	store.Load()

	remember := func(enabled, offHooks []string, flags string, wantConsole bool) {
		settings.SetEnabled(enabled)
		settings.SetOffHooks(offHooks)
		settings.Flags = strings.Fields(flags)
		settings.Debug = wantConsole
		settings.Save()
	}

	panel := ui.Java{
		View: func() ui.JavaView {
			snapshot := jdk.Snapshot()
			return ui.JavaView{
				Java:     javaRow(snapshot.Java),
				Catalog:  javaCatalog(snapshot.Catalog),
				Progress: ui.JavaProgress(snapshot.Progress),
				Busy:     snapshot.Busy,
				Vendor:   java.DefaultVendor,
				Version:  java.DefaultVersion,
			}
		},
		Load: jdk.Load,
		Get:  jdk.Get,
	}

	shelf := ui.Store{
		View:       store.View,
		Load:       store.Load,
		Refresh:    store.Refresh,
		Icon:       store.Icon,
		Get:        store.Get,
		Drop:       store.Drop,
		AddSource:  store.AddSource,
		DropSource: store.DropSource,
	}

	ui.Run(state, remember, func(enabled, offHooks []string, flags string, wantConsole bool) {
		remember(enabled, offHooks, flags, wantConsole)

		// The console is opened on demand rather than at startup: a player who
		// never ticks the box never sees one, and one who does gets it for this
		// session without restarting the launcher.
		var output io.Writer = io.Discard
		if wantConsole {
			openConsole()
			output = os.Stdout
		}

		// Read at Play rather than at startup: the player may have downloaded
		// one in between, and it is that copy the host has to be given.
		session, err := game.Start(found.LoaderDir(), found.Dir, jdk.Java().Path,
			enabled, offHooks, settings.Flags, output)
		if err != nil {
			fail("Could not start the game: " + err.Error())
			return
		}
		session.Wait()
	}, panel, shelf)
}

// javaRow is the JDK as the page draws it.
func javaRow(found java.Found) ui.JavaRow {
	return ui.JavaRow{
		Path:    found.Path,
		Version: found.Version,
		Source:  found.Source,
		Usable:  found.Usable(),
		Minimum: java.Minimum,
	}
}

func javaCatalog(catalog java.Catalog) ui.JavaCatalog {
	row := ui.JavaCatalog{Loaded: catalog.Loaded, Error: catalog.Error}
	for _, vendor := range catalog.Vendors {
		row.Vendors = append(row.Vendors, ui.JavaVendor(vendor))
	}
	for _, version := range catalog.Versions {
		row.Versions = append(row.Versions, ui.JavaVersion(version))
	}
	return row
}

func hasFlag(name string) bool {
	for _, arg := range os.Args[1:] {
		if arg == name {
			return true
		}
	}
	return false
}

// fail says what went wrong somewhere a player will see it.  The launcher is
// built as a GUI application, so without a console there is nothing to print
// to, and a silent exit reads as "it does not work" rather than as a fixable
// mistake.
func fail(message string) {
	messageBox(message, "Sacred Mod Loader")
	fmt.Fprintln(os.Stderr, message)
}

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	msgBox = user32.NewProc("MessageBoxW")
)

func messageBox(text, title string) {
	body, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	head, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	_, _, _ = msgBox.Call(0, uintptr(unsafe.Pointer(body)),
		uintptr(unsafe.Pointer(head)), 0)
}
