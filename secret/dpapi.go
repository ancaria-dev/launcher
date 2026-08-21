// Package secret keeps a token in a settings file without keeping it readable.
//
// A private mod registry is reached with an access token, and the settings this
// launcher writes live in the game folder next to everything else. A token in
// plain text there is a token in every backup of that folder, in every "send me
// your Sacred directory" thread, and readable by any mod the loader starts.
//
// Windows already has the right thing for this. DPAPI encrypts with a key
// derived from the logged-in user's credentials, so the sealed value is useless
// on another account and on another machine, and nothing has to be stored to
// unlock it. It is not a vault -- code running as that same user can unseal it
// too -- but the failure it prevents is the one that actually happens: a
// credential travelling somewhere it was never meant to go.
package secret

import (
	"encoding/base64"
	"errors"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

// Marker on a sealed value.  A settings file somebody edited by hand can say
// `"token": "ghp_..."` and it still works: the prefix is how Open tells the two
// apart, and the next Save writes the sealed form back.
const marker = "dpapi:"

// Bound to this launcher, so a blob sealed by something else on the same
// account is not a blob this will open.
var salt = []byte("ancaria-launcher/registry-token")

var (
	crypt32   = syscall.NewLazyDLL("crypt32.dll")
	kernel32  = syscall.NewLazyDLL("kernel32.dll")
	protect   = crypt32.NewProc("CryptProtectData")
	unprotect = crypt32.NewProc("CryptUnprotectData")
	localFree = kernel32.NewProc("LocalFree")
)

// CRYPTPROTECT_UI_FORBIDDEN: this runs behind a window that is drawing a list,
// and a system prompt appearing from underneath it would be a mystery.
const uiForbidden = 0x1

type blob struct {
	size uint32
	data *byte
}

// Seal turns a token into something safe to write down.  An empty value stays
// empty: there is nothing to protect and a blob of ciphertext for the empty
// string only makes the file harder to read.
func Seal(value string) string {
	if value == "" {
		return ""
	}
	sealed, err := call(protect, []byte(value))
	if err != nil {
		// Refusing to save the setting would be worse than saving it plainly,
		// and Open reads both. The UI says which of the two happened.
		return value
	}
	return marker + base64.StdEncoding.EncodeToString(sealed)
}

// Open reverses Seal.  A value with no marker is returned as it was, because a
// person may have typed it into the file themselves.
func Open(value string) string {
	if !strings.HasPrefix(value, marker) {
		return value
	}
	sealed, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, marker))
	if err != nil {
		return ""
	}
	plain, err := call(unprotect, sealed)
	if err != nil {
		// Another account, another machine, or a re-installed Windows. The
		// token is gone rather than wrong, and the player is asked for it again.
		return ""
	}
	return string(plain)
}

// Works reports whether sealing does anything on this machine, so the page can
// say what it does with a token rather than guessing.
func Works() bool {
	sealed, err := call(protect, []byte("probe"))
	if err != nil {
		return false
	}
	plain, err := call(unprotect, sealed)
	return err == nil && string(plain) == "probe"
}

func call(proc *syscall.LazyProc, in []byte) ([]byte, error) {
	if len(in) == 0 {
		return nil, errors.New("nothing to protect")
	}
	input := blob{size: uint32(len(in)), data: &in[0]}
	entropy := blob{size: uint32(len(salt)), data: &salt[0]}
	var out blob
	ok, _, err := proc.Call(
		uintptr(unsafe.Pointer(&input)),
		0, // no description
		uintptr(unsafe.Pointer(&entropy)),
		0, // reserved
		0, // no prompt
		uiForbidden,
		uintptr(unsafe.Pointer(&out)),
	)
	// The DLL holds no reference to either buffer after it returns, and until it
	// does the collector must not move them.
	runtime.KeepAlive(in)
	runtime.KeepAlive(salt)
	if ok == 0 {
		return nil, err
	}
	defer localFree.Call(uintptr(unsafe.Pointer(out.data)))
	return append([]byte(nil), unsafe.Slice(out.data, out.size)...), nil
}
