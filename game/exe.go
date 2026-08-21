package game

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

// Names are the game executables, in the order they are looked for.  Most
// installs are the community HD wrapper the addresses belong to, but the stock
// game is Sacred.exe and a few copies were renamed, so all three are tried
// rather than one being assumed.
//
// This is the only list in the repository.  Everything that has to find the
// game -- the folder check at startup, the process the launcher spawns --
// comes through here.
var Names = []string{"pureHD.exe", "Sacred.exe", "Game.exe"}

// Find returns the path to the game executable inside dir, or an empty string
// when the folder holds none of them.
//
// The comparison ignores case.  Windows does not distinguish Sacred.exe from
// SACRED.EXE, so an install that differs in case is the same install, and
// matching literally would reject a folder that works perfectly well.  The
// name as it is actually spelled on disk is what comes back.
func Find(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, wanted := range Names {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if strings.EqualFold(entry.Name(), wanted) {
				return filepath.Join(dir, entry.Name())
			}
		}
	}
	return ""
}

// Missing is the sentence to show somebody whose folder holds no game.
func Missing() string {
	return "checked for " + strings.Join(Names, ", ")
}

// Expected is the build every address in the loader was found in: the community
// HD wrapper, whose own version resource says 2.0.2.118.  The stock game says
// 2.0.2.28 and its code is somewhere else entirely, so a hook meant for one
// lands in the middle of an unrelated function in the other.
//
// The loader attaches to it anyway rather than refusing -- but the player is
// told first, which is what Build is for.
const Expected = "2.0.2.118"

// Build is which game the launcher found and whether it is the one the loader
// was made for.  A player deciding whether to press Play is the only person who
// can do anything about a mismatch, so it has to be answered before they do.
type Build struct {
	Exe         string
	Version     string
	ExpectedExe string
	Expected    string
	Matches     bool
}

// Describe reads the executable in dir.  The name is not what decides: a copy
// of the wrapper renamed to Game.exe is still the build the addresses came
// from, and a stock Sacred.exe would pass a name check on its second try.  The
// version is what decides, and a version that cannot be read is not a match --
// it is one more binary nobody has confirmed these addresses against.
func Describe(dir string) Build {
	exe := Find(dir)
	if exe == "" {
		return Build{ExpectedExe: Names[0], Expected: Expected}
	}
	found := Version(exe)
	return Build{
		Exe:         filepath.Base(exe),
		Version:     found,
		ExpectedExe: Names[0],
		Expected:    Expected,
		Matches:     found == Expected,
	}
}

// Version is the four numbers in the executable's VS_FIXEDFILEINFO block, or an
// empty string when it carries none.
//
// Deliberately not the FileVersion *string*, which is the one a file's
// Properties dialog shows and the one that is useless here: the wrapper spells
// it "2.28" and the stock game spells it "2.0", both call themselves Sacred,
// and both name Sacred.exe as their original file name.  The numbers are the
// half that tells them apart.
func Version(exe string) string {
	name, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return ""
	}
	infoSize := versionDLL.NewProc("GetFileVersionInfoSizeW")
	readInfo := versionDLL.NewProc("GetFileVersionInfoW")
	query := versionDLL.NewProc("VerQueryValueW")
	for _, proc := range []*syscall.LazyProc{infoSize, readInfo, query} {
		if proc.Find() != nil {
			return ""
		}
	}

	size, _, _ := infoSize.Call(uintptr(unsafe.Pointer(name)), 0)
	if size == 0 {
		return ""
	}
	block := make([]byte, size)
	if ok, _, _ := readInfo.Call(uintptr(unsafe.Pointer(name)), 0, size,
		uintptr(unsafe.Pointer(&block[0]))); ok == 0 {
		return ""
	}

	// The root of the version block, which is where the fixed part lives.
	root, err := syscall.UTF16PtrFromString(rootPath)
	if err != nil {
		return ""
	}
	var fixed *fixedFileInfo
	var length uint32
	ok, _, _ := query.Call(uintptr(unsafe.Pointer(&block[0])),
		uintptr(unsafe.Pointer(root)),
		uintptr(unsafe.Pointer(&fixed)),
		uintptr(unsafe.Pointer(&length)))
	// fixed points into block, which Go is otherwise free to collect while the
	// numbers are being read out of it.
	defer runtime.KeepAlive(block)
	if ok == 0 || fixed == nil || length < uint32(unsafe.Sizeof(*fixed)) ||
		fixed.Signature != fixedSignature {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d",
		fixed.FileVersionMS>>16, fixed.FileVersionMS&0xFFFF,
		fixed.FileVersionLS>>16, fixed.FileVersionLS&0xFFFF)
}

var versionDLL = syscall.NewLazyDLL("version.dll")

// The backslash VerQueryValue takes as "the whole block", and the magic the
// fixed part starts with -- a block that does not begin with it is not one.
const (
	rootPath       = `\`
	fixedSignature = 0xFEEF04BD
)

// VS_FIXEDFILEINFO.  Only the two version pairs are read; the rest is here so
// the struct is the size Windows says it is, which is what the length check
// above compares against.
type fixedFileInfo struct {
	Signature        uint32
	StrucVersion     uint32
	FileVersionMS    uint32
	FileVersionLS    uint32
	ProductVersionMS uint32
	ProductVersionLS uint32
	FileFlagsMask    uint32
	FileFlags        uint32
	FileOS           uint32
	FileType         uint32
	FileSubtype      uint32
	FileDateMS       uint32
	FileDateLS       uint32
}
