package theme

// Roles and colour coding, in true colour from the palette. Selection, dim
// text and borders are blended from it, so they stay right in any theme.
// Bubble Tea downsamples for terminals without true colour; with no palette at
// all, everything uses the 16 ANSI colours.
//
// ~/.config/topdown/theme.json overrides with hex colours: any role ("accent",
// "border", "dim", "muted", "success", "warning", "error", "selected") and
// per-agent colours: {"agents": {"claude": "#d97757"}}.

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"time"

	"github.com/viniemm/topdown/internal/util"
)

type Role int

const (
	Accent Role = iota
	Border
	Dim
	Muted
	Success
	Warning
	Error
)

var roleNames = []string{"accent", "border", "dim", "muted", "success", "warning", "error"}

type hue int

const (
	orange hue = iota
	green
	blue
	magenta
	cyan
	yellow
	red
)

var hues = []hue{orange, green, blue, magenta, cyan, yellow, red}

// Agents' preferred hues (roughly their brands), resolved against the theme.
var agentHue = map[string]hue{
	"claude": orange, "codex": green, "opencode": cyan, "copilot": magenta, "omp": blue, "pi": cyan,
	"hermes": magenta, "agy": yellow, "grok": red, "crush": magenta, "cursor-agent": blue, "amp": red,
}

var (
	ansiHue  = map[hue]string{orange: "33", green: "32", blue: "34", magenta: "35", cyan: "36", yellow: "93", red: "31"}
	ansiRole = []string{"34", "34", "2", "37", "32", "33", "31"}
)

var (
	pal        *Palette
	roles      []RGB // nil without a palette
	selectedBg *RGB
	agentColor = map[string]any{} // RGB or hue
	overrides  = map[string]RGB{}
	animations = true
)

func (p *Palette) hue(h hue) RGB {
	switch h {
	case orange:
		return p.Orange
	case green:
		return p.Green
	case blue:
		return p.Blue
	case magenta:
		return p.Magenta
	case cyan:
		return p.Cyan
	case yellow:
		return p.Yellow
	}
	return p.Red
}

// Init sets the palette (nil: 16-colour fallback) and applies theme.json overrides.
func Init(p *Palette) {
	pal, roles, selectedBg = p, nil, nil
	overrides = map[string]RGB{}
	animations = os.Getenv("TOPDOWN_REDUCED_MOTION") == ""
	if p != nil {
		roles = []RGB{p.Accent, Mix(p.Bg, p.Accent, 0.55), Mix(p.Fg, p.Bg, 0.5), Mix(p.Fg, p.Bg, 0.22), p.Green, p.Yellow, p.Red}
		s := Mix(p.Bg, p.Accent, 0.2)
		selectedBg = &s
	}
	b, err := os.ReadFile(filepath.Join(util.ConfigDir(), "theme.json"))
	if err != nil {
		return
	}
	var o map[string]json.RawMessage
	if json.Unmarshal(b, &o) != nil {
		return
	}
	str := func(raw json.RawMessage) string {
		var s string
		json.Unmarshal(raw, &s)
		return s
	}
	if roles != nil {
		for i, name := range roleNames {
			if c, ok := Hex(str(o[name])); ok {
				roles[i] = c
			}
		}
		if c, ok := Hex(str(o["selected"])); ok {
			selectedBg = &c
		}
	}
	var anim *bool
	if json.Unmarshal(o["animations"], &anim) == nil && anim != nil && !*anim {
		animations = false
	}
	var agents map[string]string
	json.Unmarshal(o["agents"], &agents)
	for id, v := range agents {
		if c, ok := Hex(v); ok {
			overrides[id] = c
		}
	}
}

// AssignAgents gives each listed agent a colour: its preferred hue unless an earlier agent has it.
func AssignAgents(ids []string) {
	agentColor = map[string]any{}
	for _, id := range ids {
		assign(id)
	}
}

// assign gives one more agent a colour without disturbing those already given.
func assign(id string) {
	if c, ok := overrides[id]; ok {
		agentColor[id] = c
		return
	}
	used := map[hue]bool{}
	for _, c := range agentColor {
		if h, ok := c.(hue); ok {
			used[h] = true
		}
	}
	h, ok := agentHue[id]
	if !ok || used[h] {
		h = hues[hashOf(id)%uint32(len(hues))] // all taken: a stable reuse
		for _, c := range hues {
			if !used[c] {
				h = c
				break
			}
		}
	}
	agentColor[id] = h
}

