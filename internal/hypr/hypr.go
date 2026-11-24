// Package hypr finds and focuses the Hyprland window an agent process runs in.
package hypr

import (
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/viniemm/topdown/internal/util"
)

func Present() bool {
	return os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" && util.Which("hyprctl") != ""
}

// WindowForPID is the window address of the terminal that owns pid, walking up its parents.
func WindowForPID(pid int) string {
	if !Present() {
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
	for hops := 0; pid > 1 && hops < 64; hops++ {
		if addr, ok := byPID[pid]; ok {
			return addr
		}
		stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if err != nil {
			return ""
		}
		// "pid (comm) state ppid …": comm may contain spaces, so split after the last ')'.
		s := string(stat)
		fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
		if len(fields) < 2 {
			return ""
		}
		pid, _ = strconv.Atoi(fields[1])
	}
	return ""
}

func Focus(address string) {
	exec.Command("hyprctl", "dispatch", `hl.dsp.focus({ window = "address:`+address+`" })`).Run()
}
