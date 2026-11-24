// Package launch builds what to run and hands it over: in place (this process
// execs it, so the terminal becomes the agent) or detached in a new window.
package launch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
		s := Spec{Argv: []string{"omarchy-agent"}, Env: env}
		// Same convention as omarchy-agent: launches from $HOME start in ~/Work.
		if wd, _ := os.Getwd(); wd == util.Home() {
			if work := filepath.Join(util.Home(), "Work"); isDir(work) {
				s.Cwd = work
			}
		}
		return s
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

func inTerminal(argv []string) []string {
	if util.Which("xdg-terminal-exec") != "" {
		return append([]string{"xdg-terminal-exec", "--app-id=" + omarchy.WindowClass, "-e"}, argv...)
	}
	for _, t := range []string{"foot", "kitty", "alacritty", "wezterm", "ghostty"} {
		if util.Which(t) != "" {
			return append([]string{t, "-e"}, argv...)
		}
	}
	return argv
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// App launches an app from the list, with launch-sheet arguments.
func App(app config.App, where Where, extra []string) Spec {
	if app.Agent != "" {
		return agentSpec(app.Agent, append(config.AgentArgs(&app, app.Agent), extra...), "", where)
	}
	sh := []string{"bash", "-lc", app.Command}
	if omarchy.Present() {
		if where == Here {
			return Spec{Argv: sh}
		}
		return Spec{Argv: append([]string{"omarchy-launch-tui", "--app-id=" + omarchy.WindowClass}, sh...)}
	}
	if where == Window {
		return Spec{Argv: inTerminal(sh)}
	}
	return Spec{Argv: sh}
}

// Resume continues a conversation in its own project directory.
func Resume(s agent.Session, where Where, extra []string) Spec {
	var resume []string
	if a := agent.Get(s.Agent); a != nil && a.Sessions != nil {
		resume = a.Sessions.ResumeArgs(s.ID)
	}
	cwd := ""
	if isDir(s.Cwd) {
		cwd = s.Cwd
	}
	args := append(append(config.AgentArgs(nil, s.Agent), extra...), resume...)
	return agentSpec(s.Agent, args, cwd, where)
}

// Default launches the default agent.
func Default(agentID string, where Where) Spec {
	var app *config.App
	if a, ok := config.AppFor(config.LoadApps(), agentID); ok {
		app = &a
	}
	return agentSpec(agentID, config.AgentArgs(app, agentID), "", where)
}

// EnvArgs are `env` arguments for a spec: [-C dir] [K=V…] argv…
func EnvArgs(s Spec) []string {
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
	cmd := exec.Command("setsid", append([]string{"-f", "env"}, EnvArgs(s)...)...)
	cmd.Stdout, cmd.Stderr = log, log
	return cmd.Run()
}
