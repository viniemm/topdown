package launch_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/viniemm/topdown/internal/agent"
	"github.com/viniemm/topdown/internal/config"
	"github.com/viniemm/topdown/internal/launch"
	"github.com/viniemm/topdown/internal/testhome"
)

// stubs puts stand-in executables (and optionally omarchy-agent) alone on PATH.
func stubs(t *testing.T, home string, omarchy bool) {
	dir := filepath.Join(home, "stubs")
	os.MkdirAll(dir, 0o755)
	bins := []string{"claude", "hermes"}
	if omarchy {
		bins = append(bins, "omarchy-agent")
	}
	for _, b := range bins {
		os.WriteFile(filepath.Join(dir, b), []byte("#!/bin/sh\n"), 0o755)
	}
	t.Setenv("PATH", dir)
}

func TestResumeWithoutOmarchy(t *testing.T) {
	home := testhome.New(t)
	stubs(t, home, false)
	cwd := filepath.Join(home, "w/a")
	os.MkdirAll(cwd, 0o755)
	s := agent.Session{Agent: "claude", ID: "c1", Cwd: cwd}
	got := launch.Resume(s, launch.Here, []string{"--effort", "high"})
	want := []string{"claude", "--permission-mode", "auto", "--effort", "high", "--resume", "c1"}
	if !slices.Equal(got.Argv, want) || got.Cwd != cwd || got.Env != nil {
		t.Fatalf("got %+v", got)
	}
}

func TestOmarchyGoesThroughShim(t *testing.T) {
	home := testhome.New(t)
	stubs(t, home, true)
	config.SaveApps([]config.App{{ID: "hermes", Label: "Hermes", Agent: "hermes"}})
	s := launch.App(config.App{ID: "hermes", Label: "Hermes", Agent: "hermes"}, launch.Here, nil, "")
	if !slices.Equal(s.Argv, []string{"omarchy-agent"}) {
		t.Fatalf("argv %v", s.Argv)
	}
	if s.Env["TOPDOWN_AGENT"] != "hermes" || s.Env["TOPDOWN_ARGS"] != "--tui" || s.Env["TOPDOWN_INLINE"] != "1" {
		t.Fatalf("env %v", s.Env)
	}
	if !strings.HasSuffix(strings.Split(s.Env["PATH"], ":")[0], "topdown/shim") {
		t.Fatalf("PATH %s", s.Env["PATH"])
	}
}

func TestAppArgsReplaceDefaults(t *testing.T) {
	home := testhome.New(t)
	stubs(t, home, true)
	args, _ := json.Marshal("--cli 'two words'")
	s := launch.App(config.App{ID: "h", Label: "H", Agent: "hermes", Args: args}, launch.Here, nil, "")
	if s.Env["TOPDOWN_ARGS"] != "--cli\ntwo words" {
		t.Fatalf("args %q", s.Env["TOPDOWN_ARGS"])
	}
}

func TestShellSplit(t *testing.T) {
	got := config.ShellSplit(`a "b c" 'd e' f\ g ""`)
	if !slices.Equal(got, []string{"a", "b c", "d e", "f g", ""}) {
		t.Fatalf("%q", got)
	}
}

// onPath puts only the named stand-in executables on PATH.
func onPath(t *testing.T, bins ...string) {
	dir := t.TempDir()
	for _, b := range bins {
		os.WriteFile(filepath.Join(dir, b), []byte("#!/bin/sh\n"), 0o755)
	}
	t.Setenv("PATH", dir)
	t.Setenv("TERMINAL", "")
}

func TestTerminalPrefersXdgTerminalExec(t *testing.T) {
	onPath(t, "xdg-terminal-exec", "foot")
	got, ok := launch.Terminal("topdown", []string{"topdown", "pick"})
	want := []string{"xdg-terminal-exec", "--app-id=org.omarchy.agent", "--title=topdown", "-e", "topdown", "pick"}
	if !ok || !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestTerminalUsesTERMINAL(t *testing.T) {
	onPath(t, "myterm", "foot")
	t.Setenv("TERMINAL", "myterm")
	got, ok := launch.Terminal("topdown", []string{"x"})
	if !ok || !slices.Equal(got, []string{"myterm", "-e", "x"}) {
		t.Fatalf("got %q", got)
	}
}

func TestTerminalFallbacks(t *testing.T) {
	for bin, want := range map[string][]string{
		"foot":           {"foot", "--app-id=org.omarchy.agent", "--title=topdown", "x"},
		"wezterm":        {"wezterm", "start", "--class=org.omarchy.agent", "--", "x"},
		"gnome-terminal": {"gnome-terminal", "--class=org.omarchy.agent", "--title=topdown", "--", "x"},
		"konsole":        {"konsole", "-e", "x"},
	} {
		onPath(t, bin)
		if got, ok := launch.Terminal("topdown", []string{"x"}); !ok || !slices.Equal(got, want) {
			t.Errorf("%s: got %q", bin, got)
		}
	}
}

func TestTerminalNoneFound(t *testing.T) {
	onPath(t)
	if got, ok := launch.Terminal("", []string{"x"}); ok || !slices.Equal(got, []string{"x"}) {
		t.Fatalf("got %q %v", got, ok)
	}
}
