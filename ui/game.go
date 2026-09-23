package ui

// The game build.  Startup reads it once for the strip under the header, but
// the launcher can also fetch the build the loader was made for, so the page
// polls it the way it polls the Java panel while that is happening.

// GameProgress is the bar in the pureHD dialog.  Total zero is drawn as an
// indeterminate sweep, the same as the JDK's.
type GameProgress struct {
	Stage string `json:"stage"`
	Note  string `json:"note"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Error string `json:"error"`
}

// GameView is one poll.  Abortable goes false once the files are being moved
// into the game folder, and the page hides Abort with it.
type GameView struct {
	Game      GameRow      `json:"game"`
	Progress  GameProgress `json:"progress"`
	Busy      bool         `json:"busy"`
	Abortable bool         `json:"abortable"`
}

// Game is how the page reaches the pureHD fetcher.  Get and Abort return at
// once and the page follows them through View.
type Game struct {
	View  func() GameView
	Get   func()
	Abort func()
}
