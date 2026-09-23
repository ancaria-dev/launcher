// Package mods reads what is in the mods folder without running any of it.
//
// A jar is a zip and its descriptor is one entry inside, so the launcher can
// show a player a list of names and descriptions without a JVM and without
// giving mod code a chance to run before they have ticked its box.
package mods

import (
	"archive/zip"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ancaria-dev/launcher/pin"
)

const descriptor = "META-INF/declaration.toml"

// API is the loader contract this launcher ships, the same number the zygote
// checks and the Gradle plugin writes ranges against.  Raising it is one edit
// here, one in Api.VERSION in coderpack and one in Verifier.API in the build
// plugin.
const API = "3"

// Loader is what a descriptor is measured against: the API contract this
// launcher implements and the release it is.
//
// Passed in rather than reached for, because the release number lives in the
// embedded payload and this package has no business importing that to answer a
// question about a zip file.
type Loader struct {
	API     string
	Version string
}

// Here is this launcher, given its own release number.
func Here(version string) Loader { return Loader{API: API, Version: version} }

// Mod is what a player sees in the list.
type Mod struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	File        string `json:"file"`

	// Conflicts names mods this one cannot sit beside.  It travels in the
	// descriptor rather than only in a registry index, so it still holds for a
	// jar somebody dropped into the folder by hand.
	Conflicts []string `json:"conflicts"`

	// API is the range of loader contracts the mod says it runs on, and Loader
	// the range of launcher releases.  Both are written in the notation the
	// `pin` package documents, and both are the mod's own words: they are shown
	// as written when one of them is the reason it will not load.
	API    string `json:"api"`
	Loader string `json:"loader"`

	// Supported says the loader would start it.  Refusal is why not, in a
	// sentence, and it is empty when Supported is true.  A mod that is not
	// supported stays in the list, switched off: hiding it gets us a bug report
	// about a mod that disappeared, and the player cannot see that a rebuild is
	// what it needs.
	Supported bool   `json:"supported"`
	Refusal   string `json:"refusal"`
}

// Scan reads every jar in a directory.  A jar without a readable descriptor is
// skipped rather than reported: it is not a mod, and a player has no use for
// the distinction.
func (l Loader) Scan(dir string) []Mod {
	paths, err := filepath.Glob(filepath.Join(dir, "*.jar"))
	if err != nil {
		return nil
	}
	found := make([]Mod, 0, len(paths))
	for _, path := range paths {
		if mod, ok := l.Read(path); ok {
			found = append(found, mod)
		}
	}
	sort.Slice(found, func(a, b int) bool { return found[a].Name < found[b].Name })
	return found
}

// Read is one jar.  Exported because installing a mod ends with checking that
// the jar which arrived says what the registry said it would, and that question
// is this same descriptor read the same way.
func (l Loader) Read(path string) (Mod, bool) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return Mod{}, false
	}
	defer archive.Close()

	for _, entry := range archive.File {
		if entry.Name != descriptor {
			continue
		}
		file, err := entry.Open()
		if err != nil {
			return Mod{}, false
		}
		defer file.Close()
		text, err := io.ReadAll(io.LimitReader(file, 64<<10))
		if err != nil {
			return Mod{}, false
		}
		mod := parse(string(text))
		if mod.ID == "" {
			return Mod{}, false
		}
		mod.File = filepath.Base(path)
		if mod.Name == "" {
			mod.Name = mod.ID
		}
		mod.Refusal = l.Refuse(mod.API, mod.Loader)
		mod.Supported = mod.Refusal == ""
		return mod, true
	}
	return Mod{}, false
}

// Refuse answers the only compatibility question there is, for a descriptor's
// two ranges: would this loader start it, and if not, what does a player have
// to be told?  The empty string means yes, and everything else is a whole
// sentence: the page prints it as the line under a mod and nothing wraps it.
//
// A missing `api` is a refusal and not a pass.  The Gradle plugin writes the
// line, so a descriptor without one was written by hand or by a plugin older
// than the field, and neither says anything about which API the code inside
// calls.  Assuming it is current produces the exact failure this check exists
// to stop: a mod that loads, registers its listeners, and dies in the middle of
// a dispatch on a method that is no longer there.
//
// A missing `loader` is a pass, and the difference is not an inconsistency. The
// API contract is what decides whether the code can run at all.  The release
// number is a mod saying it wants a fix from a particular version, which most
// mods have no opinion about and should not have to write down.
func (l Loader) Refuse(api, loader string) string {
	if strings.TrimSpace(api) == "" {
		return "This mod does not declare its loader API, so compatibility cannot " +
			"be checked. Rebuild it with a current toolchain."
	}
	wanted, err := pin.ParseRange(api)
	if err != nil {
		return "This mod’s API version range is invalid: " + err.Error()
	}
	if !wanted.Has(pin.Parse(l.API)) {
		return "This mod needs loader API " + wanted.String() +
			", but this launcher provides API " + l.API + "."
	}

	release, err := pin.ParseRange(loader)
	if err != nil {
		return "This mod’s loader version range is invalid: " + err.Error()
	}
	// Nothing to compare against is not a refusal: a launcher that cannot read
	// its own version number is a bug in the launcher, and refusing every mod
	// over it would be the wrong way round.
	if release.Any() || l.Version == "" {
		return ""
	}
	if !release.Has(pin.Parse(l.Version)) {
		return "This mod needs Sacred Mod Loader " + release.String() +
			", but this version is " + l.Version + "."
	}
	return ""
}

// parse understands `key = "value"` and nothing else, which is all a descriptor
// is — the same subset the Java side reads, deliberately.
func parse(text string) Mod {
	var mod Mod
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch key {
		case "id":
			mod.ID = value
		case "name":
			mod.Name = value
		case "version":
			mod.Version = value
		case "description":
			mod.Description = value
		case "api":
			mod.API = value
		case "loader":
			mod.Loader = value
		case "conflicts":
			mod.Conflicts = list(value)
		}
	}
	return mod
}

// list splits `["a", "b"]` back into its strings.  The plugin writes the
// brackets.  Anything else counts as one name, because a hand-written
// `conflicts = "other-mod"` means something obvious enough to honour.
func list(value string) []string {
	body := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(value), "["), "]")
	var items []string
	for _, part := range strings.Split(body, ",") {
		item := strings.Trim(strings.TrimSpace(part), `"`)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}
