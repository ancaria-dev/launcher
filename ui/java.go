package ui

// The Java panel.  The page never talks to the network itself: it asks Go to
// start something and then polls, because a WebView2 binding runs on the thread
// that draws the window and anything slow in one freezes the page it is
// supposed to be reporting progress to.
//
// These shapes are the page's, not the fetcher's, for the same reason ModRow is
// not mods.Mod: what a player is shown is allowed to change without the code
// that finds a JDK changing with it.

// JavaRow is the JDK the loader will actually be started with.  Usable false is
// the one condition on this page that stops mods from loading at all, so it is
// drawn in the red --bad rather than the amber a build mismatch gets.
type JavaRow struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Source  string `json:"source"`
	Usable  bool   `json:"usable"`
	// Minimum is the oldest release the loader runs on, so the page can say
	// which number it wanted without knowing it.
	Minimum int `json:"minimum"`
}

// JavaVendor is one distribution in the dropdown.  Site is where somebody has
// to go when a vendor will not hand the file over without a browser.
type JavaVendor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Site string `json:"site"`
}

// JavaVersion is one feature release.
type JavaVersion struct {
	Major int  `json:"major"`
	LTS   bool `json:"lts"`
}

// JavaCatalog fills the two dropdowns.  Neither list is written down anywhere:
// both come from the index, so a distribution published next year is in the
// dropdown without a release of ours.
type JavaCatalog struct {
	Vendors  []JavaVendor  `json:"vendors"`
	Versions []JavaVersion `json:"versions"`
	Loaded   bool          `json:"loaded"`
	Error    string        `json:"error"`
}

// JavaProgress is the bar.  Total zero means a stage with no measurable length,
// which the page draws as an indeterminate sweep rather than as 0%.
type JavaProgress struct {
	Stage string `json:"stage"`
	Note  string `json:"note"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Error string `json:"error"`
}

// JavaView is one poll.
type JavaView struct {
	Java     JavaRow      `json:"java"`
	Catalog  JavaCatalog  `json:"catalog"`
	Progress JavaProgress `json:"progress"`
	Busy     bool         `json:"busy"`
	Vendor   string       `json:"vendor"`
	Version  int          `json:"version"`
}

// Java is how the page reaches the fetcher.  Load and Get return at once and
// the page follows them through View.
type Java struct {
	View func() JavaView
	Load func()
	Get  func(vendor string, version int)
}
