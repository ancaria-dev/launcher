// Package hooks lists the sites the agent will attach to, asked of the host.
//
// Every site is installed through one helper that takes its own name first and
// its address second, `hook("goldDelta", RVA.goldDelta, ...)`, and the host
// already accepts `--no-hook goldDelta`. So the list a player sees is read out
// of the agent rather than kept in a table beside it: a table would be one more
// thing to forget, and a hook missing from the list is a hook nobody can turn
// off.
//
// The agent lives inside `protocol.exe` now, minified, so the reading is done
// where the agent is. `protocol.exe --hooks` prints the manifest its build
// wrote from the sources before minifying them, and this asks the copy in the
// game folder, because that is the copy that will be loaded.
package hooks

import (
	"context"
	"encoding/json"
	"os/exec"
	"syscall"
	"time"
)

// Group is one agent module and the sites it installs.
type Group struct {
	Module string   `json:"module"`
	Hooks  []string `json:"hooks"`
}

// Ask runs the host for its manifest. An empty list is the honest answer to
// every failure here: an older protocol.exe that does not know the flag, one
// that cannot be started, anything. The page then shows no hooks rather than a
// list somebody could switch off in the belief it meant something.
func Ask(exe string) []Group {
	// The window is not drawn yet and the host answers immediately, but a
	// launcher that never opens because a child process is stuck is worse than
	// a launcher with an empty hooks block.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe, "--hooks")
	// No console window: this runs while the player is looking at the desktop.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parse(out)
}

func parse(data []byte) []Group {
	var groups []Group
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil
	}
	// A module with no sites has no row, and neither has a row with no module:
	// both would draw an empty block with nothing in it to switch.
	kept := make([]Group, 0, len(groups))
	for _, group := range groups {
		if group.Module == "" || len(group.Hooks) == 0 {
			continue
		}
		kept = append(kept, group)
	}
	return kept
}
