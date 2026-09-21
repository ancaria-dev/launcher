package ui

// The updater panel.  Same arrangement as the Java one: the page never talks
// to the network, it asks Go to start something and polls for what to draw.

// UpdateRelease is the release on offer.  Page is where somebody goes when the
// launcher could not fetch it for them, which is the one case where a player
// has to finish the job by hand.
type UpdateRelease struct {
	Version string `json:"version"`
	Size    int64  `json:"size"`
	Page    string `json:"page"`
}

// UpdateProgress is the bar.  Total zero draws an indeterminate sweep rather
// than 0%, the same as the JDK's.
type UpdateProgress struct {
	Stage string `json:"stage"`
	Note  string `json:"note"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Error string `json:"error"`
}

// UpdateView is one poll.  Found false is the ordinary state and draws
// nothing at all: an up-to-date launcher, and a launcher that could not reach
// GitHub, look the same to a player on purpose.
type UpdateView struct {
	Current  string         `json:"current"`
	Release  UpdateRelease  `json:"release"`
	Found    bool           `json:"found"`
	Progress UpdateProgress `json:"progress"`
	Busy     bool           `json:"busy"`
	Staged   bool           `json:"staged"`
}

// Update is how the page reaches the updater.
//
// Apply is the one binding on this page that does not return to a window:
// on success the launcher has already started its replacement and this one is
// closing.  Open hands a URL to the browser, for the link under a failure.
type Update struct {
	View func() UpdateView
	Get  func()
	// Apply calls done once the replacement is running.  done closes this
	// window, and nothing after it is drawn.
	Apply func(done func())
	Open  func(url string)
}
