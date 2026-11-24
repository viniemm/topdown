// Package omarchy is the optional Omarchy integration. When omarchy-agent
// exists, agents launch through it (auto-permission flags, account pinning,
// ~/Work cwd, agent window class) and plan usage comes from the records the
// bar's agents plugin reads (read-only).
package omarchy

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/viniemm/topdown/internal/util"
)

const WindowClass = "org.omarchy.agent"

func Present() bool { return util.Which("omarchy-agent") != "" }

// ---- usage (read-only) ----

type Limit struct {
	Title    string  `json:"title"`
	Label    string  `json:"label"`
	Percent  float64 `json:"percent"`
	ResetsAt any     `json:"resetsAt"`
}

type Usage struct {
	Limits  []Limit `json:"limits"`
	Balance *struct {
		Remaining float64 `json:"remaining"`
		Funded    float64 `json:"funded"`
		Currency  string  `json:"currency"`
	} `json:"balance"`
	TierLabel        string  `json:"tierLabel"`
	UsageStatusText  string  `json:"usageStatusText"`
	AuthHelpText     string  `json:"authHelpText"`
	LimitsStale      bool    `json:"limitsStale"`
	HasLocalStats    bool    `json:"hasLocalStats"`
	TodayPrompts     float64 `json:"todayPrompts"`
	TodaySessions    float64 `json:"todaySessions"`
	TodayTotalTokens float64 `json:"todayTotalTokens"`
	TotalPrompts     float64 `json:"totalPrompts"`
	UpdatedAt        any     `json:"updatedAt"`
}

// usageIDs are Omarchy usage record ids that differ from the agent id.
var usageIDs = map[string]string{"cursor-agent": "cursor", "mcode": "minimax", "dsh": "deepseek", "ori": "openrouter"}

// UsageID is an app's override, else Omarchy's id for the agent.
func UsageID(override, agentID string) string {
	if override != "" {
		return override
	}
	if id, ok := usageIDs[agentID]; ok {
		return id
	}
	return agentID
}

func usageDir() string { return filepath.Join(util.StateHome(), "omarchy/agents/usage") }

func ReadUsage(id string) *Usage {
	b, err := os.ReadFile(filepath.Join(usageDir(), id+".json"))
	if err != nil {
		return nil
	}
	var u Usage
	if json.Unmarshal(b, &u) != nil {
		return nil
	}
	return &u
}

// Tightest is the fullest limit, 0..1, for the at-a-glance meter; ok=false without limits.
func Tightest(u *Usage) (float64, bool) {
	if u == nil || len(u.Limits) == 0 {
		return 0, false
	}
	m := 0.0
	for _, l := range u.Limits {
		m = max(m, l.Percent)
	}
	return m, true
}

func RefreshUsage(id string) {
	if util.Which("omarchy-agent-usage-update") != "" {
		exec.Command("omarchy-agent-usage-update", id).Run()
	}
}

// ---- launching through omarchy-agent ----

func shimDir() string { return filepath.Join(util.DataHome(), "topdown/shim") }

// EnsureShim writes two stand-ins for PATH. omarchy-agent builds each agent's
// command; the stand-ins make it run the picked agent rather than the default,
// and hand the command back so extra and resume arguments are appended, the
// session's directory pinned, and the result run in place or in a new terminal.
func EnsureShim() string {
	dir := shimDir()
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "omarchy-default-agent"), []byte("#!/bin/bash\necho \"$TOPDOWN_AGENT\"\n"), 0o755)
	tui := strings.ReplaceAll(`#!/bin/bash
# Leave PATH as it was, so nothing inside the agent session sees these stand-ins.
PATH=$(tr ':' '\n' <<<"$PATH" | grep -vxF "SHIM" | paste -sd:)
app_id=--app-id=CLASS
[[ $1 == --app-id=* ]] && app_id=$1 && shift
cmd=("$@")
[[ -n ${TOPDOWN_ARGS:-} ]] && mapfile -t -O "${#cmd[@]}" cmd <<<"$TOPDOWN_ARGS"
# omarchy-agent moves launches from $HOME to ~/Work; a resumed session must
# start in its own project dir, so pin it on the command itself.
[[ -n ${TOPDOWN_CWD:-} ]] && cmd=(env -C "$TOPDOWN_CWD" "${cmd[@]}")
inline=${TOPDOWN_INLINE:-}
unset TOPDOWN_AGENT TOPDOWN_ARGS TOPDOWN_CWD TOPDOWN_INLINE
[[ -n $inline ]] && exec "${cmd[@]}"
exec omarchy-launch-tui "$app_id" "${cmd[@]}"
`, "SHIM", dir)
	tui = strings.ReplaceAll(tui, "CLASS", WindowClass)
	os.WriteFile(filepath.Join(dir, "omarchy-launch-tui"), []byte(tui), 0o755)
	return dir
}
