// The foojay Disco API: the discovery service javaalmanac.io is built on, and
// the reason neither the vendor list nor the version list is written down here.
// A distribution published next year is in the launcher's dropdown without a
// release of ours.
//
// Every response is parsed by a `read…` function that takes bytes, so the tests
// can be run against answers the live service actually gave rather than against
// ones somebody invented -- a fixture written from the struct only ever proves
// the struct matches the fixture.
package java

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// API is the base every request is built on.  v3.0 is what foojay publishes and
// what javaalmanac.io reads.
const API = "https://api.foojay.io/disco/v3.0/"

// Defaults are what the page opens on.  Oracle 25 is directly downloadable for
// Windows x64 -- checked against the live service, because several
// distributions are listed and then hand you a licence click instead of a file,
// and a default nobody can download is worse than a different default.
const (
	DefaultVendor  = "oracle"
	DefaultVersion = 25
)

// Vendor is one distribution, as the player picks it.
type Vendor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Site string `json:"site"`
}

// Version is one feature release.  LTS is shown because it is the only thing
// telling them apart that a player has a reason to care about.
type Version struct {
	Major int  `json:"major"`
	LTS   bool `json:"lts"`
}

// Pkg is one downloadable archive.
type Pkg struct {
	ID       string `json:"id"`
	Vendor   string `json:"vendor"`
	Version  string `json:"version"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

// Link is where a package actually is, and what it should hash to.
type Link struct {
	URL      string
	Checksum string
	Kind     string
}

// index is short-tempered on purpose: none of these answers is large, and a
// launcher that hangs on a dead network is a launcher nobody can close.  The
// archive itself is fetched by get.go with no deadline at all.
var index = &http.Client{Timeout: 30 * time.Second}

// Vendors lists the distributions worth offering: maintained, available, and
// builds of OpenJDK.
//
// A rule rather than a list, so nothing has to be edited here when a new
// distribution appears.  What the rule leaves out is the GraalVM-derived
// entries -- a different product with a different reason to exist, where the
// loader wants a plain JDK.
func Vendors() ([]Vendor, error) {
	data, err := disco("distributions?include_versions=false&include_synonyms=false")
	if err != nil {
		return nil, err
	}
	return readVendors(data)
}

func readVendors(data []byte) ([]Vendor, error) {
	var body struct {
		Result []struct {
			Name       string `json:"name"`
			Parameter  string `json:"api_parameter"`
			Maintained bool   `json:"maintained"`
			Available  bool   `json:"available"`
			OpenJDK    bool   `json:"build_of_openjdk"`
			Site       string `json:"official_uri"`
		} `json:"result"`
	}
	if err := decode(data, &body); err != nil {
		return nil, err
	}
	var found []Vendor
	for _, entry := range body.Result {
		if !entry.Maintained || !entry.Available || !entry.OpenJDK {
			continue
		}
		found = append(found, Vendor{ID: entry.Parameter, Name: entry.Name, Site: entry.Site})
	}
	if len(found) == 0 {
		return nil, errors.New("The Java catalog contains no supported distributions")
	}
	sort.Slice(found, func(a, b int) bool { return found[a].Name < found[b].Name })
	return found, nil
}

// Versions lists the released feature versions the loader can run on, newest
// first.  Anything below Minimum is left out rather than offered and refused:
// a choice that cannot work is not a choice.
func Versions() ([]Version, error) {
	data, err := disco("major_versions?ga=true&maintained=true&include_versions=false")
	if err != nil {
		return nil, err
	}
	return readVersions(data)
}

func readVersions(data []byte) ([]Version, error) {
	var body struct {
		Result []struct {
			Major   int    `json:"major_version"`
			Support string `json:"term_of_support"`
		} `json:"result"`
	}
	if err := decode(data, &body); err != nil {
		return nil, err
	}
	var found []Version
	for _, entry := range body.Result {
		if entry.Major < Minimum {
			continue
		}
		found = append(found, Version{
			Major: entry.Major,
			LTS:   strings.EqualFold(entry.Support, "lts"),
		})
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("The Java catalog contains no release of Java %d or newer", Minimum)
	}
	sort.Slice(found, func(a, b int) bool { return found[a].Major > found[b].Major })
	return found, nil
}

// Resolve asks for the newest build of one distribution and one feature version
// that this machine can actually download.
//
// The second query is deliberate.  When the filtered one comes back empty the
// same question is asked without `directly_downloadable`, because those are two
// different pieces of news: "this vendor has no Windows build of that release"
// and "it has one and will not hand it over without a browser".  A player can
// act on the second.
func Resolve(vendor string, major int) (Pkg, error) {
	query := url.Values{
		"distribution":     {vendor},
		"version":          {strconv.Itoa(major)},
		"operating_system": {"windows"},
		"architecture":     {arch()},
		"archive_type":     {"zip"},
		"package_type":     {"jdk"},
		"javafx_bundled":   {"false"},
		"latest":           {"available"},
	}

	found, err := packages(query, true)
	if err != nil {
		return Pkg{}, err
	}
	if len(found) > 0 {
		return found[0], nil
	}

	if gated, err := packages(query, false); err == nil && len(gated) > 0 {
		return Pkg{}, fmt.Errorf("%s provides Java %d for Windows, but requires you "+
			"to download it from its website", vendor, major)
	}
	return Pkg{}, fmt.Errorf("%s does not provide a Windows %s build of Java %d", vendor, arch(), major)
}

func packages(query url.Values, direct bool) ([]Pkg, error) {
	query.Set("directly_downloadable", strconv.FormatBool(direct))
	data, err := disco("packages?" + query.Encode())
	if err != nil {
		return nil, err
	}
	return readPackages(data, direct)
}

func readPackages(data []byte, direct bool) ([]Pkg, error) {
	var body struct {
		Result []struct {
			ID       string `json:"id"`
			Vendor   string `json:"distribution"`
			Version  string `json:"java_version"`
			Filename string `json:"filename"`
			Size     int64  `json:"size"`
			Direct   bool   `json:"directly_downloadable"`
		} `json:"result"`
	}
	if err := decode(data, &body); err != nil {
		return nil, err
	}
	var found []Pkg
	for _, entry := range body.Result {
		// The filter is a query parameter, and trusting it is how a licence-gated
		// build ends up being downloaded as an HTML page.
		if entry.Direct != direct {
			continue
		}
		found = append(found, Pkg{
			ID:       entry.ID,
			Vendor:   entry.Vendor,
			Version:  entry.Version,
			Filename: entry.Filename,
			Size:     entry.Size,
		})
	}
	return found, nil
}

// Locate turns a package id into a download.
//
// The checksum field is often empty with `checksum_uri` pointing at a `.sha256`
// file beside the archive -- Oracle publishes it that way -- so the digest is
// one more small fetch rather than a field that is simply missing.
func Locate(id string) (Link, error) {
	data, err := disco("ids/" + url.PathEscape(id))
	if err != nil {
		return Link{}, err
	}
	link, digestURI, err := readLink(data)
	if err != nil {
		return Link{}, err
	}
	if link.Checksum == "" && digestURI != "" {
		link.Checksum = digestFile(digestURI)
	}
	return link, nil
}

func readLink(data []byte) (Link, string, error) {
	var body struct {
		Result []struct {
			URL         string `json:"direct_download_uri"`
			Checksum    string `json:"checksum"`
			ChecksumURI string `json:"checksum_uri"`
			Kind        string `json:"checksum_type"`
		} `json:"result"`
	}
	if err := decode(data, &body); err != nil {
		return Link{}, "", err
	}
	if len(body.Result) == 0 || body.Result[0].URL == "" {
		return Link{}, "", errors.New("The Java catalog has no download URL for this build")
	}
	entry := body.Result[0]
	return Link{
		URL:      entry.URL,
		Checksum: strings.TrimSpace(entry.Checksum),
		Kind:     strings.ToLower(strings.TrimSpace(entry.Kind)),
	}, entry.ChecksumURI, nil
}

// digestFile reads a checksum published as its own file.  The content is either
// a bare digest or the `<digest>  <filename>` shape sha256sum writes, so the
// first field is the answer either way.
func digestFile(address string) string {
	response, err := index.Get(address)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<10))
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func disco(path string) ([]byte, error) {
	response, err := index.Get(API + path)
	if err != nil {
		return nil, fmt.Errorf("Could not reach the Java catalog: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("The Java catalog returned %s", response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, 8<<20))
}

func decode(data []byte, into any) error {
	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("The Java catalog returned unreadable data: %w", err)
	}
	return nil
}

// arch is what foojay calls this machine.  The game is 32-bit and the JVM is
// not inside it -- the host starts Java as its own process -- so the JDK is
// simply the machine's own.
func arch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "aarch64"
	default:
		return "x64"
	}
}
