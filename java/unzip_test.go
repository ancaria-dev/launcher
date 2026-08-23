package java

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// archiveOf writes a zip whose entries are exactly the names given.  A name
// ending in / is a directory.
func archiveOf(t *testing.T, entries map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "jdk.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	for name, body := range entries {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(name, "/") {
			continue
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// Vendors do not agree on what to call the folder inside the archive -- Oracle
// ships jdk-25.0.4, Temurin ships jdk-25.0.4.1+1 -- so it is found and dropped
// rather than named, and Find only ever has to look at java/bin/java.exe.
func TestUnzipDropsTheArchivesOwnRoot(t *testing.T) {
	archive := archiveOf(t, map[string]string{
		"jdk-25.0.4/":             "",
		"jdk-25.0.4/release":      "JAVA_VERSION=\"25.0.4\"\n",
		"jdk-25.0.4/bin/":         "",
		"jdk-25.0.4/bin/java.exe": "MZ",
		"jdk-25.0.4/lib/rt.txt":   "classes",
	})
	dir := filepath.Join(t.TempDir(), "java")
	if err := Unzip(archive, dir, nil); err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{"bin/java.exe", "release", "lib/rt.txt"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(wanted))); err != nil {
			t.Errorf("wanted %s under the target, got %v", wanted, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "jdk-25.0.4")); err == nil {
		t.Error("the archive's own directory was kept")
	}
}

// An archive that is already flat, or one holding two top-level folders, has no
// root to drop and must be written as it is.
func TestUnzipKeepsAFlatArchive(t *testing.T) {
	archive := archiveOf(t, map[string]string{
		"bin/java.exe": "MZ",
		"release":      "JAVA_VERSION=\"25\"\n",
	})
	dir := filepath.Join(t.TempDir(), "java")
	if err := Unzip(archive, dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "bin", "java.exe")); err != nil {
		t.Fatalf("wanted bin/java.exe, got %v", err)
	}
}

// A zip is a list of names somebody else wrote, and nothing stops one of them
// climbing out of the folder it is being written into.
func TestUnzipRefusesToWriteOutsideItsFolder(t *testing.T) {
	cases := map[string]string{
		"climbing out":     "jdk-25.0.4/../../owned.txt",
		"climbing further": "jdk-25.0.4/bin/../../../../owned.txt",
		"absolute":         "/windows/system32/owned.txt",
		"backslashes":      `jdk-25.0.4\..\..\owned.txt`,
	}
	for what, name := range cases {
		t.Run(what, func(t *testing.T) {
			archive := archiveOf(t, map[string]string{
				"jdk-25.0.4/bin/java.exe": "MZ",
				name:                      "owned",
			})
			parent := t.TempDir()
			dir := filepath.Join(parent, "java")
			err := Unzip(archive, dir, nil)
			if err == nil {
				t.Fatal("wanted the extraction refused")
			}
			if !strings.Contains(err.Error(), "outside") {
				t.Errorf("the error does not say what happened: %v", err)
			}
			if _, err := os.Stat(filepath.Join(parent, "owned.txt")); err == nil {
				t.Fatal("a file was written outside the target folder")
			}
		})
	}
}

func TestUnzipReportsProgress(t *testing.T) {
	archive := archiveOf(t, map[string]string{
		"jdk-25/bin/java.exe": strings.Repeat("M", 4096),
		"jdk-25/release":      "JAVA_VERSION=\"25\"\n",
	})
	var last, total int64
	err := Unzip(archive, filepath.Join(t.TempDir(), "java"), func(done, of int64) {
		if done < last {
			t.Errorf("progress went backwards: %d after %d", done, last)
		}
		last, total = done, of
	})
	if err != nil {
		t.Fatal(err)
	}
	if last == 0 || last != total {
		t.Fatalf("wanted the bar to end full, got %d of %d", last, total)
	}
}
