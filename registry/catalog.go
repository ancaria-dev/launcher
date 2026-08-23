package registry

import (
	"sort"

	"github.com/ancaria-dev/launcher/mods"
)

// Offer is a mod that could be installed: an entry, plus where it came from.
type Offer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Website     string `json:"website"`
	Size        int64  `json:"size"`

	// Source is the line under the name, and Remote is the URL that line is
	// short for -- the one Install is told to fetch from.
	Source string `json:"source"`
	Remote string `json:"remote"`

	// Supported is false for a mod this loader would refuse, and Refusal is the
	// sentence saying which of its two ranges this launcher falls outside of. It
	// stays in the list and the button is not offered: hiding it earns a bug
	// report about a mod missing from a page that says it has everything.
	Supported bool   `json:"supported"`
	Refusal   string `json:"refusal"`
}

// Known is what a registry has to say about a mod already in the folder.
type Known struct {
	// Source is the remote offering it, empty when none does -- a mod somebody
	// built themselves, or a registry that is not answering today.
	Source string `json:"source"`
	Remote string `json:"remote"`

	// Update is the version on offer, set only when it differs from the one
	// installed. Differs rather than exceeds: an author who pulls a release
	// back means it, and comparing version strings for order is a guess about
	// somebody else's numbering.
	Update string `json:"update"`
}

// Merge turns what every remote answered into the two lists a page draws.
//
// The rule about conflicts is the blunt one on purpose. Two mods that rewrite
// the same thing produce whichever answer the loader asked for last, and no
// player can be expected to know that. So a conflict does not become a warning
// or an ordering: both sides leave the list, and the pair stops existing as a
// choice. Somebody who wants one of them anyway can still install it by hand,
// which is the right amount of effort for taking that on knowingly.
func Merge(loader mods.Loader, installed []mods.Mod, indexes map[string]*Index, remotes []Remote) ([]Offer, map[string]Known) {
	label := map[string]string{}
	for _, remote := range remotes {
		label[remote.URL] = remote.Label()
	}

	here := map[string]mods.Mod{}
	refused := map[string]bool{}
	for _, mod := range installed {
		here[mod.ID] = mod
		// What an installed mod refuses to sit beside is as binding as what a
		// candidate does. It is here and they are not.
		for _, other := range mod.Conflicts {
			refused[other] = true
		}
	}

	// Every candidate, before anything is hidden, so a conflict can be seen
	// from both ends.
	type candidate struct {
		entry  Entry
		remote string
	}
	candidates := []candidate{}
	offered := map[string]int{}
	for _, remote := range remotes {
		index := indexes[remote.URL]
		if index == nil {
			continue
		}
		for _, entry := range index.Mods {
			candidates = append(candidates, candidate{entry, remote.URL})
			offered[entry.ID]++
		}
	}

	// The same id from two repositories is a conflict of its own: there is no
	// way to say which one the button means.
	duplicated := map[string]bool{}
	for id, count := range offered {
		if count > 1 {
			duplicated[id] = true
		}
	}

	// Both sides of every conflict between two candidates.
	fought := map[string]bool{}
	for _, one := range candidates {
		for _, other := range one.entry.Conflicts {
			if offered[other] > 0 {
				fought[one.entry.ID] = true
				fought[other] = true
			}
		}
	}

	offers := []Offer{}
	for _, found := range candidates {
		entry := found.entry
		// Asked of the index rather than of the jar, which has not been
		// downloaded yet. Install asks the jar the same question again before
		// anything is kept, because an index is somebody else's file.
		refusal := loader.Refuse(entry.API, entry.Loader)
		if _, installed := here[entry.ID]; installed {
			continue
		}
		if duplicated[entry.ID] || fought[entry.ID] || refused[entry.ID] {
			continue
		}
		if conflictsWithInstalled(entry, here) {
			continue
		}
		offers = append(offers, Offer{
			ID:          entry.ID,
			Name:        entry.Name,
			Version:     entry.Version,
			Description: entry.Description,
			Website:     entry.Website,
			Size:        entry.Size,
			Source:      label[found.remote],
			Remote:      found.remote,
			Supported:   refusal == "",
			Refusal:     refusal,
		})
	}
	sort.Slice(offers, func(a, b int) bool { return offers[a].Name < offers[b].Name })

	known := map[string]Known{}
	for _, found := range candidates {
		mod, installed := here[found.entry.ID]
		if !installed {
			continue
		}
		// Two repositories offering the same installed mod is the same
		// ambiguity as above: the source is shown, the update is not offered.
		if duplicated[found.entry.ID] {
			known[mod.ID] = Known{Source: label[found.remote], Remote: found.remote}
			continue
		}
		update := ""
		if found.entry.Version != mod.Version {
			update = found.entry.Version
		}
		known[mod.ID] = Known{
			Source: label[found.remote],
			Remote: found.remote,
			Update: update,
		}
	}
	return offers, known
}

func conflictsWithInstalled(entry Entry, here map[string]mods.Mod) bool {
	for _, other := range entry.Conflicts {
		if _, found := here[other]; found {
			return true
		}
	}
	return false
}
