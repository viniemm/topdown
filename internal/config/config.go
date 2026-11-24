// Package config holds the user's app list and default agent.
//
//	~/.config/topdown/apps.json   the list (migrated from Omarchy's agent-launcher.json)
//	default agent                 Omarchy's ~/.config/omarchy/defaults/agent on Omarchy, else ~/.config/topdown/default
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/viniemm/topdown/internal/agent"
	"github.com/viniemm/topdown/internal/omarchy"
	"github.com/viniemm/topdown/internal/util"
)

// App is one entry in the list: a known agent, or any command to run in a terminal.
type App struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Agent   string `json:"agent,omitempty"`
	Command string `json:"command,omitempty"`
	// Args are extra arguments for an agent: a list or one shell-quoted string.
	Args json.RawMessage `json:"args,omitempty"`
	// Usage overrides the Omarchy usage record id.
	Usage string `json:"usage,omitempty"`
}

func AppsFile() string   { return filepath.Join(util.ConfigDir(), "apps.json") }
func legacyFile() string { return filepath.Join(util.ConfigHome(), "omarchy/agent-launcher.json") }

func defaultFile() string {
	if omarchy.Present() {
		return filepath.Join(util.ConfigHome(), "omarchy/defaults/agent")
	}
	return filepath.Join(util.ConfigDir(), "default")
}

// seed is the first-run list: installed agents topdown fully supports.
func seed() []App {
	var apps []App
	for _, id := range []string{"claude", "codex", "opencode", "omp", "pi", "hermes", "agy"} {
		if a := agent.Get(id); a != nil && a.Installed() {
			apps = append(apps, App{ID: a.ID, Label: a.Label, Agent: a.ID})
		}
	}
	return apps
}

func LoadApps() []App {
	for _, f := range []string{AppsFile(), legacyFile()} {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var apps []App
		if json.Unmarshal(b, &apps) == nil {
			if f != AppsFile() {
				SaveApps(apps) // migrate once
			}
			return apps
		}
	}
	apps := seed()
	SaveApps(apps)
	return apps
}

func SaveApps(apps []App) error {
	if apps == nil {
		apps = []App{}
	}
	b, err := json.MarshalIndent(apps, "", "  ")
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(AppsFile(), append(b, '\n'), 0o644)
}

func Default() string {
	b, err := os.ReadFile(defaultFile())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
}

func SetDefault(agentID string) error {
	return util.WriteFileAtomic(defaultFile(), []byte(agentID+"\n"), 0o644)
}

// AppFor returns the first app that runs agentID.
func AppFor(apps []App, agentID string) (App, bool) {
	for _, a := range apps {
		if a.Agent == agentID {
			return a, true
		}
	}
	return App{}, false
}

// AgentArgs: an app's own "args", else the adapter's defaults.
func AgentArgs(app *App, agentID string) []string {
	if app == nil {
		if a, ok := AppFor(LoadApps(), agentID); ok {
			app = &a
		}
	}
	if app != nil && len(app.Args) > 0 {
		var list []string
		if json.Unmarshal(app.Args, &list) == nil {
			return list
		}
		var s string
		if json.Unmarshal(app.Args, &s) == nil {
			return ShellSplit(s)
		}
	}
	if a := agent.Get(agentID); a != nil {
		return append([]string(nil), a.DefaultArgs...)
	}
	return nil
}

// ShellSplit does minimal POSIX-ish word splitting for a shell-quoted "args" string.
func ShellSplit(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	has := false
	r := []rune(s)
	for i := 0; i < len(r); i++ {
		c := r[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else if c == '\\' && quote == '"' && i+1 < len(r) {
				i++
				cur.WriteRune(r[i])
			} else {
				cur.WriteRune(c)
			}
		case c == '\'' || c == '"':
			quote, has = c, true
		case c == '\\' && i+1 < len(r):
			i++
			cur.WriteRune(r[i])
			has = true
		case c == ' ' || c == '\t' || c == '\n':
			if cur.Len() > 0 || has {
				out = append(out, cur.String())
			}
			cur.Reset()
			has = false
		default:
			cur.WriteRune(c)
			has = true
		}
	}
	if cur.Len() > 0 || has {
		out = append(out, cur.String())
	}
	return out
}
