package agent

import "github.com/viniemm/topdown/internal/util"

// launchOnly are the rest of Omarchy's agents: launchable now, full adapters later (M2).
var launchOnly = [][2]string{
	{"codex", "Codex"}, {"opencode", "OpenCode"}, {"copilot", "GitHub Copilot"}, {"crush", "Crush"},
	{"grok", "Grok"}, {"cursor-agent", "Cursor Agent"}, {"amp", "Amp"}, {"kimi", "Kimi"},
	{"qwen", "Qwen Code"}, {"kiro-cli", "Kiro"}, {"kilo", "Kilo"}, {"devin", "Devin"}, {"muse", "Muse"},
	{"ori", "Ori"}, {"dsh", "DeepSeek Shell"}, {"mcode", "MiniMax Code"}, {"freebuff", "Freebuff"},
	{"commandcode", "Command Code"}, {"openclaw", "OpenClaw"},
}

// All is every agent topdown knows, full adapters first.
var All = func() []*Adapter {
	all := []*Adapter{Claude, Omp, Pi, Hermes, Agy}
	for _, a := range launchOnly {
		all = append(all, &Adapter{ID: a[0], Label: a[1], Bin: a[0]})
	}
	return all
}()

var byID = func() map[string]*Adapter {
	m := map[string]*Adapter{}
	for _, a := range All {
		m[a.ID] = a
	}
	return m
}()

// Get returns the adapter for an agent id, or nil.
func Get(id string) *Adapter { return byID[id] }

func (a *Adapter) Installed() bool { return util.Which(a.Bin) != "" }

// Live lists the open sessions of the given agents: id -> pid.
func Live(ids ...string) map[string]int {
	live := map[string]int{}
	for _, id := range ids {
		if a := Get(id); a != nil {
			if l, ok := a.Sessions.(Liver); ok {
				for sid, pid := range l.Live() {
					live[sid] = pid
				}
			}
		}
	}
	return live
}
