package registry

import (
	"sort"
	"strings"

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
	// short for: the one Install is told to fetch from.
	Source string `json:"source"`
	Remote string `json:"remote"`

	// Supported is false for a mod this loader would refuse, and Refusal is the
	// sentence saying which of its two ranges this launcher falls outside of. It
	// stays in the list and the button is not offered: hiding it earns a bug
	// report about a mod missing from a page that says it has everything.
	Supported bool   `json:"supported"`
	Refusal   string `json:"refusal"`

	// Caution is the sentence for a mod declared to clash with another one the
	// player can see. It is not a refusal and it stops nothing: installing
	// both is allowed and drawn in the amber a caution gets, because this is a
	// thing somebody has to decide rather than a thing that cannot work.
	Caution string `json:"caution"`
}

// Known is what a registry has to say about a mod already in the folder.
type Known struct {
	// Source is the remote offering it, empty when none does: a mod somebody
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
	for _, mod := range installed {
		here[mod.ID] = mod
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

	// Who has said they clash with whom, across everything on this page.  A
	// declared conflict is a caution and not a gate: the pair is drawn with a
	// sentence and both stay installable.
	clash := &Clash{}
	for _, mod := range installed {
		clash.Add(mod.ID, mod.Name, mod.Conflicts)
	}
	for _, one := range candidates {
		clash.Add(one.entry.ID, one.entry.Name, one.entry.Conflicts)
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
		// The one thing still hidden, and it is not a conflict between two
		// mods. One id from two repositories is an ambiguity about which file
		// Install means, and there is no sentence that resolves it.
		if duplicated[entry.ID] {
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
			Caution:     clash.Sentence(entry.ID),
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

// Clash is who has declared a conflict with whom, in both directions.
//
// A mod names what it will not sit beside, and the other half of that pair
// says nothing. Both are equally affected by the combination, so both are told
// about it: a player looking at the one that stayed quiet would otherwise have
// no way of knowing it was half of a pair.
//
// Only pairs where both sides are present produce a sentence. A mod naming
// something nobody offers and nobody has installed is naming nothing the
// player can act on.
type Clash struct {
	names map[string]string
	with  map[string]map[string]bool
}

// Add records one mod: what it is called, and what it says it clashes with.
func (c *Clash) Add(id, name string, conflicts []string) {
	if c.names == nil {
		c.names = map[string]string{}
		c.with = map[string]map[string]bool{}
	}
	if name == "" {
		name = id
	}
	c.names[id] = name
	for _, other := range conflicts {
		c.pair(id, other)
		c.pair(other, id)
	}
}

func (c *Clash) pair(from, to string) {
	if c.with[from] == nil {
		c.with[from] = map[string]bool{}
	}
	c.with[from][to] = true
}

// Sentence is what goes under one mod's description, empty when it clashes
// with nothing the player can see.
//
// Written here rather than on the page for the same reason Refusal is: the
// names are Go's, and a sentence assembled in JavaScript out of a list is a
// second place to keep the wording right.
func (c *Clash) Sentence(id string) string {
	var named []string
	for other := range c.with[id] {
		if name, known := c.names[other]; known {
			named = append(named, name)
		}
	}
	if len(named) == 0 {
		return ""
	}
	sort.Strings(named)

	list, allowed := named[0], "Both can be installed"
	if len(named) > 1 {
		list = strings.Join(named[:len(named)-1], ", ") + " and " + named[len(named)-1]
		allowed = "They can all be installed"
	}
	return "May not work correctly alongside " + list + ". " + allowed +
		"; turn one off if the game misbehaves."
}
