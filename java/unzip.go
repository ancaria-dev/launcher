package java

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Unzip writes a JDK archive into dir, dropping the archive's own top-level
// directory so the result is always `<dir>/bin/java.exe` whatever the vendor
// named their folder.
//
// progress is called with bytes written and the total the archive says it
// holds; it may be nil.
func Unzip(archive, dir string, progress func(done, total int64)) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("The download is not a readable ZIP archive: %w", err)
	}
	defer reader.Close()

	strip := commonRoot(reader.File)
	var total int64
	for _, entry := range reader.File {
		total += int64(entry.UncompressedSize64)
	}

	var done int64
	for _, entry := range reader.File {
		name, ok := target(entry.Name, strip)
		if !ok {
			continue
		}
		// The guard.  A zip is a list of names somebody else wrote, and
		// `../../Windows/System32/…` is a perfectly legal name to put in one.
		// filepath.IsLocal is the whole check: it rejects absolute paths, drive
		// letters, anything that climbs out with .., and the Windows device
		// names besides.
		if !filepath.IsLocal(name) {
			return fmt.Errorf("The archive contains an entry that would write outside "+
				"the loader folder: %s", entry.Name)
		}
		where := filepath.Join(dir, name)

		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(where, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(where), 0o755); err != nil {
			return err
		}
		written, err := extract(entry, where)
		if err != nil {
			return err
		}
		done += written
		if progress != nil {
			progress(done, total)
		}
	}
	return nil
}

func extract(entry *zip.File, where string) (int64, error) {
	source, err := entry.Open()
	if err != nil {
		return 0, err
	}
	defer source.Close()

	file, err := os.OpenFile(where, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, entry.Mode().Perm()|0o200)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	// The size the header claims, plus a megabyte of slack, is the ceiling: an
	// entry that keeps producing bytes past its own declared length is a zip
	// bomb, not a JDK.
	limit := int64(entry.UncompressedSize64) + 1<<20
	written, err := io.Copy(file, io.LimitReader(source, limit))
	if err != nil {
		return written, err
	}
	if written > int64(entry.UncompressedSize64) {
		return written, errors.New("An archive entry is larger than its declared size")
	}
	return written, nil
}

// target is the entry's path with the archive's own root removed, and false for
// the root entry itself.
//
// A leading slash is left where it is rather than trimmed off.  Trimming would
// quietly turn `/windows/system32/…` into a relative path and write it, which
// is containment by accident; a JDK archive has no absolute names in it, and
// one that does is worth stopping on.
func target(name, strip string) (string, bool) {
	name = path.Clean(strings.ReplaceAll(name, `\`, "/"))
	if strip != "" {
		if name == strip {
			return "", false
		}
		name = strings.TrimPrefix(name, strip+"/")
	}
	if name == "" || name == "." {
		return "", false
	}
	return filepath.FromSlash(name), true
}

// commonRoot is the single directory every entry sits inside, or an empty
// string when there is not exactly one.  Vendors do not agree on what to call
// it -- `jdk-25.0.4` from Oracle, `jdk-25.0.4.1+1` from Temurin -- so it is
// found rather than guessed, and an archive that is already flat is left alone.
func commonRoot(entries []*zip.File) string {
	root := ""
	for _, entry := range entries {
		name := strings.TrimPrefix(strings.ReplaceAll(entry.Name, `\`, "/"), "/")
		head, _, nested := strings.Cut(name, "/")
		if head == "" || head == "." || head == ".." {
			return ""
		}
		if !nested && !entry.FileInfo().IsDir() {
			return "" // a file at the top: the archive is flat
		}
		if root == "" {
			root = head
			continue
		}
		if root != head {
			return ""
		}
	}
	return root
}
