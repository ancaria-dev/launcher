package purehd

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ancaria-dev/launcher/game"
)

func archive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// work is a Work pointed at a test server, with a version check that believes
// whatever the fake exe says, since a test cannot build a real version resource.
func work(t *testing.T, url string, data []byte) (*Work, string, string) {
	t.Helper()
	gameDir := t.TempDir()
	loaderDir := filepath.Join(gameDir, "launcher")
	w := New(gameDir, loaderDir)
	w.url = url
	w.sum = digest(data)
	w.size = int64(len(data))
	w.version = func(exe string) string {
		body, _ := os.ReadFile(exe)
		return string(body)
	}
	return w, gameDir, loaderDir
}

func wait(t *testing.T, w *Work) Snapshot {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if snapshot := w.Snapshot(); !snapshot.Busy {
			return snapshot
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the job never finished")
	return Snapshot{}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestGetPlacesBothFilesBesideTheGameAndKeepsTheOldOnes(t *testing.T) {
	data := archive(t, map[string]string{game.Names[0]: game.Expected, game.Library: "library"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(data)
	}))
	defer server.Close()

	w, gameDir, loaderDir := work(t, server.URL, data)
	if err := os.WriteFile(filepath.Join(gameDir, "Sacred.exe"), []byte("stock"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "PUREHD.EXE"), []byte("older"), 0o644); err != nil {
		t.Fatal(err)
	}

	w.Get()
	snapshot := wait(t, w)
	if snapshot.Progress.Stage != StageDone {
		t.Fatalf("stage %q, error %q", snapshot.Progress.Stage, snapshot.Progress.Error)
	}
	if got := read(t, filepath.Join(gameDir, "Sacred.exe")); got != "stock" {
		t.Fatalf("Sacred.exe was changed to %q", got)
	}
	if got := read(t, filepath.Join(gameDir, game.Names[0])); got != game.Expected {
		t.Fatalf("pureHD.exe holds %q", got)
	}
	if got := read(t, filepath.Join(gameDir, game.Library)); got != "library" {
		t.Fatalf("pHD.dll holds %q", got)
	}
	kept, _ := filepath.Glob(filepath.Join(loaderDir, backupName, "*", "PUREHD.EXE"))
	if len(kept) != 1 || read(t, kept[0]) != "older" {
		t.Fatalf("the older wrapper was not kept: %v", kept)
	}
	for _, left := range []string{partName, stagingName} {
		if _, err := os.Stat(filepath.Join(loaderDir, left)); !os.IsNotExist(err) {
			t.Fatalf("%s was left behind", left)
		}
	}
}

func TestAChecksumMismatchChangesNothing(t *testing.T) {
	data := archive(t, map[string]string{game.Names[0]: game.Expected, game.Library: "library"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(data)
	}))
	defer server.Close()

	w, gameDir, loaderDir := work(t, server.URL, data)
	w.sum = digest([]byte("something else"))

	w.Get()
	snapshot := wait(t, w)
	if snapshot.Progress.Stage != StageFailed {
		t.Fatalf("stage %q", snapshot.Progress.Stage)
	}
	if _, err := os.Stat(filepath.Join(gameDir, game.Names[0])); !os.IsNotExist(err) {
		t.Fatal("a mismatched archive was installed")
	}
	if _, err := os.Stat(filepath.Join(loaderDir, partName)); !os.IsNotExist(err) {
		t.Fatal("the mismatched archive was kept")
	}
}

func TestTheWrongBuildInsideTheArchiveIsRefused(t *testing.T) {
	data := archive(t, map[string]string{game.Names[0]: "2.0.2.28", game.Library: "library"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(data)
	}))
	defer server.Close()

	w, gameDir, _ := work(t, server.URL, data)
	w.Get()
	if snapshot := wait(t, w); snapshot.Progress.Stage != StageFailed {
		t.Fatalf("stage %q", snapshot.Progress.Stage)
	}
	if _, err := os.Stat(filepath.Join(gameDir, game.Names[0])); !os.IsNotExist(err) {
		t.Fatal("the wrong build was installed")
	}
}

func TestAbortStopsTheDownload(t *testing.T) {
	data := archive(t, map[string]string{game.Names[0]: game.Expected, game.Library: "library"})
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		_, _ = w.Write(data[:10])
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	w, gameDir, loaderDir := work(t, server.URL, data)
	w.Get()
	<-started
	if !w.Snapshot().Abortable {
		t.Fatal("a download in progress is not abortable")
	}
	w.Abort()
	snapshot := wait(t, w)
	if snapshot.Progress.Stage != StageAborted {
		t.Fatalf("stage %q, error %q", snapshot.Progress.Stage, snapshot.Progress.Error)
	}
	if _, err := os.Stat(filepath.Join(gameDir, game.Names[0])); !os.IsNotExist(err) {
		t.Fatal("an aborted download was installed")
	}
	if _, err := os.Stat(filepath.Join(loaderDir, partName)); !os.IsNotExist(err) {
		t.Fatal("the aborted download was kept")
	}
}
