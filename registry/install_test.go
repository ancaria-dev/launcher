package registry

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ancaria-dev/launcher/fetch"
)

// A repository is a web server with a handful of paths on it, so the tests
// below are a web server with a handful of paths on it. Nothing is faked at the
// package boundary: the index is parsed, the jar is downloaded, hashed, opened
// and read, and what ends up in the folder is what a player would have.
type repo struct {
	*httptest.Server
	files map[string][]byte
	seen  []string
}

func serve(t *testing.T) *repo {
	t.Helper()
	made := &repo{files: map[string][]byte{}}
	made.Server = httptest.NewTLSServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			made.seen = append(made.seen, request.URL.Path)
			body, found := made.files[request.URL.Path]
			if !found {
				http.NotFound(writer, request)
				return
			}
			_, _ = writer.Write(body)
		}))
	// The certificate is this server's own, and nothing else in the process
	// makes requests while a test is running.
	fetch.Transport = made.Client().Transport
	t.Cleanup(func() {
		made.Close()
		fetch.Transport = http.DefaultTransport
	})
	return made
}

// clone is the address a player would paste for this server.
func (r *repo) clone() string { return r.URL + "/someone/mods.git" }

// gitea is the shape this server answers on: not the first one tried, so
// resolving it proves the others were tried and passed over.
func (r *repo) put(path string, body []byte) {
	r.files["/someone/mods/raw/branch/main/"+path] = body
}

