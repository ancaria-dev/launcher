package ui

import "syscall"

// Windows scales an application that has not said it understands scaling by
// stretching its output as a bitmap, which is why the launcher looked soft on a
// display running at anything but 100%. go-webview2 says nothing about DPI and
// hands raw pixels to CreateWindowExW, so both halves are ours: declare
// awareness before any window exists, then ask for a window measured in this
// monitor's pixels rather than in 96-dpi ones.

var shcore = syscall.NewLazyDLL("shcore.dll")

const (
	// (DPI_AWARENESS_CONTEXT)-4: per-monitor v2, the one that also scales the
	// non-client area and updates when a window is dragged to another screen.
	perMonitorV2 = ^uintptr(3)
	perMonitor   = 2  // PROCESS_PER_MONITOR_DPI_AWARE
	logPixelsX   = 88 // GetDeviceCaps index
	baseDPI      = 96
)

// awareOfDPI opts the process in, newest call first.  Each of these arrived in
// a different Windows, and the older ones are worse but still sharp: v2 in
// 1703, the shcore call in 8.1, and the blunt system-wide one before that.
// Nothing here is fatal.  A launcher that cannot ask is a launcher that looks
// like it did yesterday.
func awareOfDPI() {
	// BOOL: non-zero is success.
	if ret, ok := call(user32.NewProc("SetProcessDpiAwarenessContext"), perMonitorV2); ok && ret != 0 {
		return
	}
	// HRESULT: S_OK is ZERO.  Reading this one the same way as the others is
	// how a working call gets treated as a failure and the blunt fallback runs
	// on top of it.
	if ret, ok := call(shcore.NewProc("SetProcessDpiAwareness"), perMonitor); ok && ret == 0 {
		return
	}
	call(user32.NewProc("SetProcessDPIAware"))
}

// scale converts a size written in ordinary 96-dpi pixels into what the window
// needs to be to look that size.  Without it, declaring awareness would make
// the window shrink instead of sharpen.
func scale(size uint) uint {
	return size * dpi() / baseDPI
}

func dpi() uint {
	if proc := user32.NewProc("GetDpiForSystem"); proc.Find() == nil {
		if value, _, _ := proc.Call(); value > 0 {
			return uint(value)
		}
	}
	// Before Windows 10 1607: read it off the desktop device context.
	getDC := user32.NewProc("GetDC")
	releaseDC := user32.NewProc("ReleaseDC")
	caps := syscall.NewLazyDLL("gdi32.dll").NewProc("GetDeviceCaps")
	if getDC.Find() != nil || caps.Find() != nil {
		return baseDPI
	}
	screen, _, _ := getDC.Call(0)
	if screen == 0 {
		return baseDPI
	}
	defer releaseDC.Call(0, screen)
	value, _, _ := caps.Call(screen, logPixelsX)
	if value == 0 {
		return baseDPI
	}
	return uint(value)
}

// call runs a proc that may not exist on this Windows, returning its value and
// whether it was there at all.  What counts as success differs per call, so
// that judgement stays with the caller.  LazyProc.Call panics on a missing
// export, so Find comes first.
func call(proc *syscall.LazyProc, args ...uintptr) (uintptr, bool) {
	if proc.Find() != nil {
		return 0, false
	}
	ret, _, _ := proc.Call(args...)
	return ret, true
}
