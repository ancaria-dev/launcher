// Package update is the launcher keeping itself current.
//
// The whole loader travels inside this one executable: the host, the jars, the
// agent minified into the host. Replacing the executable and starting it again
// is therefore the entire upgrade, because the new one unpacks its own payload
// on the first run that finds a different VERSION in the game folder.
//
// Which is also why this exists at all. A player who never notices a release
// keeps a host and a set of jars that were current the day they downloaded the
// launcher, and every mod they install afterwards is measured against those.
package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ancaria-dev/launcher/fetch"
	"github.com/ancaria-dev/launcher/pin"
)

// Repo is the one this launcher upgrades itself from. Written here and nowhere
// else, and never taken from a downloaded file: an update that can be pointed
// somewhere else is an update that can be pointed anywhere.
const Repo = "ancaria-dev/launcher"

// api is the release the project published last.
//
// A variable for the same reason fetch.Transport is one: the alternative is an
// updater that is only ever exercised against a fake of itself. Unexported, so
// the only thing that can move it is a test in this package.
var api = "https://api.github.com/repos/" + Repo + "/releases/latest"

// The answer is a few kilobytes of JSON. The cap is not an expectation.
const answerLimit = 512 << 10

// Release is the newest published launcher and where to get it.
type Release struct {
	// Version is the tag without its v, which is what .version holds and what
	// a player is shown.
	Version string `json:"version"`
	URL     string `json:"-"`
	Size    int64  `json:"size"`
	// Page is the release on github.com, for the player who has to fetch it by
	// hand because something here did not work.
	Page string `json:"page"`
}

// Latest asks GitHub what the newest release is.
func Latest() (Release, error) {
	head := http.Header{}
	// Asking for the documented media type rather than whatever the default
	// becomes: this parses two fields out of that answer and they are only
	// promised in the versioned one.
	head.Set("Accept", "application/vnd.github+json")
	head.Set("X-GitHub-Api-Version", "2022-11-28")

	data, err := fetch.Bytes(api, head, answerLimit)
	if err != nil {
		return Release{}, err
	}
	var answer struct {
		Tag    string `json:"tag_name"`
		Page   string `json:"html_url"`
		Draft  bool   `json:"draft"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(data, &answer); err != nil {
		return Release{}, fmt.Errorf("GitHub's answer could not be read: %w", err)
	}
	if answer.Draft || answer.Tag == "" {
		return Release{}, errors.New("GitHub named no published release")
	}

	release := Release{
		Version: strings.TrimPrefix(answer.Tag, "v"),
		Page:    answer.Page,
	}
	for _, asset := range answer.Assets {
		// By extension rather than by name. GitHub rewrites the spaces in
		// "Sacred Mod Loader.exe" into dots on the way out, and a launcher that
		// matched the name it uploaded would stop finding its own release the
		// day somebody renamed the artifact.
		if strings.EqualFold(filepath.Ext(asset.Name), ".exe") {
			release.URL = asset.URL
			release.Size = asset.Size
			break
		}
	}
	if release.URL == "" {
		return Release{}, errors.New("The newest release carries no executable")
	}
	return release, nil
}

// Newer says whether offered is worth telling somebody about.
//
// Both sides have to parse. A launcher built from a checkout calls itself
// "dev", which is not a version and must not be treated as an old one: that
// player has a build newer than any release and no business being offered it.
func Newer(installed, offered string) bool {
	here, there := pin.Parse(installed), pin.Parse(offered)
	return here.Known() && there.Known() && there.Compare(here) > 0
}

// Staged is where a downloaded launcher waits: inside the game folder, beside
// everything else this loader owns.
//
// Not %TEMP%. Uninstalling this loader is deleting the game folder, and that
// sentence stops being true the moment something is written outside it. It is
// also the only place the swap can be a rename rather than a copy, since
// %TEMP% is regularly on another volume.
func Staged(loaderDir string) string {
	return filepath.Join(loaderDir, "update", "launcher.exe")
}

// Download fetches the release and leaves it staged, or leaves nothing.
//
// The local name is fixed and the downloaded one is never read. Nothing off
// the network decides a path here any more than it does for a mod jar.
func Download(loaderDir string, release Release, progress func(done, total int64)) (string, error) {
	target := Staged(loaderDir)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	part := target + ".part"
	_ = os.Remove(part)

	// One byte past the advertised size, so a body that is longer than it said
	// fails the size check below instead of being silently cut to fit it.
	var max int64
	if release.Size > 0 {
		max = release.Size + 1
	}
	if _, err := fetch.File(release.URL, nil, part, max, progress); err != nil {
		return "", err
	}
	if err := check(part, release.Size); err != nil {
		_ = os.Remove(part)
		return "", err
	}
	_ = os.Remove(target)
	if err := os.Rename(part, target); err != nil {
		_ = os.Remove(part)
		return "", err
	}
	return target, nil
}

// check is what can be checked without a digest.
//
// GitHub publishes no checksum beside the asset, so there is no published
// number to compare against and saying otherwise would be a lie. What is left
// is worth doing anyway: both of these catch the common failure, which is an
// error page or a proxy's interstitial arriving with a 200 and being run as
// the launcher.
func check(path string, expected int64) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if expected > 0 && info.Size() != expected {
		return fmt.Errorf("The download is %d bytes and the release says %d",
			info.Size(), expected)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	var magic [2]byte
	if _, err := file.Read(magic[:]); err != nil {
		return err
	}
	if magic != [2]byte{'M', 'Z'} {
		return errors.New("What arrived is not a Windows executable")
	}
	return nil
}

// Apply puts the staged launcher in this one's place and starts it.
//
// Windows will not let a running executable be deleted, but it will let one be
// renamed, which is the whole trick and the same one install.write uses for the
// host. The old copy stays as .old until the next start sweeps it: it cannot go
// now, because it is the code executing this line.
//
// The caller closes the window afterwards. Both launchers are alive for that
// moment, which is why the new one is started last.
func Apply(loaderDir string) error {
	staged := Staged(loaderDir)
	if _, err := os.Stat(staged); err != nil {
		return errors.New("The downloaded launcher is no longer there")
	}
	running, err := os.Executable()
	if err != nil {
		return err
	}

	stale := running + ".old"
	_ = os.Remove(stale)
	if err := os.Rename(running, stale); err != nil {
		return fmt.Errorf("Could not move the running launcher aside: %w", err)
	}
	if err := os.Rename(staged, running); err != nil {
		// Back where it was. A game folder with no launcher in it is a worse
		// outcome than a failed update, and this is the one window where that
		// can happen.
		_ = os.Rename(stale, running)
		return fmt.Errorf("Could not put the new launcher in place: %w", err)
	}

	started := exec.Command(running)
	started.Dir = filepath.Dir(running)
	if err := started.Start(); err != nil {
		return fmt.Errorf("The new launcher is installed but would not start: %w", err)
	}
	return nil
}

// Sweep clears what an update left behind: the launcher it replaced, and a
// staged copy that was downloaded and never applied.
//
// Called at startup, the way java.Sweep is, because neither can be removed
// while the process that would remove them is the one holding them open.
func Sweep(loaderDir string) {
	if running, err := os.Executable(); err == nil {
		_ = os.Remove(running + ".old")
	}
	_ = os.RemoveAll(filepath.Dir(Staged(loaderDir)))
}
