package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Layout is the version of the file format this launcher reads.  A repository
// that says something higher was written for a launcher that knows more than
// this one, and guessing at it is how a player installs something unintended.
const Layout = 1

// Index is `sacred.mods.repository.json` at the root of a repository.
type Index struct {
	SRML        int     `json:"srml"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	URL         string  `json:"url"`
	Icon        string  `json:"icon"`
	Mods        []Entry `json:"mods"`
}

// Entry is one mod on offer.  Everything down to Website is copied out of the
// descriptor inside the jar by whoever generated the index, so it says the same
// thing the jar will say once it is downloaded, and Install checks that it
// does.
type Entry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Authors     []string `json:"authors"`
	Website     string   `json:"website"`
	Conflicts   []string `json:"conflicts"`

	// API is the range of loader contracts the mod runs on and Loader the range
	// of launcher releases, both copied out of the descriptor. An index written
	// before ranges existed says `api = "1"`, which the notation reads as that
	// one version, so an old index keeps meaning what it meant.
	API    string `json:"api"`
	Loader string `json:"loader"`

	// Source is where the code sits inside the repository, and Icon a file in
	// it. Both are relative paths and both are optional.
	Source string `json:"source"`
	Icon   string `json:"icon"`

	// File is the jar's name as published, kept for the record. It is never
	// used as a path: the name a jar is saved under is built out of the id and
	// the version, both of which are checked, because a file name that arrived
	// over the network has no business deciding where anything is written.
	File   string `json:"file"`
	Size   int64  `json:"size"`
	Sha256 string `json:"sha256"`
	URL    string `json:"url"`
}

// Parse reads an index and drops the entries it cannot trust.
//
// A repository is somebody else's file. One malformed entry is theirs to fix
// and no reason to hide the rest, so a bad entry is left out and counted. A
// malformed file, or one from a layout this launcher does not know, is an error
// with nothing usable in it.
func Parse(data []byte) (*Index, int, error) {
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, 0, fmt.Errorf("This is not a readable mod repository index: %w", err)
	}
	if index.SRML == 0 {
		return nil, 0, errors.New("This file is not a mod repository index")
	}
	if index.SRML > Layout {
		return nil, 0, fmt.Errorf("This repository requires a newer launcher "+
			"(layout %d, this launcher supports %d)", index.SRML, Layout)
	}
	if strings.TrimSpace(index.Name) == "" {
		return nil, 0, errors.New("The repository index has no name")
	}
	if !safePath(index.Icon) {
		index.Icon = ""
	}

	kept := make([]Entry, 0, len(index.Mods))
	seen := map[string]bool{}
	dropped := 0
	for _, mod := range index.Mods {
		if !mod.sound() || seen[mod.ID] {
			dropped++
			continue
		}
		seen[mod.ID] = true
		if !safePath(mod.Icon) {
			mod.Icon = ""
		}
		if !safePath(mod.Source) {
			mod.Source = ""
		}
		kept = append(kept, mod)
	}
	index.Mods = kept
	return &index, dropped, nil
}

// Jar is the name this entry is saved under.  Built here rather than taken from
// the index, out of two fields that have been checked character by character.
func (e Entry) Jar() string {
	return e.ID + "-" + e.Version + ".jar"
}

// sound reports whether an entry is worth showing at all.  Everything checked
// here is something that decides where bytes are written or whether they are
// the right bytes.
func (e Entry) sound() bool {
	return validID(e.ID) &&
		validVersion(e.Version) &&
		strings.HasPrefix(e.URL, "https://") &&
		hex64(e.Sha256) &&
		e.Size > 0
}

// validID is the launcher's own rule for a mod id, the one the Gradle plugin
// enforces and the one the enabled list is written with.
func validID(id string) bool {
	if id == "" || len(id) > 64 || strings.HasPrefix(id, "-") || strings.HasSuffix(id, "-") {
		return false
	}
	for _, character := range id {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= '0' && character <= '9':
		case character == '-':
		default:
			return false
		}
	}
	return true
}

// validVersion is deliberately narrow: this string becomes part of a file name.
func validVersion(version string) bool {
	if version == "" || len(version) > 32 {
		return false
	}
	for _, character := range version {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		case character == '.' || character == '-' || character == '_' || character == '+':
		default:
			return false
		}
	}
	return true
}

func hex64(digest string) bool {
	if len(digest) != 64 {
		return false
	}
	for _, character := range digest {
		switch {
		case character >= '0' && character <= '9':
		case character >= 'a' && character <= 'f':
		case character >= 'A' && character <= 'F':
		default:
			return false
		}
	}
	return true
}

// safePath accepts a relative path inside the repository and nothing else.  It
// is appended to a URL and hashed into a cache file name, and `..` in either
// place is somebody else's idea of where this launcher should be reading.
func safePath(path string) bool {
	if path == "" {
		return true
	}
	if strings.HasPrefix(path, "/") || strings.Contains(path, "\\") ||
		strings.Contains(path, "://") || strings.Contains(path, "..") {
		return false
	}
	return len(path) <= 200
}
