// Package java finds a JDK for the mod loader, and fetches one when the machine
// has none.
//
// The loader's whole Java side -- the zygote and every mod in it -- runs in a
// JVM the host starts as a child process.  No JDK, no mods: the game still
// launches and nothing at all happens, which is the confusing failure this
// package exists to turn into an explained one.
//
// A downloaded JDK goes into `<game>/launcher/java` and nowhere else.
// Uninstalling this loader is deleting the game folder, and that stays true
// only while nothing is written outside it.
package java

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Minimum is the oldest Java the zygote is built for.  Anything older is not a
// smaller problem than no Java at all -- the class files simply will not load.
const Minimum = 21

// Where a JDK came from, as the player is told it.  Naming the source is the
// point: a downloaded copy is preferred over a working system one, and silently
// using the wrong `java` is how somebody ends up debugging the loader with a
// JDK they replaced an hour ago.
const (
	FromLoader = "downloaded by the launcher"
	FromHome   = "JAVA_HOME"
	FromPath   = "PATH"
)

// Found is one JDK on this machine.  Path is empty when there is none.
type Found struct {
	Path    string
	Home    string
	Version string
	Major   int
	Source  string
}

// Usable reports whether the loader can run on it.
func (f Found) Usable() bool { return f.Path != "" && f.Major >= Minimum }

// Dir is where a JDK the launcher fetched lives: inside the game folder, beside
// the host it is started by.
func Dir(loaderDir string) string { return filepath.Join(loaderDir, "java") }

// Find looks for a JDK in the order the loader will use one: the copy this
// launcher downloaded first, then JAVA_HOME, then PATH.
//
// The first usable one wins.  When none of them is new enough, the first that
// exists comes back anyway, so the page can say "Java 17, and the loader needs
// 21" rather than the much less useful "no Java".
func Find(loaderDir string) Found {
	var first Found
	for _, candidate := range candidates(loaderDir) {
		if candidate.Path == "" {
			continue
		}
		candidate.Version = version(candidate.Home, candidate.Path)
		candidate.Major = Major(candidate.Version)
		if candidate.Usable() {
			return candidate
		}
		if first.Path == "" {
			first = candidate
		}
	}
	return first
}

// candidates is the search order, before any of them is asked its version.
func candidates(loaderDir string) []Found {
	found := make([]Found, 0, 3)
	if home := local(Dir(loaderDir)); home != "" {
		found = append(found, Found{Path: exeIn(home), Home: home, Source: FromLoader})
	}
	if home := strings.TrimSpace(os.Getenv("JAVA_HOME")); home != "" {
		if isJDK(home) {
			found = append(found, Found{Path: exeIn(home), Home: home, Source: FromHome})
		}
	}
	if path, err := exec.LookPath("java"); err == nil {
		// <home>/bin/java.exe: the home is two levels up from the executable.
		home := filepath.Dir(filepath.Dir(path))
		found = append(found, Found{Path: path, Home: home, Source: FromPath})
	}
	return found
}

// local is the JDK the launcher unpacked.  Extraction strips the archive's own
// top directory, so `java/bin/java.exe` is the normal shape -- but a copy
// somebody unzipped by hand keeps it, and refusing that would be pedantry.
func local(dir string) string {
	if isJDK(dir) {
		return dir
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if nested := filepath.Join(dir, entry.Name()); isJDK(nested) {
			return nested
		}
	}
	return ""
}

func isJDK(home string) bool {
	info, err := os.Stat(exeIn(home))
	return err == nil && !info.IsDir()
}

// The launcher is a Windows program throughout -- everything around it spells
// the game and the host with .exe -- so the JDK is spelled the same way.
func exeIn(home string) string {
	return filepath.Join(home, "bin", "java.exe")
}

// version reads the JDK's own `release` file, which every build writes and
// which costs no process.  A java on PATH belonging to something that is not a
// JDK layout falls back to asking the binary itself.
//
// Which is why the executable is passed in rather than rebuilt out of the home:
// the common `java` on a Windows machine is Oracle's `javapath` shim, a
// directory of small launchers with no `release` beside them and no
// `bin/java.exe` under the folder above them.  Asking `<home>/bin/java.exe`
// there is asking a file that does not exist, and the answer is an empty
// version, a major of 0, and a page saying there is no Java on a machine that
// has one.
func version(home, exe string) string {
	if value := releaseFile(filepath.Join(home, "release")); value != "" {
		return value
	}
	if exe == "" {
		exe = exeIn(home)
	}
	return ask(exe)
}

func releaseFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found || key != "JAVA_VERSION" {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"`)
	}
	return ""
}

// ask runs `java -version`, which prints to stderr and looks like
// `openjdk version "21.0.8" 2025-07-15`.
func ask(exe string) string {
	command := exec.Command(exe, "-version")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := command.CombinedOutput()
	if err != nil && len(output) == 0 {
		return ""
	}
	_, rest, found := strings.Cut(string(output), `version "`)
	if !found {
		return ""
	}
	value, _, _ := strings.Cut(rest, `"`)
	return value
}

// Major is the feature release number in a Java version string.
//
// It has to survive every spelling in circulation: `21`, `25.0.4`,
// `25.0.4.1+1`, `26-ea`, and the old `1.8.0_452` where the number that matters
// is the second one.
func Major(version string) int {
	version = strings.Trim(strings.TrimSpace(version), `"`)
	if version == "" {
		return 0
	}
	parts := strings.FieldsFunc(version, func(r rune) bool {
		return r == '.' || r == '-' || r == '+' || r == '_'
	})
	if len(parts) == 0 {
		return 0
	}
	head := parts[0]
	// 1.8.0 is Java 8, and nothing since 9 spells itself that way.
	if head == "1" && len(parts) > 1 {
		head = parts[1]
	}
	value, err := strconv.Atoi(head)
	if err != nil {
		return 0
	}
	return value
}