func hashOf(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

func fg(c RGB, s string) string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[39m", c[0], c[1], c[2], s)
}

func paint(c any, s string) string {
	switch v := c.(type) {
	case RGB:
		return fg(v, s)
	case hue:
		if pal != nil {
			return fg(pal.hue(v), s)
		}
		return "\x1b[" + ansiHue[v] + "m" + s + "\x1b[39m"
	}
	return s
}

// Fg colours s in a semantic role.
func Fg(r Role, s string) string {
	if roles != nil {
		return fg(roles[r], s)
	}
	code := ansiRole[r]
	if code == "2" {
		return "\x1b[2m" + s + "\x1b[22m"
	}
	return "\x1b[" + code + "m" + s + "\x1b[39m"
}

func Bold(s string) string    { return "\x1b[1m" + s + "\x1b[22m" }
func Inverse(s string) string { return "\x1b[7m" + s + "\x1b[27m" }

// Selected is the selection background; inner colour changes never reset it (only \x1b[49m would).
func Selected(s string) string {
	if selectedBg == nil {
		return Inverse(s)
	}
	c := *selectedBg
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm%s\x1b[49m", c[0], c[1], c[2], s)
}

// ---- colour coding ----

// Agent colours s (default: the id) in the agent's own colour, consistently everywhere.
func Agent(id, s string) string {
	if _, ok := agentColor[id]; !ok {
		assign(id)
	}
	return paint(agentColor[id], s)
}

// Project colours s in a stable hue from the project name, softened toward the text colour.
func Project(name, s string) string {
	if pal == nil {
		return Fg(Muted, s)
	}
	return fg(Mix(pal.hue(hues[hashOf(name)%uint32(len(hues))]), pal.Fg, 0.45), s)
}

// Age fades timestamps: today bright, this week muted, older dim.
func Age(ts float64, s string) string {
	days := time.Since(time.UnixMilli(int64(ts*1000))).Hours() / 24
	if pal == nil {
		if days < 1 {
			return Fg(Muted, s)
		}
		return Fg(Dim, s)
	}
	t := 0.68
	switch {
	case days < 1:
		t = 0.1
	case days < 7:
		t = 0.35
	case days < 30:
		t = 0.55
	}
	return fg(Mix(pal.Fg, pal.Bg, t), s)
}

// Level colours by 0..1: green, through yellow, to red (usage meters).
func Level(p float64, s string) string {
	v := min(1, max(0, p))
	if pal == nil {
		switch {
		case v < 0.6:
			return Fg(Success, s)
		case v < 0.85:
			return Fg(Warning, s)
		}
		return Fg(Error, s)
	}
	if v < 0.6 {
		return fg(Mix(pal.Green, pal.Yellow, v/0.6), s)
	}
	return fg(Mix(pal.Yellow, pal.Red, (v-0.6)/0.4), s)
}

// ---- gradients and effects (true colour only) ----

// Rich reports whether gradients and animated effects are available: a palette
// is known (true colour). Without one, callers draw the plain version.
func Rich() bool { return pal != nil }

// Animated reports whether motion is wanted (true colour, not turned off by
// "animations": false in theme.json or TOPDOWN_REDUCED_MOTION).
func Animated() bool { return pal != nil && animations }

// Bg and FgColor are the palette's background and text colours.
func Bg() RGB      { return pal.Bg }
func FgColor() RGB { return pal.Fg }

// RoleColor is a role's true colour (only meaningful when Rich).
func RoleColor(r Role) RGB { return roles[r] }

// SelectedColor is the selection background (only meaningful when Rich).
func SelectedColor() RGB { return *selectedBg }

// LogoStops are the logo's gradient stops: magenta, the accent, cyan (omp uses pink → blue).
func LogoStops() []RGB { return []RGB{pal.Magenta, pal.Accent, pal.Cyan} }

// Green is the success colour (open-conversation markers pulse in it).
func Green() RGB { return pal.Green }

// Grad is the colour at t (0..1) along evenly spaced stops.
func Grad(stops []RGB, t float64) RGB {
	t = min(1, max(0, t))
	if len(stops) == 1 {
		return stops[0]
	}
	f := t * float64(len(stops)-1)
	i := min(int(f), len(stops)-2)
	return Mix(stops[i], stops[i+1], f-float64(i))
}

// FgRGB and BgRGB colour s with an explicit true colour.
func FgRGB(c RGB, s string) string { return fg(c, s) }
func BgRGB(c RGB, s string) string {
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm%s\x1b[49m", c[0], c[1], c[2], s)
}
