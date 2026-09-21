package ui

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	shell32      = syscall.NewLazyDLL("shell32.dll")
	shellExecute = shell32.NewProc("ShellExecuteW")
)

const swShowNormal = 1

// Open hands a URL to whatever the player browses with.
//
// The page cannot do this itself.  A link in the view navigates the view, and
// the window is the launcher, so following one would replace the launcher with
// a web page and leave no way back: there is no address bar and no Back
// button in it.
//
// Only http and https.  ShellExecute runs whatever a scheme is registered to,
// so an unfiltered one is a way to start a program by naming it, and the
// strings that reach here have come off the network.
func Open(url string) {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return
	}
	verb, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return
	}
	target, err := syscall.UTF16PtrFromString(url)
	if err != nil {
		return
	}
	_, _, _ = shellExecute.Call(0, uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(target)), 0, 0, swShowNormal)
}
