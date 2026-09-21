package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ancaria-dev/launcher/fetch"
)

// A Windows executable starts with these two bytes and the check that looks
// for them is the difference between installing a launcher and installing a
// captive-portal login page that arrived with a 200.
var exe = append([]byte("MZ"), make([]byte, 4094)...)

// asset is one file on a release. body defaults to a plausible executable, so
// a test only spells out the bytes when the bytes are the point.
type asset struct {
	Name string
	Size int64
	body []byte
}

// release stands in for the GitHub API and for the files behind it.
func release(t *testing.T, tag string, assets []asset) *httptest.Server {
	t.Helper()
	var made *httptest.Server
	made = httptest.NewTLSServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			if name := strings.TrimPrefix(request.URL.Path, "/dl/"); name != request.URL.Path {
				for _, one := range assets {
					if one.Name == name {
						body := one.body
						if body == nil {
							body = exe
						}
						_, _ = writer.Write(body)
						return
					}
				}
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			answer := map[string]any{
				"tag_name": tag,
				"html_url": made.URL + "/releases/" + tag,
			}
			var list []map[string]any
			for _, one := range assets {
				list = append(list, map[string]any{
					"name":                 one.Name,
					"size":                 one.Size,
					"browser_download_url": made.URL + "/dl/" + one.Name,
				})
			}
			answer["assets"] = list
			_ = json.NewEncoder(writer).Encode(answer)
		}))

	// The certificate is this server's own, and nothing else in the process
	// makes requests while a test is running.
	previous, endpoint := fetch.Transport, api
	fetch.Transport = made.Client().Transport
	api = made.URL + "/releases/latest"
	t.Cleanup(func() {
		fetch.Transport = previous
		api = endpoint
	})
	return made
}

func TestNewerNeedsBothSidesToParse(t *testing.T) {
	cases := []struct {
		installed, offered string
		want               bool
	}{
		{"0.101.0", "0.102.0", true},
		{"0.101.0", "0.101.0", false},
		{"0.102.0", "0.101.0", false},
		// The one that bites: as text "0.99.0" sorts above "0.100.0", and a
		// launcher that compared strings would offer everybody a downgrade.
		{"0.100.0", "0.99.0", false},
		{"0.99.0", "0.100.0", true},
		// A build from a checkout is newer than every release, not older than
		// all of them.
		{"dev", "0.102.0", false},
		{"0.101.0", "", false},
	}
	for _, c := range cases {
		if got := Newer(c.installed, c.offered); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.installed, c.offered, got, c.want)
		}
	}
}

func TestLatestTakesTheExecutableAndDropsTheV(t *testing.T) {
	made := release(t, "v0.102.0", []asset{
		{Name: "checksums.txt", Size: 12},
		// GitHub rewrites the spaces out of the name it was given, which is
		// why nothing here matches on one.
		{Name: "Sacred.Mod.Loader.exe", Size: int64(len(exe))},
	})
	defer made.Close()

	found, err := Latest()
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if found.Version != "0.102.0" {
		t.Errorf("version = %q, want 0.102.0", found.Version)
	}
	if found.Size != int64(len(exe)) {
		t.Errorf("size = %d, want %d", found.Size, len(exe))
	}
	if filepath.Ext(found.URL) != ".exe" {
		t.Errorf("URL = %q, want the executable asset", found.URL)
	}
}

func TestLatestRefusesAReleaseWithNoExecutable(t *testing.T) {
	made := release(t, "v0.102.0", []asset{{Name: "notes.md", Size: 4}})
	defer made.Close()

	if _, err := Latest(); err == nil {
		t.Fatal("a release carrying no executable must not be offered")
	}
}

func TestDownloadStagesInsideTheGameFolder(t *testing.T) {
	made := release(t, "v0.102.0", []asset{
		{Name: "Sacred.Mod.Loader.exe", Size: int64(len(exe))},
	})
	defer made.Close()

	found, err := Latest()
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	dir := t.TempDir()
	var last int64
	path, err := Download(dir, found, func(done, _ int64) { last = done })
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if path != Staged(dir) {
		t.Errorf("staged at %q, want %q", path, Staged(dir))
	}
	if last != int64(len(exe)) {
		t.Errorf("progress stopped at %d, want %d", last, len(exe))
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) != len(exe) {
		t.Fatalf("staged file is %d bytes: %v", len(data), err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "update", "*.part")); len(left) > 0 {
		t.Errorf("a partial file survived: %v", left)
	}
}

func TestDownloadKeepsNothingThatIsNotAnExecutable(t *testing.T) {
	page := []byte("<html>Sign in to continue</html>")
	made := release(t, "v0.102.0", []asset{
		{Name: "Sacred.Mod.Loader.exe", Size: int64(len(page)), body: page},
	})
	defer made.Close()

	found, _ := Latest()
	dir := t.TempDir()
	if _, err := Download(dir, found, nil); err == nil {
		t.Fatal("an HTML page must not be installed as the launcher")
	}
	if _, err := os.Stat(Staged(dir)); err == nil {
		t.Error("the staged file was kept")
	}
}

func TestDownloadKeepsNothingOfTheWrongSize(t *testing.T) {
	made := release(t, "v0.102.0", []asset{
		// The release says one thing and the server sends another, which is a
		// truncated transfer that a naive reader would treat as a whole file.
		{Name: "Sacred.Mod.Loader.exe", Size: int64(len(exe)) + 99, body: exe},
	})
	defer made.Close()

	found, _ := Latest()
	dir := t.TempDir()
	if _, err := Download(dir, found, nil); err == nil {
		t.Fatal("a short download must not be installed as the launcher")
	}
	if _, err := os.Stat(Staged(dir)); err == nil {
		t.Error("the staged file was kept")
	}
}

func TestApplyWithNothingStagedSaysSo(t *testing.T) {
	if err := Apply(t.TempDir()); err == nil {
		t.Fatal("Apply must refuse when no launcher was downloaded")
	}
}

func TestSweepClearsAnAbandonedDownload(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(Staged(dir)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Staged(dir), exe, 0o755); err != nil {
		t.Fatal(err)
	}
	Sweep(dir)
	if _, err := os.Stat(Staged(dir)); err == nil {
		t.Error("a downloaded launcher nobody installed was left behind")
	}
}
