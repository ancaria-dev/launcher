package registry

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ancaria-dev/launcher/fetch"
)

// An icon is decoration. A quarter of a megabyte is generous for something
// drawn at 64 pixels, and small enough that a repository cannot make the
// launcher hold a gallery in memory.
const iconLimit = 256 << 10

// Icons is the picture beside each mod, cached in the game folder.
//
// They reach the page as data URIs rather than as files. The window is loaded
// from a string, so it has no origin and no directory to be relative to: a
// `file://` path in there resolves to nothing at all. Base64 in the markup is
// what is left, which is also why the cap above is small and why the page asks
// for one icon at a time instead of being handed all of them on every poll.
//
// The cache is keyed by mod id, not by URL. An id is unique across the list by
// construction (two repositories offering one id is a conflict and neither is
// shown) and a name that survives is what lets an installed mod keep its icon
// on a day the repository it came from is not answering.
type Icons struct {
	dir string

	mu    sync.Mutex
	ready map[string]string
	tried map[string]bool
}

func NewIcons(loaderDir string) *Icons {
	return &Icons{
		dir:   filepath.Join(loaderDir, "cache", "icons"),
		ready: map[string]string{},
		tried: map[string]bool{},
	}
}

// Get is one icon for the page, or an empty string when there is not one yet.
// An empty answer is not a failure: the page draws its own mark, which is what
// most mods will be shown with for a while yet.
func (i *Icons) Get(id string) string {
	i.mu.Lock()
	if uri, found := i.ready[id]; found {
		i.mu.Unlock()
		return uri
	}
	i.mu.Unlock()

	data, err := os.ReadFile(i.file(id))
	uri := ""
	if err == nil {
		uri = encode(data)
	}
	i.mu.Lock()
	i.ready[id] = uri
	i.mu.Unlock()
	return uri
}

// Want fetches an icon that is not cached yet.  Called for every entry after an
// index arrives. The ones already on disk cost a map lookup and no request.
func (i *Icons) Want(id, url string, head http.Header) {
	if url == "" {
		return
	}
	i.mu.Lock()
	if i.tried[id] {
		i.mu.Unlock()
		return
	}
	i.tried[id] = true
	i.mu.Unlock()

	if _, err := os.Stat(i.file(id)); err == nil {
		return
	}
	data, err := fetch.Bytes(url, head, iconLimit)
	if err != nil || encode(data) == "" {
		return
	}
	if err := os.MkdirAll(i.dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(i.file(id), data, 0o644)

	i.mu.Lock()
	delete(i.ready, id)
	i.mu.Unlock()
}

// Forget drops what is cached so the next look asks again.  What Refresh means
// for pictures.
func (i *Icons) Forget() {
	i.mu.Lock()
	i.ready = map[string]string{}
	i.tried = map[string]bool{}
	i.mu.Unlock()
	_ = os.RemoveAll(i.dir)
}

func (i *Icons) file(id string) string {
	// The id is checked before it ever reaches here, and checked again: this is
	// the one place a name off the network becomes a path.
	if !validID(id) {
		return filepath.Join(i.dir, "invalid")
	}
	return filepath.Join(i.dir, id)
}

// encode turns bytes into a data URI, and refuses anything that is not a
// picture. A repository can put whatever it likes at that path. What the page
// gets told it is deciding by sniffing the bytes rather than by trusting the
// name.
func encode(data []byte) string {
	kind := http.DetectContentType(data)
	if !strings.HasPrefix(kind, "image/") || strings.Contains(kind, "svg") {
		// SVG is a document with scripting in it, and this one would be
		// rendered inside the launcher's own page.
		return ""
	}
	return "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(data)
}
