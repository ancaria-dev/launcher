package java

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// Every fixture under testdata is an answer the live foojay service actually
// gave, saved verbatim.  A fixture written from the structs would only prove
// the structs match the fixture.
func answer(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestReadVendors(t *testing.T) {
	vendors, err := readVendors(answer(t, "distributions.json"))
	if err != nil {
		t.Fatal(err)
	}

	by := map[string]Vendor{}
	for _, vendor := range vendors {
		by[vendor.ID] = vendor
	}
	for _, wanted := range []string{"oracle", "temurin", "zulu", "corretto"} {
		if _, ok := by[wanted]; !ok {
			t.Errorf("wanted %s in the list, got %d distributions without it", wanted, len(vendors))
		}
	}
	if oracle := by["oracle"]; oracle.Name != "Oracle" || oracle.Site == "" {
		t.Errorf("oracle came back as %+v", oracle)
	}

	// Left out by the rule rather than by a list: unmaintained builds, and the
	// GraalVM-derived ones, which are a different product.
	for _, unwanted := range []string{"trava", "ojdk_build", "graalvm", "mandrel", "liberica_native"} {
		if _, ok := by[unwanted]; ok {
			t.Errorf("%s should not be offered", unwanted)
		}
	}

	if !sort.SliceIsSorted(vendors, func(a, b int) bool { return vendors[a].Name < vendors[b].Name }) {
		t.Error("the dropdown is not in name order")
	}
}

func TestReadVersions(t *testing.T) {
	versions, err := readVersions(answer(t, "major_versions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) == 0 {
		t.Fatal("no versions parsed")
	}
	for index, version := range versions {
		if version.Major < Minimum {
			t.Errorf("Java %d is older than the loader can run on", version.Major)
		}
		if index > 0 && versions[index-1].Major <= version.Major {
			t.Errorf("wanted newest first, got %d after %d", version.Major, versions[index-1].Major)
		}
	}
	for _, version := range versions {
		if version.Major == 25 && !version.LTS {
			t.Error("25 is an LTS release and should say so")
		}
	}
}

// The default the page opens on. Oracle's Windows x64 build of 25 answers this
// query with a package that is directly downloadable -- several distributions
// are listed and then want a licence click instead, and this is the check that
// says which kind Oracle is.
func TestReadPackages(t *testing.T) {
	found, err := readPackages(answer(t, "packages.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("wanted one package, got %d", len(found))
	}
	pkg := found[0]
	if pkg.ID == "" || pkg.Vendor != "oracle" || pkg.Version != "25.0.4" {
		t.Errorf("parsed as %+v", pkg)
	}
	if pkg.Filename != "jdk-25.0.4_windows-x64_bin.zip" || pkg.Size != 215811018 {
		t.Errorf("parsed as %+v", pkg)
	}
}

// directly_downloadable is a query parameter, and a filter applied by somebody
// else is a filter that can come back wrong.  Asked for the gated ones, the
// same answer must yield none.
func TestReadPackagesDoesNotTrustTheQuery(t *testing.T) {
	found, err := readPackages(answer(t, "packages.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("wanted no gated packages, got %d", len(found))
	}
}

func TestReadLink(t *testing.T) {
	link, digest, err := readLink(answer(t, "package_info.json"))
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != "https://download.oracle.com/java/25/archive/jdk-25.0.4_windows-x64_bin.zip" {
		t.Errorf("download address parsed as %q", link.URL)
	}
	if link.Kind != "sha256" {
		t.Errorf("checksum type parsed as %q", link.Kind)
	}
	// Oracle publishes the digest as a file beside the archive rather than as a
	// field, so an empty checksum with a checksum_uri is the normal case and
	// not a reason to skip verification.
	if link.Checksum != "" {
		t.Errorf("wanted the checksum field empty, got %q", link.Checksum)
	}
	if digest == "" {
		t.Error("wanted a checksum_uri to fetch the digest from")
	}
}

func TestReadRefusesRubbish(t *testing.T) {
	if _, err := readVendors([]byte("<html>404</html>")); err == nil {
		t.Error("wanted an error for a non-JSON answer")
	}
	if _, err := readVersions([]byte(`{"result":[]}`)); err == nil {
		t.Error("wanted an error for an empty version list")
	}
	if _, _, err := readLink([]byte(`{"result":[]}`)); err == nil {
		t.Error("wanted an error when there is no download address")
	}
}