func modJar(t *testing.T, id, version, api string) []byte {
	t.Helper()
	out := &bytes.Buffer{}
	archive := zip.NewWriter(out)
	entry, err := archive.Create("META-INF/declaration.toml")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(entry, "id = %q\nname = \"A Mod\"\nversion = %q\napi = %q\n", id, version, api)
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func sum(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func index(server *repo, entries ...string) []byte {
	return []byte(`{"srml": 1, "name": "Test Repo", "url": "` + server.URL +
		`", "mods": [` + strings.Join(entries, ",") + `]}`)
}

func entry(server *repo, id, version string, jar []byte, digest string) string {
	return fmt.Sprintf(`{"id": %q, "name": "A Mod", "version": %q, "api": "1",
		"icon": "%s/icon.png", "file": "a.jar", "size": %d, "sha256": %q,
		"url": "%s/jars/%s.jar"}`,
		id, version, id, len(jar), digest, server.URL, id)
}

func TestResolveTriesEveryShapeUntilOneAnswers(t *testing.T) {
	server := serve(t)
	jar := modJar(t, "a-mod", "1.0.0", "3")
	server.put(IndexFile, index(server, entry(server, "a-mod", "1.0.0", jar, sum(jar))))

	remote, found, err := Resolve(server.clone(), "")
	if err != nil {
		t.Fatal(err)
	}
	if remote.Name != "Test Repo" || len(found.Mods) != 1 {
		t.Fatalf("resolved to %+v with %+v", remote, found)
	}
	// The GitLab shape first, then the one that answered. Remembering it is the
	// point: nothing after this pays for the miss.
	if len(server.seen) != 2 || !strings.Contains(server.seen[0], "/-/raw/") {
		t.Fatalf("asked for %v", server.seen)
	}
	if !strings.Contains(remote.Raw, "/raw/branch/main/") {
		t.Fatalf("kept the wrong shape: %s", remote.Raw)
	}
}

func TestResolveSaysWhenThereIsNoIndex(t *testing.T) {
	server := serve(t)
	if _, _, err := Resolve(server.clone(), ""); err == nil ||
		!strings.Contains(err.Error(), IndexFile) {
		t.Fatalf("error was %v", err)
	}
}

func TestInstallLeavesTheJarWhereTheLoaderLooks(t *testing.T) {
	server := serve(t)
	jar := modJar(t, "a-mod", "1.0.0", "3")
	server.files["/jars/a-mod.jar"] = jar
	server.put(IndexFile, index(server, entry(server, "a-mod", "1.0.0", jar, sum(jar))))

	remote, found, err := Resolve(server.clone(), "")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := Install(here, dir, remote, found.Mods[0], nil); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(dir, "a-mod-1.0.0.jar")
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("nothing was installed: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(left) != 0 {
		t.Fatalf("a part file was left behind: %v", left)
	}

	// An update: same id, new version, and the old jar is gone rather than
	// sitting beside it for the loader to choose between.
	newer := modJar(t, "a-mod", "2.0.0", "3")
	server.files["/jars/a-mod.jar"] = newer
	server.put(IndexFile, index(server, entry(server, "a-mod", "2.0.0", newer, sum(newer))))
	updated, _, err := Load(remote)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(here, dir, remote, updated.Mods[0], nil); err != nil {
		t.Fatal(err)
	}
	jars, _ := filepath.Glob(filepath.Join(dir, "*.jar"))
	if len(jars) != 1 || filepath.Base(jars[0]) != "a-mod-2.0.0.jar" {
		t.Fatalf("the folder holds %v", jars)
	}
}

func TestInstallRefusesBytesThatAreNotWhatWasPublished(t *testing.T) {
	server := serve(t)
	jar := modJar(t, "a-mod", "1.0.0", "3")
	other := modJar(t, "a-mod", "9.9.9", "3")
	server.files["/jars/a-mod.jar"] = other
	server.put(IndexFile, index(server, entry(server, "a-mod", "1.0.0", jar, sum(jar))))

	remote, found, err := Resolve(server.clone(), "")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	err = Install(here, dir, remote, found.Mods[0], nil)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("error was %v", err)
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Fatalf("something was left behind: %v", left)
	}
}

func TestInstallRefusesAJarThatIsNotTheModItWasOfferedAs(t *testing.T) {
	server := serve(t)
	// The index offers a-mod. The jar behind it says it is something else.
	jar := modJar(t, "something-else", "1.0.0", "3")
	server.files["/jars/a-mod.jar"] = jar
	server.put(IndexFile, index(server, entry(server, "a-mod", "1.0.0", jar, sum(jar))))

	remote, found, _ := Resolve(server.clone(), "")
	dir := t.TempDir()
	err := Install(here, dir, remote, found.Mods[0], nil)
	if err == nil || !strings.Contains(err.Error(), "something-else") {
		t.Fatalf("error was %v", err)
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Fatalf("something was left behind: %v", left)
	}
}

func TestInstallRefusesAModForAnotherLoader(t *testing.T) {
	server := serve(t)
	jar := modJar(t, "a-mod", "1.0.0", "0")
	server.files["/jars/a-mod.jar"] = jar
	server.put(IndexFile, index(server, entry(server, "a-mod", "1.0.0", jar, sum(jar))))

	remote, found, _ := Resolve(server.clone(), "")
	err := Install(here, t.TempDir(), remote, found.Mods[0], nil)
	if err == nil || !strings.Contains(err.Error(), "loader API") {
		t.Fatalf("error was %v", err)
	}
}

func TestIconsArriveAsDataURIsAndNothingElseDoes(t *testing.T) {
	server := serve(t)
	// The eight bytes every PNG starts with are enough for the sniffer, which is
	// what decides what the page is told this is.
	png := append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, make([]byte, 64)...)
	server.put("a-mod/icon.png", png)
	server.put("b-mod/icon.png", []byte("<svg onload=\"alert(1)\"></svg>"))

	icons := NewIcons(t.TempDir())
	remote := Remote{Raw: server.URL + "/someone/mods/raw/branch/main/{path}"}
	icons.Want("a-mod", remote.File("a-mod/icon.png"), nil)
	icons.Want("b-mod", remote.File("b-mod/icon.png"), nil)

	if got := icons.Get("a-mod"); !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("the icon came back as %.40q", got)
	}
	if got := icons.Get("b-mod"); got != "" {
		t.Fatalf("a document was handed to the page as a picture: %.40q", got)
	}
}

func TestLoadExplainsARepositoryThatWantsAToken(t *testing.T) {
	server := serve(t)
	remote := Remote{URL: server.clone(), Raw: server.URL + "/someone/mods/raw/branch/main/{path}"}
	_, _, err := Load(remote)
	if err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("error was %v", err)
	}
}
