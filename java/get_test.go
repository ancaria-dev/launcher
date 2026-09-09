package java

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A server on the loopback interface, not the internet: what is being tested is
// what Download does with the bytes, and a test that needs a network is a test
// that fails on a train.
func serving(t *testing.T, body []byte) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func digestOf(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func TestDownloadKeepsAFileThatMatchesItsChecksum(t *testing.T) {
	body := []byte(strings.Repeat("jdk", 5000))
	dir := t.TempDir()

	path, err := Download(dir, Pkg{Filename: "jdk.zip", Size: int64(len(body))},
		Link{URL: serving(t, body), Checksum: digestOf(body), Kind: "sha256"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(body) {
		t.Fatalf("wanted %d bytes on disk, got %d", len(body), len(got))
	}
}

// The one case that must not leave anything behind.  A half-written or
// tampered-with archive named like a whole one is unpacked on the next run into
// a java.exe that dies on the first class it reads, and nothing would say why.
func TestDownloadDeletesAFileThatDoesNot(t *testing.T) {
	body := []byte("this is not the JDK you were promised")
	dir := t.TempDir()

	_, err := Download(dir, Pkg{Filename: "jdk.zip"},
		Link{URL: serving(t, body), Checksum: digestOf([]byte("something else")), Kind: "sha256"}, nil)
	if err == nil {
		t.Fatal("wanted the download refused")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("the error does not say what was wrong: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "java.part")); err == nil {
		t.Fatal("the partial download is still there")
	}
}

// Not every distribution publishes a digest.  There is nothing to compare
// against, and refusing on that account would mean refusing perfectly good
// vendors.  A digest that disagrees is the case worth stopping for.
func TestDownloadAcceptsAPackageWithNoPublishedChecksum(t *testing.T) {
	body := []byte("no digest for this one")
	if _, err := Download(t.TempDir(), Pkg{Filename: "jdk.zip"},
		Link{URL: serving(t, body)}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadReportsProgress(t *testing.T) {
	body := []byte(strings.Repeat("x", 1<<20))
	var last int64
	_, err := Download(t.TempDir(), Pkg{Filename: "jdk.zip", Size: int64(len(body))},
		Link{URL: serving(t, body), Checksum: digestOf(body), Kind: "sha256"},
		func(done, _ int64) { last = done })
	if err != nil {
		t.Fatal(err)
	}
	if last != int64(len(body)) {
		t.Fatalf("wanted the bar to end at %d, got %d", len(body), last)
	}
}

// Install unpacks beside the copy already there and swaps at the end, so a
// download that fails halfway leaves a working JDK working.
func TestInstallReplacesTheCopyAlreadyThere(t *testing.T) {
	loader := t.TempDir()
	jdk(t, Dir(loader), "21.0.8")

	archive := archiveOf(t, map[string]string{
		"jdk-25.0.4/bin/java.exe": "MZ",
		"jdk-25.0.4/release":      "JAVA_VERSION=\"25.0.4\"\n",
	})
	found, err := Install(loader, archive, nil)
	if err != nil {
		t.Fatal(err)
	}
	if found.Version != "25.0.4" || !found.Usable() || found.Source != FromLoader {
		t.Fatalf("installed as %+v", found)
	}
	if _, err := os.Stat(found.Path); err != nil {
		t.Fatalf("wanted java.exe at %s, got %v", found.Path, err)
	}
	stale, _ := filepath.Glob(Dir(loader) + ".old-*")
	if len(stale) != 0 {
		t.Errorf("the replaced copy was left behind: %v", stale)
	}
}

func TestInstallRefusesAnArchiveThatIsNotAJdk(t *testing.T) {
	loader := t.TempDir()
	archive := archiveOf(t, map[string]string{"notes/readme.txt": "hello"})
	if _, err := Install(loader, archive, nil); err == nil {
		t.Fatal("wanted an archive with no java.exe refused")
	}
	if _, err := os.Stat(Dir(loader)); err == nil {
		t.Error("the failed attempt was left in place")
	}
}

func TestSweepClearsWhatAnInterruptedRunLeft(t *testing.T) {
	loader := t.TempDir()
	part := filepath.Join(loader, "java.part")
	if err := os.WriteFile(part, []byte("half a JDK"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(Dir(loader)+".new", 0o755); err != nil {
		t.Fatal(err)
	}

	Sweep(loader)

	if _, err := os.Stat(part); err == nil {
		t.Error("the partial download survived")
	}
	if _, err := os.Stat(Dir(loader) + ".new"); err == nil {
		t.Error("the staging folder survived")
	}
}
