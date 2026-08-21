// Package conf is what the player chose last time.
//
// It lives in the game folder as plain JSON, so it survives an update of the
// launcher and can be read by a human who wants to know why a mod is not
// loading.
package conf

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ancaria-dev/launcher/registry"
	"github.com/ancaria-dev/launcher/secret"
)

// Conf is small on purpose: everything in it is something a player set.
type Conf struct {
	// Enabled is nil the first time, which means "everything" -- a fresh
	// install should do something rather than nothing.
	Enabled []string `json:"enabled"`
	// Flags are passed to the game executable as-is.
	Flags []string `json:"flags"`
	// Debug keeps the console open with the host's output in it.
	Debug bool `json:"debug"`
	// OffHooks names agent hook sites to leave uninstalled.  Stored the way
	// round it is -- what is OFF, not what is on -- so that a hook added in a
	// later version arrives switched on rather than silently missing.
	OffHooks []string `json:"offHooks"`

	// Remotes are the mod repositories this launcher asks.  Nil is the official
	// list, the same way a nil Enabled is everything: a fresh install has
	// somewhere to get mods from without anybody adding it.
	Remotes []registry.Remote `json:"remotes"`

	path string
}

// Load reads the file, or returns the defaults when there is none.
func Load(dir string) *Conf {
	conf := &Conf{path: filepath.Join(dir, "launcher.json")}
	data, err := os.ReadFile(conf.path)
	if err != nil {
		return conf
	}
	// A corrupt file is not worth refusing to start over: the defaults are
	// harmless and the player's next Play writes a good one.
	_ = json.Unmarshal(data, conf)
	return conf
}

// Save writes the file, ignoring a failure -- losing a preference is not worth
// stopping a player from starting their game.
func (c *Conf) Save() {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(c.path), 0o755)
	_ = os.WriteFile(c.path, data, 0o644)
}

// IsEnabled reports whether a mod should load.  Nil means a fresh install, and
// a fresh install loads everything it shipped with.
func (c *Conf) IsEnabled(id string) bool {
	if c.Enabled == nil {
		return true
	}
	for _, enabled := range c.Enabled {
		if enabled == id {
			return true
		}
	}
	return false
}

// SetEnabled replaces the whole list, which is what the UI hands back.
func (c *Conf) SetEnabled(ids []string) {
	if ids == nil {
		ids = []string{}
	}
	c.Enabled = ids
}

// Enable adds one mod to the enabled list.  For a mod that was just installed:
// asking for it is choosing it, and a nil list already means everything.
func (c *Conf) Enable(id string) {
	if c.Enabled == nil || c.IsEnabled(id) {
		return
	}
	c.Enabled = append(c.Enabled, id)
	c.Save()
}

// Sources are the repositories to ask, with their tokens readable again.
func (c *Conf) Sources() []registry.Remote {
	if len(c.Remotes) == 0 {
		found := make([]registry.Remote, 0, len(registry.Official))
		for _, url := range registry.Official {
			// No Raw: the first Load works out how to read this repository and
			// remembers the answer, exactly as it does for one a player adds.
			found = append(found, registry.Remote{URL: url})
		}
		return found
	}
	found := make([]registry.Remote, 0, len(c.Remotes))
	for _, remote := range c.Remotes {
		remote.Token = secret.Open(remote.Token)
		found = append(found, remote)
	}
	return found
}

// SetSources replaces the list, sealing every token on the way in.  A token
// reaches this file encrypted or it does not reach it at all.
func (c *Conf) SetSources(remotes []registry.Remote) {
	kept := make([]registry.Remote, 0, len(remotes))
	for _, remote := range remotes {
		remote.Token = secret.Seal(remote.Token)
		kept = append(kept, remote)
	}
	c.Remotes = kept
	c.Save()
}

// IsHookOn reports whether a hook site should be installed.
func (c *Conf) IsHookOn(name string) bool {
	for _, off := range c.OffHooks {
		if off == name {
			return false
		}
	}
	return true
}

// SetOffHooks replaces the whole list, which is what the UI hands back.
func (c *Conf) SetOffHooks(names []string) {
	if names == nil {
		names = []string{}
	}
	c.OffHooks = names
}
