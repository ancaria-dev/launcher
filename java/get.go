package java

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ancaria-dev/launcher/fetch"
)

// Download pulls a package into `<loaderDir>/java.part`, hashing it on the way,
// and returns the file it wrote.
//
// Nothing partial survives a failure.  A JDK is a couple of hundred megabytes,
// a broken half of one is indistinguishable from a whole one by name alone, and
// the next run would extract it and produce a java.exe that crashes on the
// first class it reads.
func Download(loaderDir string, pkg Pkg, link Link, progress func(done, total int64)) (string, error) {
	part := filepath.Join(loaderDir, "java.part")
	if err := os.MkdirAll(loaderDir, 0o755); err != nil {
		return "", err
	}
	_ = os.Remove(part)

	// No overall timeout: this transfer is minutes long by design, and the
	// short one on the index client would abort it halfway every time.
	client := &http.Client{Timeout: 0}
	response, err := client.Get(link.URL)
	if err != nil {
		return "", fmt.Errorf("Could not download %s: %w", pkg.Filename, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s returned %s", link.URL, response.Status)
	}

	total := response.ContentLength
	if total <= 0 {
		// Some mirrors send no length.  The API already told us the size.
		total = pkg.Size
	}

	file, err := os.Create(part)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	err = fetch.Copy(io.MultiWriter(file, digest), response.Body, total, progress)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(part)
		return "", fmt.Errorf("The download of %s stopped early: %w", pkg.Filename, err)
	}

	if err := check(link, digest.Sum(nil)); err != nil {
		_ = os.Remove(part)
		return "", err
	}
	return part, nil
}

// check compares what arrived against what the index published.  A digest the
// index does not have is not a reason to refuse the file (there is nothing to
// compare it with) but a digest that disagrees is: the bytes are not the ones
// the vendor signed off, and unpacking them is how a mod loader turns into a
// way of running somebody else's code.
func check(link Link, sum []byte) error {
	if link.Checksum == "" {
		return nil
	}
	if link.Kind != "" && link.Kind != "sha256" {
		return nil
	}
	got := hex.EncodeToString(sum)
	if !strings.EqualFold(got, link.Checksum) {
		return fmt.Errorf("The download does not match its published checksum "+
			"(expected %s, received %s), so it was deleted", short(link.Checksum), short(got))
	}
	return nil
}

func short(digest string) string {
	if len(digest) <= 12 {
		return digest
	}
	return digest[:12] + "…"
}

// Install replaces `<loaderDir>/java` with the contents of an archive.
//
// It unpacks beside the old copy and swaps at the end, so a failure halfway
// leaves whatever was working before still working.
func Install(loaderDir, archive string, progress func(done, total int64)) (Found, error) {
	final := Dir(loaderDir)
	staging := final + ".new"
	if err := os.RemoveAll(staging); err != nil {
		return Found{}, err
	}
	if err := Unzip(archive, staging, progress); err != nil {
		_ = os.RemoveAll(staging)
		return Found{}, err
	}
	if !isJDK(staging) {
		_ = os.RemoveAll(staging)
		return Found{}, errors.New("The archive contains no bin/java.exe, so it is not a JDK")
	}

	// The old copy cannot simply be deleted while something is reading it, so
	// it is moved aside first and removed on the way past.
	if _, err := os.Stat(final); err == nil {
		stale := fmt.Sprintf("%s.old-%d", final, time.Now().UnixNano())
		if err := os.Rename(final, stale); err != nil {
			_ = os.RemoveAll(staging)
			return Found{}, fmt.Errorf("Could not replace the JDK in %s: %w", final, err)
		}
		defer os.RemoveAll(stale)
	}
	if err := os.Rename(staging, final); err != nil {
		return Found{}, err
	}

	found := Found{Path: exeIn(final), Home: final, Source: FromLoader}
	found.Version = version(final, found.Path)
	found.Major = Major(found.Version)
	return found, nil
}

// Sweep removes what an interrupted attempt left in the loader folder.  Called
// on the way in, because the launcher may have been closed mid-download and a
// stale 90 MB `java.part` is not something a player should have to find.
func Sweep(loaderDir string) {
	_ = os.Remove(filepath.Join(loaderDir, "java.part"))
	_ = os.RemoveAll(Dir(loaderDir) + ".new")
	matches, _ := filepath.Glob(Dir(loaderDir) + ".old-*")
	for _, stale := range matches {
		_ = os.RemoveAll(stale)
	}
}
