package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	tea "charm.land/bubbletea/v2"

	"github.com/viniemm/topdown/internal/agent"
	"github.com/viniemm/topdown/internal/catalog"
	"github.com/viniemm/topdown/internal/config"
	"github.com/viniemm/topdown/internal/omarchy"
	"github.com/viniemm/topdown/internal/theme"
	"github.com/viniemm/topdown/internal/util"
)

// viewCache is the last full frame. While only the header band moves (the
// logo's shimmer, the landscape) and nothing else has happened, a frame
// repaints just the band's rows over it: the list, sidebar and footer cost
// far more to draw than the band.
type viewCache struct {
	ok    bool // the cached frame is still right below the band
	w, h  int
	at    time.Time
	head  int // band rows, from line 1
	lines []string
}

const viewCacheAge = 2 * time.Second // relative times ("5 min ago") in the body stay fresh

func (h *Hub) View() tea.View {
	if c := &h.cache; c.ok && c.w == h.width && c.h == h.height && h.anim.now.Sub(c.at) < viewCacheAge {
		if hd := h.header(); hd != nil {
			W := max(40, h.width)
			if band := hd(W - 4); len(band) == c.head {
				for i, l := range band {
					c.lines[1+i] = headLine(l, W)
				}
				return h.view(strings.Join(c.lines, "\n"))
			}
		}
	}
	content := h.render()
	h.cache = viewCache{w: h.width, h: h.height, at: h.anim.now, lines: strings.Split(content, "\n"),
		ok: h.fade() >= 1 && !h.anim.bodyPulsing && len(h.refreshing) == 0}
	if h.height >= 22 && h.width >= 50 {
		h.cache.head = len(logoMark)
	} else {
		h.cache.ok = false
	}
	return h.view(content)
}

