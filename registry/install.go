package registry

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ancaria-dev/launcher/fetch"
	"github.com/ancaria-dev/launcher/mods"
)

// An index is a few kilobytes of text. A megabyte is a hundred times more room
// than a repository of a hundred mods needs, and a limit that would never be
// reached by an honest one.
const indexLimit = 1 << 20

// Load asks a remote what it has.
func Load(remote Remote) (*Index, int, error) {
	data, err := fetch.Bytes(remote.File(IndexFile), remote.Header(), indexLimit)
	if err != nil {
		return nil, 0, explain(remote, err)
	}
	return Parse(data)
}

// Resolve works out how to read files out of a repository nobody has asked
// before, and proves it by reading the one that matters.
//
// This is the only place the several URL shapes are tried. Once one has
// answered with an index this launcher can read, that shape is what gets saved,
// and every request afterwards goes straight to it.
func Resolve(clone, token string) (Remote, *Index, error) {
	url, err := Normalize(clone)
	if err != nil {
		return Remote{}, nil, err
	}
	candidates := Candidates(url)
	if len(candidates) == 0 {
		return Remote{}, nil, errors.New("This does not look like a repository URL")
	}
	var last error
	for _, template := range candidates {
		remote := Remote{URL: url, Raw: template, Token: token}
		index, _, err := Load(remote)
		if err == nil {
			remote.Name = index.Name
			return remote, index, nil
		}
		// A file that was reached and could not be read is the answer: trying
		// the next shape would only lose the reason.
		var status *fetch.Status
		if !errors.As(err, &status) {
			return Remote{}, nil, err
		}
		last = err
	}
	return Remote{}, nil, fmt.Errorf("This repository has no %s, or it is private "+
		"and requires an access token (%w)", IndexFile, last)
}

// Install puts one mod in the folder the loader reads.
//
// Three things are checked before the jar is allowed to stay, and they check
// different lies. The hash says the bytes are the ones the index published. The
// descriptor inside says the jar is the mod it was offered as, rather than
// something else published under a trusted name. Its version ranges say the
// loader will actually run it, checked against the jar rather than against
// the index entry, because the index is somebody else's file and the descriptor
// is the thing the loader itself will read. A download that fails any of them
// leaves nothing behind.
func Install(loader mods.Loader, modsDir string, remote Remote, entry Entry, progress func(done, total int64)) error {
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		return err
	}
	part := filepath.Join(modsDir, entry.Jar()+".part")
	_ = os.Remove(part)

	sum, err := fetch.File(entry.URL, remote.Header(), part, entry.Size, progress)
	if err != nil {
		return err
	}
	defer os.Remove(part)

	if !strings.EqualFold(sum, entry.Sha256) {
		return fmt.Errorf("%s does not match the checksum published by its repository, "+
			"so it was not installed", entry.Name)
	}
	arrived, ok := loader.Read(part)
	if !ok {
		return fmt.Errorf("%s is not a valid mod JAR", entry.Name)
	}
	if arrived.ID != entry.ID {
		return fmt.Errorf("The downloaded mod identifies itself as “%s”, but it was offered as “%s”",
			arrived.ID, entry.ID)
	}
	if !arrived.Supported {
		return fmt.Errorf("%s cannot run with this loader. %s", entry.Name, arrived.Refusal)
	}

	// An update is the same two steps as an install, in this order: the old jar
	// goes before the new one lands, so the folder never holds two versions of
	// one mod for the loader to pick between.
	if err := Remove(loader, modsDir, entry.ID); err != nil {
		return err
	}
	return os.Rename(part, filepath.Join(modsDir, entry.Jar()))
}

// Remove deletes every jar in the folder claiming an id.  Every, not the one
// with the expected name: a jar renamed by hand is still the mod the loader
// would start.
func Remove(loader mods.Loader, modsDir, id string) error {
	for _, mod := range loader.Scan(modsDir) {
		if mod.ID != id {
			continue
		}
		if err := os.Remove(filepath.Join(modsDir, mod.File)); err != nil {
			return fmt.Errorf("Could not remove %s: %w", mod.File, err)
		}
	}
	return nil
}

// explain turns an HTTP answer into something a player can act on, without
// throwing away what it was.
//
// Both halves matter. The player is told what to do about it, and Resolve is
// still able to see a 404 underneath and go on to the next URL shape, which is
// what a plain rewritten message would have quietly stopped it doing.
func explain(remote Remote, err error) error {
	var status *fetch.Status
	if !errors.As(err, &status) {
		return err
	}
	switch status.Code {
	case http.StatusNotFound:
		if remote.Token == "" {
			return &said{fmt.Sprintf("%s has no %s, or the repository is private "+
				"and requires an access token", remote.Label(), IndexFile), err}
		}
	case http.StatusUnauthorized, http.StatusForbidden:
		if remote.Token == "" {
			return &said{fmt.Sprintf("%s is private, and this launcher has no access token "+
				"for it", remote.Label()), err}
		}
		return &said{fmt.Sprintf("%s rejected the access token", remote.Label()), err}
	}
	return err
}

// said is one sentence for a player with the original underneath it.
type said struct {
	text  string
	cause error
}

func (s *said) Error() string { return s.text }

func (s *said) Unwrap() error { return s.cause }
