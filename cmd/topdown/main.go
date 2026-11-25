// topdown: one TUI to launch, resume and manage coding agents.
//
//	topdown [pick]            the hub in this terminal
//	topdown open              the hub in a new terminal window (Super+Shift+A)
//	topdown default           launch the default agent; none yet -> the hub chooses (Super+A)
//	topdown launch <app-id>   launch one app from the list
//	topdown resume <agent> <session-id>
//	topdown list [agent…]     recent conversations, tab-separated (for scripts)
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/viniemm/topdown/internal/catalog"
	"github.com/viniemm/topdown/internal/config"
	"github.com/viniemm/topdown/internal/hypr"
	"github.com/viniemm/topdown/internal/launch"
	"github.com/viniemm/topdown/internal/omarchy"
	"github.com/viniemm/topdown/internal/theme"
	"github.com/viniemm/topdown/internal/ui"
	"github.com/viniemm/topdown/internal/util"
)

// version is set at build time: -ldflags "-X main.version=…".
var version = "dev"

const usage = "topdown [pick|open|default|launch <app>|resume <agent> <id>|list [agent…]]"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "topdown:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "pick"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "pick":
		return pick(slices.Contains(args, "--choose-default"))
	case "open":
		return openWindow(args)
	case "default":
		id := config.Default()
		if id == "" {
			return openWindow([]string{"--choose-default"})
		}
		return launch.ExecWindow(launch.Default(id, launch.Window))
	case "launch":
		if len(args) == 0 {
			return errors.New("usage: topdown launch <app-id>")
		}
		for _, a := range config.LoadApps() {
			if a.ID == args[0] {
				return launch.ExecHere(launch.App(a, launch.Here, nil, ""), a.Label)
			}
		}
		return fmt.Errorf("no app named %q", args[0])
	case "resume":
		if len(args) < 2 {
			return errors.New("usage: topdown resume <agent> <session-id>")
		}
		s, ok := catalog.Get(args[0], args[1])
		if !ok {
			return errors.New("session not found")
		}
		return launch.ExecHere(launch.Resume(s, launch.Here, nil), s.Title)
	case "list":
		agents := args
		if len(agents) == 0 {
			agents = config.SessionAgents(config.LoadApps())
		}
		for _, s := range catalog.Recent(agents, 50).Sessions {
			fmt.Println(strings.Join([]string{util.Stamp(s.When), s.Agent, s.ID, s.Cwd, s.Title}, "\t"))
		}
		return nil
	case "-v", "--version", "version":
		fmt.Println("topdown", version)
		return nil
	case "-h", "--help", "help":
		if omarchy.Present() {
			fmt.Println(usage + "  (Omarchy integration on)")
		} else {
			fmt.Println(usage)
		}
		return nil
	}
	return fmt.Errorf("unknown command: %s\n%s", cmd, usage)
}

func pick(chooseDefault bool) error {
	theme.Init(theme.Detect())
	hub := ui.New(ui.Options{ChooseDefault: chooseDefault, Version: version})
	if _, err := tea.NewProgram(hub).Run(); err != nil {
		return err
	}
	r := hub.Result
	switch r.Action {
	case "focus":
		if r.Pane != nil {
			r.Pane.Select()
		}
		hypr.Focus(r.Address)
	case "attach":
		r.Pane.Select()
		return act(launch.Command(r.Pane.AttachArgv(), r.Where), r.Where, "herdr")
	case "launch":
		return act(launch.App(r.App, r.Where, r.Extra, r.Cwd), r.Where, r.App.Label)
	case "resume":
		return act(launch.Resume(r.Session, r.Where, r.Extra), r.Where, r.Session.Agent+" — "+util.Oneline(r.Session.Title, 60))
	}
	return nil
}

func act(s launch.Spec, where launch.Where, title string) error {
	if where == launch.Window {
		return launch.ExecWindow(s)
	}
	return launch.ExecHere(s, title)
}

// openWindow runs the hub in a new terminal window with the agent window class
// (see launch.Terminal for which terminal).
func openWindow(args []string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	argv, ok := launch.Terminal("topdown", append([]string{self, "pick"}, args...))
	if !ok {
		return fmt.Errorf("no terminal found: install xdg-terminal-exec or set $TERMINAL")
	}
	if util.Which("uwsm-app") != "" {
		argv = append([]string{"uwsm-app", "--"}, argv...)
	}
	c := exec.Command("setsid", append([]string{"-f"}, argv...)...)
	return c.Run()
}
