package java

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// pretendJava turns this test binary into a stand-in for a `java.exe`.  A shim
// has to be executed to be asked its version, os/exec on Windows cannot run a
// .bat, and the only executable a test can be sure exists is the one it is
// running in.
const pretendJava = "SML_TEST_JAVA_VERSION"

func TestMain(m *testing.M) {
	if version := os.Getenv(pretendJava); version != "" {
		// What `java -version` writes, on the stream it writes it to.
		fmt.Fprintf(os.Stderr, "openjdk version %q 2026-01-01\n", version)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// shim copies this test binary somewhere and calls it java.exe: a launcher with
// no `release` file beside it and no JDK layout around it, which is exactly the
// shape of the `javapath` directory Oracle's installer puts on PATH.
func shim(t *testing.T, dir string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "java.exe"), data, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// jdk writes the two files Find reads: the executable it looks for and the
// `release` file every real JDK carries, which is where the version comes from
// without starting a process.
func jdk(t *testing.T, home, version string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "bin", "java.exe"), []byte("not really"), 0o755); err != nil {
		t.Fatal(err)
	}
	release := "IMPLEMENTOR=\"Test\"\nJAVA_VERSION=\"" + version + "\"\nOS_ARCH=\"x86_64\"\n"
	if err := os.WriteFile(filepath.Join(home, "release"), []byte(release), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

// noJavaOnPath points PATH at an empty directory, so a JDK installed on the
// machine running the tests cannot decide their outcome.
func noJavaOnPath(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("JAVA_HOME", "")
}

func TestFindPrefersTheCopyItDownloadedItself(t *testing.T) {
	noJavaOnPath(t)
	loader := t.TempDir()
	jdk(t, Dir(loader), "25.0.4")
	t.Setenv("JAVA_HOME", jdk(t, filepath.Join(t.TempDir(), "system"), "21.0.8"))

	found := Find(loader)
	if found.Source != FromLoader {
		t.Fatalf("wanted the downloaded copy, got %q from %q", found.Version, found.Source)
	}
	if found.Version != "25.0.4" || found.Major != 25 || !found.Usable() {
		t.Fatalf("wanted a usable 25.0.4, got %+v", found)
	}
}

func TestFindFallsBackToJavaHome(t *testing.T) {
	noJavaOnPath(t)
	loader := t.TempDir()
	t.Setenv("JAVA_HOME", jdk(t, filepath.Join(t.TempDir(), "system"), "21.0.8"))

	found := Find(loader)
	if found.Source != FromHome || found.Major != 21 {
		t.Fatalf("wanted JAVA_HOME's 21, got %+v", found)
	}
}

func TestFindFallsBackToPath(t *testing.T) {
	noJavaOnPath(t)
	loader := t.TempDir()
	home := jdk(t, filepath.Join(t.TempDir(), "onpath"), "24.0.1")
	t.Setenv("PATH", filepath.Join(home, "bin"))

	found := Find(loader)
	if found.Source != FromPath || found.Major != 24 {
		t.Fatalf("wanted PATH's 24, got %+v", found)
	}
}

// The `java` most Windows machines have on PATH is Oracle's javapath shim: no
// `release` file beside it, and nothing laid out like a JDK above it.  Rebuilding
// the executable's path out of that home asks a file that does not exist, and the
// launcher then tells somebody with Java 25 installed that they have no Java.
func TestFindAsksAShimOnPathForItsVersion(t *testing.T) {
	noJavaOnPath(t)
	loader := t.TempDir()
	t.Setenv("PATH", shim(t, filepath.Join(t.TempDir(), "Oracle", "Java", "javapath")))
	t.Setenv(pretendJava, "25.0.4.1")

	found := Find(loader)
	if found.Source != FromPath || found.Major != 25 {
		t.Fatalf("wanted the shim's own 25, got %+v", found)
	}
	if !found.Usable() {
		t.Fatalf("wanted a usable JDK, got %+v", found)
	}
}

// The order is a preference, not a rule that overrides whether the thing works.
// A downloaded JDK too old for the zygote must not shadow a system one that is
// new enough, or the launcher would fetch a JDK and then refuse to run on it.
func TestFindSkipsAnythingOlderThanTheLoaderNeeds(t *testing.T) {
	noJavaOnPath(t)
	loader := t.TempDir()
	jdk(t, Dir(loader), "17.0.9")
	t.Setenv("JAVA_HOME", jdk(t, filepath.Join(t.TempDir(), "system"), "21.0.8"))

	found := Find(loader)
	if found.Source != FromHome || found.Major != 21 {
		t.Fatalf("wanted the newer JAVA_HOME copy, got %+v", found)
	}
}

// With nothing new enough anywhere, the first thing found still comes back, so
// the page can say which Java is there and why it will not do.
func TestFindReportsTooOldRatherThanNothing(t *testing.T) {
	noJavaOnPath(t)
	loader := t.TempDir()
	jdk(t, Dir(loader), "1.8.0_452")

	found := Find(loader)
	if found.Path == "" {
		t.Fatal("wanted the old JDK reported, got nothing at all")
	}
	if found.Major != 8 || found.Usable() {
		t.Fatalf("wanted an unusable Java 8, got %+v", found)
	}
}

// A copy somebody unzipped by hand keeps the archive's own top directory.
func TestFindLooksOneLevelInside(t *testing.T) {
	noJavaOnPath(t)
	loader := t.TempDir()
	jdk(t, filepath.Join(Dir(loader), "jdk-25.0.4"), "25.0.4")

	found := Find(loader)
	if found.Source != FromLoader || found.Major != 25 {
		t.Fatalf("wanted the nested copy, got %+v", found)
	}
}

func TestFindOnAMachineWithNoJava(t *testing.T) {
	noJavaOnPath(t)
	found := Find(t.TempDir())
	if found.Path != "" || found.Usable() {
		t.Fatalf("wanted nothing found, got %+v", found)
	}
}

func TestMajor(t *testing.T) {
	cases := map[string]int{
		"25.0.4":       25,
		"25.0.4.1+1":   25,
		"21":           21,
		"21.0.8+9":     21,
		"26-ea":        26,
		"1.8.0_452":    8,
		"1.7.0":        7,
		`"25.0.4"`:     25,
		" 24.0.1 ":     24,
		"":             0,
		"not a number": 0,
	}
	for version, want := range cases {
		if got := Major(version); got != want {
			t.Errorf("Major(%q) = %d, wanted %d", version, got, want)
		}
	}
}
