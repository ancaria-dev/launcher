package registry

import (
	"path/filepath"
	"testing"
	"time"
)

// Everything the page can do, in the order a player would do it: open the
// launcher with nothing installed, see what is on offer, install one, and find
// it in the other list switched on.
func TestWorkGoesFromAnEmptyFolderToAnInstalledMod(t *testing.T) {
	server := serve(t)
	jar := modJar(t, "a-mod", "1.0.0", "3")
	server.files["/jars/a-mod.jar"] = jar
	server.put(IndexFile, index(server, entry(server, "a-mod", "1.0.0", jar, sum(jar))))

	home := t.TempDir()
	modsDir := filepath.Join(home, "mods")

	var remembered []Remote
	var switchedOn []string
	work := New(here, home, modsDir, []Remote{{URL: server.clone()}}, Hooks{
		Remember: func(remotes []Remote) { remembered = remotes },
		Enable:   func(id string) { switchedOn = append(switchedOn, id) },
		Enabled:  func(id string) bool { return true },
	})

	if view := work.View(); len(view.Installed) != 0 || view.Loaded {
		t.Fatalf("a fresh launcher already had %+v", view)
	}

	work.Load()
	view := settle(t, work)
	if len(view.Offers) != 1 || view.Offers[0].ID != "a-mod" {
		t.Fatalf("offered %+v", view.Offers)
	}
	if view.Offers[0].Source == "" {
		t.Fatal("the row does not say where the mod came from")
	}
	// The remote arrived without a resolved shape, so loading worked one out
	// and the answer was handed back to be saved.
	if len(remembered) != 1 || remembered[0].Raw == "" {
		t.Fatalf("nothing usable was remembered: %+v", remembered)
	}

	work.Get("a-mod")
	view = settle(t, work)
	if len(view.Offers) != 0 {
		t.Fatalf("a mod that is installed is still on offer: %+v", view.Offers)
	}
	if len(view.Installed) != 1 || view.Installed[0].ID != "a-mod" {
		t.Fatalf("installed %+v", view.Installed)
	}
	if !view.Installed[0].Enabled {
		t.Fatal("a mod somebody asked for arrived switched off")
	}
	if view.Installed[0].Update != "" {
		t.Fatalf("an update was offered for the version just installed: %q",
			view.Installed[0].Update)
	}
	if len(switchedOn) != 1 || switchedOn[0] != "a-mod" {
		t.Fatalf("the settings were told %v", switchedOn)
	}

	// And back out again.
	work.Drop("a-mod")
	view = settle(t, work)
	if len(view.Installed) != 0 || len(view.Offers) != 1 {
		t.Fatalf("after removing it: %+v", view)
	}
}

func TestWorkKeepsTheOfficialRemote(t *testing.T) {
	work := New(here, t.TempDir(), t.TempDir(), []Remote{{URL: Official[0]}}, Hooks{})
	work.DropSource(Official[0])
	if view := work.View(); len(view.Sources) != 1 || view.Sources[0].Removable {
		t.Fatalf("sources are %+v", view.Sources)
	}
}

// settle waits for the one job to finish.  Polling rather than a channel
// because polling is exactly what the page does, so this is the same view it
// would be drawing.
func settle(t *testing.T, work *Work) View {
	t.Helper()
	for range 200 {
		view := work.View()
		if !view.Busy {
			return view
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the job never finished")
	return View{}
}
