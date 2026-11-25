// Package launch builds what to run and hands it over: in place (this process
// execs it, so the terminal becomes the agent) or detached in a new window.
package launch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/viniemm/topdown/internal/agent"
	"github.com/viniemm/topdown/internal/config"
	"github.com/viniemm/topdown/internal/omarchy"
	"github.com/viniemm/topdown/internal/util"
)

type Where int

const (
	Here Where = iota
	Window
)

// Spec is what to run: argv in Cwd (if set) with Env added.
type Spec struct {
	Argv []string
	Cwd  string
	Env  map[string]string
}

func agentSpec(agentID string, args []string, cwd string, where Where) Spec {
	// Launches from $HOME start in the workdir (~/Work, as omarchy-agent does).
	if wd, _ := os.Getwd(); cwd == "" && wd == util.Home() {
		cwd = config.Workdir()
	}
	if omarchy.Present() {
		shim := omarchy.EnsureShim()
		env := map[string]string{
			"TOPDOWN_AGENT": agentID,
			"TOPDOWN_ARGS":  strings.Join(args, "\n"),
			"TOPDOWN_CWD":   cwd,
			"PATH":          shim + ":" + os.Getenv("PATH"),
		}
		if where == Here {
			env["TOPDOWN_INLINE"] = "1"
		}
		return Spec{Argv: []string{"omarchy-agent"}, Env: env}
	}
	bin := agentID
	var launchArgs []string
	if a := agent.Get(agentID); a != nil {
		bin, launchArgs = a.Bin, a.LaunchArgs
	}
	argv := append(append([]string{bin}, launchArgs...), args...)
	if where == Window {
		argv = inTerminal(argv)
	}
	return Spec{Argv: argv, Cwd: cwd}
}

// terminals are tried in order when xdg-terminal-exec is missing: the command
// that opens a window, the window-class and title flags (empty: it has none),
// and what goes between the flags and the command to run.
var terminals = []struct {
	cmd          []string
	class, title string
	run          []string
}{
	{[]string{"foot"}, "--app-id=", "--title=", nil},
	{[]string{"kitty"}, "--class=", "--title=", nil},
	{[]string{"alacritty"}, "--class=", "--title=", []string{"-e"}},
	{[]string{"ghostty"}, "--class=", "--title=", []string{"-e"}},
	{[]string{"wezterm", "start"}, "--class=", "", []string{"--"}},
	{[]string{"ptyxis"}, "", "", []string{"--"}},
	{[]string{"gnome-terminal"}, "--class=", "--title=", []string{"--"}},
	{[]string{"konsole"}, "", "", []string{"-e"}},
	{[]string{"xfce4-terminal"}, "", "--title=", []string{"-x"}},
	{[]string{"xterm"}, "", "", []string{"-e"}},
}

// Terminal wraps argv to run in a new terminal window with the agent window
// class: xdg-terminal-exec when there is one, else $TERMINAL, else the first
// known terminal installed. ok is false when there is no terminal to use.
func Terminal(title string, argv []string) (out []string, ok bool) {
	if util.Which("xdg-terminal-exec") != "" {
		out = []string{"xdg-terminal-exec", "--app-id=" + omarchy.WindowClass}
		if title != "" {
			out = append(out, "--title="+title)
		}
		return append(append(out, "-e"), argv...), true
	}
	if t := os.Getenv("TERMINAL"); t != "" && util.Which(t) != "" {
		return append([]string{t, "-e"}, argv...), true
	}
	for _, t := range terminals {
		if util.Which(t.cmd[0]) == "" {
			continue
		}
		out = slices.Clone(t.cmd)
		if t.class != "" {
			out = append(out, t.class+omarchy.WindowClass)
		}
		if t.title != "" && title != "" {
			out = append(out, t.title+title)
		}
		return append(append(out, t.run...), argv...), true
	}
	return argv, false
}

func inTerminal(argv []string) []string {
	out, _ := Terminal("", argv)
	return out
}

// App launches an app from the list, with launch-sheet arguments, in cwd
// (empty: where topdown runs, or ~/Work from $HOME under Omarchy).
func App(app config.App, where Where, extra []string, cwd string) Spec {
	if app.Agent != "" {
		return agentSpec(app.Agent, append(config.AgentArgs(&app, app.Agent), extra...), cwd, where)
	}
	s := Command([]string{"bash", "-lc", app.Command}, where)
	s.Cwd = cwd
	return s
}

// Command runs a plain command (no agent wrapper) here or in a new window.
func Command(argv []string, where Where) Spec {
	switch {
	case where == Here:
		return Spec{Argv: argv}
	case omarchy.Present():
		return Spec{Argv: append([]string{"omarchy-launch-tui", "--app-id=" + omarchy.WindowClass}, argv...)}
	}
	return Spec{Argv: inTerminal(argv)}
}

// Resume continues a conversation in its own project directory.
func Resume(s agent.Session, where Where, extra []string) Spec {
	var resume []string
	if a := agent.Get(s.Agent); a != nil && a.Sessions != nil {
		resume = a.Sessions.ResumeArgs(s.ID)
	}
	cwd := ""
	if util.IsDir(s.Cwd) {
		cwd = s.Cwd
	}
	args := append(append(config.AgentArgs(nil, s.Agent), extra...), resume...)
	return agentSpec(s.Agent, args, cwd, where)
}

// Default launches the default agent.
func Default(agentID string, where Where) Spec {
	return agentSpec(agentID, config.AgentArgs(nil, agentID), "", where)
}

// envArgs are `env` arguments for a spec: [-C dir] [K=V…] argv…
func envArgs(s Spec) []string {
	var out []string
	if s.Cwd != "" {
		out = append(out, "-C", s.Cwd)
	}
	for k, v := range s.Env {
		out = append(out, k+"="+v)
	}
	return append(out, s.Argv...)
}

// ExecHere replaces this process with the spec, so the terminal becomes the agent.
func ExecHere(s Spec, title string) error {
	if title != "" {
		fmt.Printf("\x1b]2;%s\x07", title)
	}
	// The spec's variables replace inherited ones (a duplicate PATH would be ambiguous).
	var env []string
	for _, kv := range os.Environ() {
		if _, override := s.Env[kv[:max(0, strings.IndexByte(kv, '='))]]; !override {
			env = append(env, kv)
		}
	}
	for k, v := range s.Env {
		env = append(env, k+"="+v)
	}
	if s.Cwd != "" {
		if err := os.Chdir(s.Cwd); err != nil {
			return err
		}
	}
	path, err := lookPath(s.Argv[0], s.Env["PATH"])
	if err != nil {
		return err
	}
	return syscall.Exec(path, s.Argv, env)
}

// lookPath resolves argv[0] against the spec's PATH when it sets one (the Omarchy shim).
func lookPath(name, path string) (string, error) {
	if path == "" || strings.Contains(name, "/") {
		return exec.LookPath(name)
	}
	for _, dir := range filepath.SplitList(path) {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s: not found", name)
}

// ExecWindow runs the spec detached in a new window. omarchy-launch-tui ends in
// `systemd-run --scope`, which stays in the foreground until that terminal
// closes, so detach fully (setsid -f) and log the terminal's chatter.
func ExecWindow(s Spec) error {
	logPath := filepath.Join(util.StateDir(), "launch.log")
	os.MkdirAll(filepath.Dir(logPath), 0o755)
	log, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command("setsid", append([]string{"-f", "env"}, envArgs(s)...)...)
	cmd.Stdout, cmd.Stderr = log, log
	return cmd.Run()
}