func (h *Hub) view(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (h *Hub) render() string {
	h.anim.pulsing, h.anim.bodyPulsing = false, false // set again by whatever draws a marker
	entries := h.entries()
	content := h.frame.render(h.width, h.height, entries,
		func(w, rows int) []string {
			return h.frame.sidebar(entries, w, rows, h.scope, h.focusScope, func(e entry) entryStyle {
				var icon string
				switch e.kind {
				case "apps":
					icon = theme.Fg(theme.Accent, "▦")
				case "recent":
					icon = theme.Fg(theme.Accent, "◷")
				default:
					icon = theme.Agent(e.label, "◆")
				}
				return entryStyle{icon: icon, annotation: theme.Fg(theme.Dim, e.annotation)}
			})
		},
		h.renderBody, h.footer(), h.header())
	return content
}

// header is the logo band, when the window has room for it: the logo, the
// stats on the right, and the landscape in the space between (smaller as the
// window narrows, gone when it has no room). The logo is never cut.
func (h *Hub) header() func(w int) []string {
	h.anim.scenery = false
	if h.height < 22 || h.width < 50 {
		return nil
	}
	return func(w int) []string {
		f := h.logoFrame()
		var lines []string
		if w >= logoWidth+30 {
			lines = logo(f, "  v"+h.opts.Version)
		} else {
			lines = compactLogo(f)
		}
		open := len(h.live)
		dim := func(s string) string { return theme.Fg(theme.Dim, s) }
		stats := []string{
			dim(fmt.Sprintf("%d apps", len(h.apps))),
			"",
			"",
			dim("agents, from the top down"),
		}
		if open > 0 {
			stats[0] += dim(" · ") + h.liveDot() + dim(fmt.Sprintf(" %d open", open))
		}
		if d := config.Default(); d != "" {
			stats[1] = dim("default ") + theme.Agent(d, d)
		}
		left, right := 0, 0 // the logo's columns, and the stats' (with their margin)
		for i := range lines {
			left = max(left, 1+width(lines[i]))
		}
		shown := make([]bool, len(lines))
		for i := range lines {
			// Stats only where they fit beside the logo.
			if i < len(stats) && width(lines[i])+width(stats[i])+4 <= w {
				shown[i] = true
				right = max(right, width(stats[i])+1)
			}
		}
		var land []string
		if sw := w - left - right - 2*sceneGap; len(lines) == sceneRows {
			land = h.scenery(sw, f.reveal)
		}
		for i := range lines {
			l := " " + lines[i]
			if land != nil {
				l = pad(l, left+sceneGap) + land[i]
			}
			if shown[i] {
				lines[i] = spread(l, stats[i]+" ", w)
			} else {
				lines[i] = fit(l, w)
			}
		}
		return lines
	}
}

// liveDot is the open-conversation marker, breathing when animated.
func (h *Hub) liveDot() string {
	h.anim.pulsing = true
	if !theme.Animated() {
		return theme.Fg(theme.Success, "●")
	}
	return theme.FgRGB(theme.Mix(theme.Green(), theme.Bg(), 0.6*h.pulse()), "●")
}

// bodyDot is liveDot for the list and detail panel.
func (h *Hub) bodyDot() string {
	h.anim.bodyPulsing = true
	return h.liveDot()
}

// selectBar paints the selection: a background that fades in, and fades out
// toward the right edge; inverse video without true colour.
func (h *Hub) selectBar(line string, w int) string {
	if !theme.Rich() {
		return theme.Selected(line)
	}
	f := h.fade()
	bg, accent := theme.Bg(), theme.RoleColor(theme.Accent)
	left, right := theme.Mix(bg, accent, 0.34*f), theme.Mix(bg, accent, 0.06*f)
	var b strings.Builder
	// About two dozen bands: smooth enough, and each Cut rescans the line, so
	// narrower bands would cost far more on every frame the hub draws.
	step := max(2, w/24)
	for x := 0; x < w; x += step {
		b.WriteString(theme.BgRGB(theme.Mix(left, right, float64(x)/float64(max(1, w-1))), ansi.Cut(line, x, min(w, x+step))))
	}
	return b.String()
}

func (h *Hub) footer() string {
	k := func(key, what string) string { return theme.Fg(theme.Accent, key) + " " + what }
	sep := theme.Fg(theme.Dim, " · ")
	switch {
	case h.modal != nil && h.modal.kind == "sheet":
		return strings.Join([]string{k("↑/↓", "option"), k("←/→", "value"), k("⏎", "launch"), k("Alt-⏎", "new window"), k("esc", "back")}, sep)
	case h.modal != nil:
		return strings.Join([]string{k("⏎", "ok"), k("esc", "cancel")}, sep)
	case h.focusScope:
		return strings.Join([]string{k("↑/↓", "scope"), k("⏎/→", "list"), k("esc", "close")}, sep)
	}
	other := "apps"
	if h.scope == "apps" {
		other = "recent"
	}
	parts := []string{k("⏎", "open"), k("Alt-⏎", "window"), k("^O", "options"), k("Tab", other)}
	if h.scope == "apps" {
		parts = append(parts, k("→", "chats"), k("^D", "default"), k("^R", "usage"), k("^A", "add"))
	} else if strings.HasPrefix(h.scope, "agent:") {
		parts = append(parts, k("←", "back"))
	}
	parts = append(parts, k("^E", "rename"), k("^X", "delete"), k("esc", "close"))
	return strings.Join(parts, sep)
}

func (h *Hub) renderBody(w, height int) []string {
	title := "Apps"
	switch {
	case h.scope == "recent":
		title = "Recent conversations"
	case strings.HasPrefix(h.scope, "agent:"):
		title = strings.TrimPrefix(h.scope, "agent:") + " conversations"
	}
	banner := ""
	if h.opts.ChooseDefault {
		banner = theme.Fg(theme.Warning, "  No default agent yet: Enter makes it the default and launches it")
	}
	out := []string{fit(" "+theme.Bold(title)+banner, w)}
	if h.modal != nil {
		out = append(out, "")
		for _, l := range h.renderModal(w - 2) {
			out = append(out, " "+l)
		}
		for len(out) < height {
			out = append(out, "")
		}
		return out[:height]
	}
	h.search.SetWidth(max(1, w-5))
	if h.focusScope {
		h.search.Blur()
	} else {
		h.search.Focus()
	}
	out = append(out, " "+theme.Fg(theme.Accent, "›")+" "+h.search.View())

	var detail []string
	for _, l := range h.renderDetail(w - 2) {
		detail = append(detail, " "+l)
	}
	listRows := max(3, height-len(out)-len(detail)-1)
	h.bodyRows = listRows
	if h.cursor < h.scroll {
		h.scroll = h.cursor
	}
	if h.cursor >= h.scroll+listRows {
		h.scroll = h.cursor - listRows + 1
	}
	aw, lw := 6, 10
	for _, r := range h.rows {
		if r.kind == rowSession {
			aw = max(aw, len(r.s.Agent))
		}
	}
	for _, a := range h.apps {
		lw = max(lw, width(a.Label))
	}
	thumb := scrollbar(len(h.rows), listRows, h.scroll)
	for i := 0; i < listRows; i++ {
		line := ""
		if j := h.scroll + i; j < len(h.rows) {
			line = h.renderRow(h.rows[j], aw, lw)
		}
		line = fit(line, w-2)
		if h.scroll+i == h.cursor && !h.focusScope {
			line = h.selectBar(line, w-2)
		}
		t := " "
		if i < len(thumb) {
			t = thumb[i]
		}
		out = append(out, line+t+" ")
	}
	out = append(out, border(strings.Repeat("─", w)))
	out = append(out, detail...)
	for len(out) < height {
		out = append(out, "")
	}
	return out[:height]
}

func (h *Hub) renderRow(r row, aw, lw int) string {
	dim := func(s string) string { return theme.Fg(theme.Dim, s) }
	switch r.kind {
	case rowApp:
		a := h.apps[r.app]
		def := " "
		if a.Agent != "" && a.Agent == config.Default() {
			def = theme.Fg(theme.Warning, "★")
		}
		meter := strings.Repeat(" ", 6)
		if t, ok := omarchy.Tightest(omarchy.ReadUsage(omarchy.UsageID(a.Usage, a.Agent))); ok && a.Agent != "" {
			meter = bar(t, 6)
		}
		dot, kind := dim("◇"), "tui: "+a.Command
		if a.Agent != "" {
			dot, kind = theme.Agent(a.Agent, "◆"), "agent: "+a.Agent
		}
		return " " + def + " " + dot + " " + pad(a.Label, lw) + "  " + meter + "  " + dim(kind)
	case rowAdd:
		return "   " + theme.Fg(theme.Accent, "+ Add app…")
	case rowMore:
		return dim("   ↓ more conversations…")
	case rowEmpty:
		return dim("   " + r.text)
	}
	s := r.s
	live := " "
	if _, ok := h.live[s.ID]; ok {
		live = h.bodyDot()
	}
	proj := util.Project(s.Cwd)
	return " " + live + " " + theme.Age(s.When, pad(util.Stamp(s.When), 11)) + theme.Agent(s.Agent, pad(s.Agent, aw)) +
		"  " + theme.Project(proj, pad(cut(proj, 12), 12)) + " " + util.Oneline(s.Title, 200)
}

func (h *Hub) renderDetail(w int) []string {
	r, ok := h.current()
	if !ok {
		return nil
	}
	dim := func(s string) string { return theme.Fg(theme.Dim, s) }
	switch r.kind {
	case rowApp:
		a := h.apps[r.app]
		star := ""
		if a.Agent != "" && a.Agent == config.Default() {
			star = "  " + theme.Fg(theme.Warning, "★ default")
		}
		what := "command: " + a.Command
		if a.Agent != "" {
			what = "agent: " + a.Agent
		}
		head := theme.Bold(a.Label) + star + "  " + dim(what)
		if a.Agent == "" {
			return []string{head}
		}
		if h.refreshing[a.Agent] {
			head += "  " + theme.Fg(theme.Accent, h.spinner()) + dim(" refreshing usage…")
		}
		return append([]string{head}, usageLines(omarchy.ReadUsage(omarchy.UsageID(a.Usage, a.Agent)), w)...)
	case rowSession:
		s, found := catalog.Get(r.s.Agent, r.s.ID)
		if !found {
			s = r.s
		}
		live := ""
		if _, ok := h.live[s.ID]; ok {
			live = "  " + h.bodyDot() + theme.Fg(theme.Success, " open")
		}
		out := []string{
			theme.Bold(util.Oneline(s.Title, w)),
			theme.Agent(s.Agent, s.Agent) + "  " + theme.Project(util.Project(s.Cwd), util.ShortPath(s.Cwd)) + "  " +
				theme.Age(s.When, util.Stamp(s.When)+" · "+util.Ago(s.When)) + live,
		}
		for i, p := range s.Prompts {
			if i == 2 {
				break
			}
			out = append(out, theme.Fg(theme.Accent, "›")+" "+util.Oneline(p, w-2))
		}
		return out
	case rowAdd:
		return []string{dim("Installed agents topdown knows, or any command to run in a terminal.")}
	}
	return nil
}

func (h *Hub) renderModal(w int) []string {
	m := h.modal
	hint := func(s string) string { return theme.Fg(theme.Dim, s) }
	switch m.kind {
	case "message":
		return append(append([]string{}, m.lines...), "", hint("any key to go back"))
	case "confirm":
		return append(append([]string{}, m.lines...), "", theme.Fg(theme.Accent, "y")+" yes   "+theme.Fg(theme.Accent, "n")+" no")
	case "input":
		m.input.SetWidth(max(1, w-12))
		return []string{theme.Bold(m.title), "", truncate(m.input.View(), w), "", hint("⏎ save · esc cancel")}
	case "add":
		out := []string{theme.Bold("Add which app?"), ""}
		for i, it := range m.items {
			if i == m.cursor {
				out = append(out, theme.Selected(fit(" "+it.label, w)))
			} else {
				out = append(out, " "+it.label)
			}
		}
		return out
	}
	verb := "Launch"
	if m.row.kind == rowSession {
		verb = "Resume"
	}
	out := []string{theme.Bold(verb + " " + m.agentID + " with…"), ""}
	for i, o := range m.options {
		var choices strings.Builder
		for j, c := range o.Choices {
			if j == m.values[i] {
				choices.WriteString(theme.Selected(theme.Fg(theme.Accent, " "+c+" ")))
			} else {
				choices.WriteString(theme.Fg(theme.Dim, " "+c+" "))
			}
		}
		label := "  "
		if i == m.cursor {
			label = theme.Fg(theme.Accent, "▸ ")
		}
		out = append(out, truncate(label+pad(o.Label, 12)+choices.String(), w))
	}
	bin := m.agentID
	if a := agent.Get(m.agentID); a != nil {
		bin = a.Bin
	}
	out = append(out, "", hint(strings.TrimSpace(bin+" "+strings.Join(sheetArgs(m), " "))))
	if help := m.options[m.cursor].Help; help != "" {
		out = append(out, hint(help))
	}
	return out
}

// scrollbar is one cell per visible row: a thumb (█) over a track (│), none when it all fits.
func scrollbar(total, visible, offset int) []string {
	if total <= visible {
		return nil
	}
	size := max(1, visible*visible/total)
	start := min(visible-size, offset*(visible-size)/max(1, total-visible))
	out := make([]string, visible)
	for i := range out {
		if i >= start && i < start+size {
			out[i] = theme.Fg(theme.Accent, "█")
		} else {
			out[i] = theme.Fg(theme.Dim, "│")
		}
	}
	return out
}
