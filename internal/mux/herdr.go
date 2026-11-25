// Package mux finds agents that run inside a terminal multiplexer, where no
// window owns them, and brings them back: herdr for now.
package mux

import (
	"encoding/json"
	"os/exec"
	"slices"
	"strings"

	"github.com/viniemm/topdown/internal/proc"
	"github.com/viniemm/topdown/internal/util"
)

// Pane is where an agent runs inside herdr.
type Pane struct {
	Session string // herdr session ("default" unless named)
	ID      string // pane id, e.g. "wB:p1"
}

// Find is the herdr pane running pid, if herdr's server is one of its ancestors.
func Find(pid int) (Pane, bool) {
	if util.Which("herdr") == "" || !underServer(pid) {
		return Pane{}, false
	}
	up := proc.Ancestors(pid)
	for _, session := range sessions() {
		var list struct {
			Result struct {
				Panes []struct {
					ID string `json:"pane_id"`
				} `json:"panes"`
			} `json:"result"`
		}
		if !call(session, &list, "pane", "list") {
			continue
		}
		for _, p := range list.Result.Panes {
			var info struct {
				Result struct {
					Info struct {
						Shell int `json:"shell_pid"`
						Group int `json:"foreground_process_group_id"`
					} `json:"process_info"`
				} `json:"result"`
			}
			if !call(session, &info, "pane", "process-info", "--pane", p.ID) {
				continue
			}
			if i := info.Result.Info; slices.Contains(up, i.Shell) || slices.Contains(up, i.Group) {
				return Pane{Session: session, ID: p.ID}, true
			}
		}
	}
	return Pane{}, false
}

func underServer(pid int) bool {
	for _, p := range proc.Ancestors(pid) {
		if argv := proc.Argv(p); len(argv) > 1 && strings.HasSuffix(argv[0], "herdr") && argv[1] == "server" {
			return true
		}
	}
	return false
}

// sessions are the running herdr sessions, default first.
func sessions() []string {
	out, err := exec.Command("herdr", "session", "list").Output()
	if err != nil {
		return []string{"default"}
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n")[1:] { // skip the header
		if f := strings.Fields(line); len(f) >= 2 && f[1] == "running" {
			names = append(names, f[0])
		}
	}
	slices.SortStableFunc(names, func(a, b string) int {
		if a == "default" {
			return -1
		}
		if b == "default" {
			return 1
		}
		return 0
	})
	return names
}

func sessionArgs(session string, args ...string) []string {
	if session == "" || session == "default" {
		return args
	}
	return append([]string{"--session", session}, args...)
}

func call(session string, into any, args ...string) bool {
	out, err := exec.Command("herdr", sessionArgs(session, args...)...).Output()
	return err == nil && json.Unmarshal(out, into) == nil
}

// Select makes the pane the focused one in its session (workspace, tab and pane).
func (p Pane) Select() {
	exec.Command("herdr", sessionArgs(p.Session, "agent", "focus", p.ID)...).Run()
}

// Clients are the pids of herdr clients attached to the pane's session.
func (p Pane) Clients() []int {
	var out []int
	for _, pid := range proc.Named("herdr") {
		argv := proc.Argv(pid)
		if len(argv) > 1 && argv[1] == "server" {
			continue
		}
		if clientSession(argv) == p.Session {
			out = append(out, pid)
		}
	}
	return out
}

// clientSession is the session a herdr client command line attaches to
// (`herdr`, `herdr --session x`, `herdr session attach x`).
func clientSession(argv []string) string {
	for i, a := range argv {
		if a == "--session" && i+1 < len(argv) {
			return argv[i+1]
		}
		if a == "attach" && i > 0 && argv[i-1] == "session" && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	return "default"
}

// AttachArgv attaches a new client to the pane's session.
func (p Pane) AttachArgv() []string {
	if p.Session == "" || p.Session == "default" {
		return []string{"herdr"}
	}
	return []string{"herdr", "session", "attach", p.Session}
}
