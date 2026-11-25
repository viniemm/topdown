// Package hypr finds and focuses the Hyprland window an agent process runs in.
package hypr

import (
	"encoding/json"
	"os"
	"os/exec"

	"github.com/viniemm/topdown/internal/proc"
	"github.com/viniemm/topdown/internal/util"
)

func present() bool {
	return os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" && util.Which("hyprctl") != ""
}

// WindowForPID is the window address of the terminal that owns pid, walking up its parents.
func WindowForPID(pid int) string {
	if !present() {
		return ""
	}
	out, err := exec.Command("hyprctl", "clients", "-j").Output()
	if err != nil {
		return ""
	}
	var clients []struct {
		PID     int    `json:"pid"`
		Address string `json:"address"`
	}
	if json.Unmarshal(out, &clients) != nil {
		return ""
	}
	byPID := map[int]string{}
	for _, c := range clients {
		byPID[c.PID] = c.Address
	}
	for _, p := range proc.Ancestors(pid) {
		if addr, ok := byPID[p]; ok {
			return addr
		}
	}
	return ""
}

func Focus(address string) {
	exec.Command("hyprctl", "dispatch", `hl.dsp.focus({ window = "address:`+address+`" })`).Run()
}
