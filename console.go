package main

import (
	"os"
	"syscall"
	"unsafe"
)

// The console the launcher opens for itself.  A windowsgui binary has none, so
// there is nothing to print to until AllocConsole, and the handles still
// point at nothing afterwards, which is the part that is easy to forget.

var kernel32 = syscall.NewLazyDLL("kernel32.dll")

const (
	// A console started this way inherits QuickEdit, and QuickEdit means a
	// stray click puts the window into selection mode: the host keeps running,
	// its next write blocks on a full buffer, and nothing appears until Enter
	// or Escape is pressed.  It reads exactly like a hang, and it is the reason
	// the log seemed to need a keypress to move.
	quickEdit = 0x0040
	// And this one has to go with it.  While QuickEdit is on, the console
	// handles the mouse itself.  Turn QuickEdit off and leave this set, and the
	// wheel is delivered to whatever is running instead, so the window stops
	// scrolling and looks frozen in a second, quieter way.  Clearing both hands
	// the mouse back to the console for scrolling, without the selection that
	// stops the output.
	mouseInput = 0x0010
	// Neither bit changes anything unless this one is set with them.
	extendedFlags = 0x0080
)

var consoleOpen bool

func openConsole() {
	if consoleOpen {
		return
	}
	if ret, _, _ := kernel32.NewProc("AllocConsole").Call(); ret == 0 {
		return
	}
	consoleOpen = true
	for _, target := range []**os.File{&os.Stdout, &os.Stderr} {
		handle, err := syscall.Open("CONOUT$", syscall.O_RDWR, 0)
		if err != nil {
			continue
		}
		*target = os.NewFile(uintptr(handle), "CONOUT$")
	}
	dontPauseOnClick()
}

func dontPauseOnClick() {
	input, err := syscall.Open("CONIN$", syscall.O_RDWR, 0)
	if err != nil {
		return
	}
	defer syscall.Close(input)

	get := kernel32.NewProc("GetConsoleMode")
	set := kernel32.NewProc("SetConsoleMode")
	var mode uint32
	if ret, _, _ := get.Call(uintptr(input), uintptr(unsafe.Pointer(&mode))); ret == 0 {
		return
	}
	_, _, _ = set.Call(uintptr(input),
		uintptr((mode&^(quickEdit|mouseInput))|extendedFlags))
}
