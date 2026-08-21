// Package game starts a session and waits for it to end.
//
// Two processes, and the order matters: the host has to be waiting before the
// game exists, because it attaches to the game as soon as it appears and a
// world can finish loading before a late attach ever happens.
package game

import (
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Session is a running host plus a running game.
type Session struct {
	host *exec.Cmd
	game *exec.Cmd
}

// Start launches the host, then the game.
//
// enabled is the mod ids the player ticked; offHooks the agent sites to leave
// uninstalled; flags go to the game untouched.  output receives everything
// the host prints, which is where the console in debug mode gets its content --
// and is discarded otherwise.
//
// javaExe is the JDK the launcher settled on.  It is passed through rather than
// left to the host, whose own default is JAVA_HOME and then whatever `java`
// means on PATH: the whole point of the launcher fetching a JDK is that the
// loader then runs on that one and not on something else the machine happened
// to have.  Empty means "you decide", which is what happens when there is no
// JDK at all and the player pressed Play anyway.
func Start(dir, gameDir, javaExe string, enabled, offHooks, flags []string, output io.Writer) (*Session, error) {
	// Before the host, not after: starting the host and then discovering there
	// is nothing to attach to leaves a process behind for no reason.
	exe := Find(gameDir)
	if exe == "" {
		return nil, errors.New("No game was found in " + gameDir + " (" + Missing() + ")")
	}

	args := []string{"--enable", strings.Join(enabled, ",")}
	// Only when there is something to name: --no-hook with an empty value
	// would reach the agent as a list containing one empty name.
	if len(offHooks) > 0 {
		args = append(args, "--no-hook", strings.Join(offHooks, ","))
	}
	if javaExe != "" {
		args = append(args, "--java", javaExe)
	}
	host := exec.Command(filepath.Join(dir, "protocol.exe"), args...)
	host.Dir = dir
	host.Stdout = output
	host.Stderr = output
	// The host's own console window would flash up over the launcher; its
	// output is already being read here.
	host.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := host.Start(); err != nil {
		return nil, err
	}

	game := exec.Command(exe, flags...)
	game.Dir = gameDir
	if err := game.Start(); err != nil {
		_ = host.Process.Kill()
		return nil, err
	}
	return &Session{host: host, game: game}, nil
}

// Wait blocks until the game exits, then stops the host.
//
// The host is meant to outlive one game -- it loops, waiting for the process to
// come back -- which is exactly wrong under a launcher: the player closed the
// game to get the launcher back, so the session ends with it.
func (s *Session) Wait() {
	_ = s.game.Wait()
	// A moment for the host to notice the detach and unload cleanly; killing it
	// mid-callback is how the host itself used to fault.
	time.Sleep(500 * time.Millisecond)
	if s.host.Process != nil {
		_ = s.host.Process.Kill()
	}
	_ = s.host.Wait()
}
