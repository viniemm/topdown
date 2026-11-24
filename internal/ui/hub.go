package ui

// The main screen, in the layout of omp's model picker:
//
//	sidebar  Apps · Recent · one entry per agent with conversations
//	body     scope title, search line, list, detail panel
//	footer   key hints for whatever has focus

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/viniemm/topdown/internal/agent"
	"github.com/viniemm/topdown/internal/catalog"
	"github.com/viniemm/topdown/internal/config"
	"github.com/viniemm/topdown/internal/fuzzy"
	"github.com/viniemm/topdown/internal/hypr"
	"github.com/viniemm/topdown/internal/launch"
	"github.com/viniemm/topdown/internal/omarchy"
	"github.com/viniemm/topdown/internal/theme"
	"github.com/viniemm/topdown/internal/util"
)

const (
	page        = 40
	searchLimit = 300
)

type rowKind int

const (
	rowApp rowKind = iota
	rowAdd
	rowSession
	rowMore
	rowEmpty
)

type row struct {
	kind rowKind
	app  int // index into Hub.apps
	s    agent.Session
	text string
}

// Result is what the hub decided; the caller acts on it after the UI is gone.
type Result struct {
	Action  string // "quit", "launch", "resume", "focus"
	App     config.App
	Session agent.Session
	Where   launch.Where
	Extra   []string
	Address string
}

type addItem struct {
	label string
	app   *config.App
}

type modal struct {
	kind     string // "message", "confirm", "input", "add", "sheet"
	lines    []string
	onYes    func() error
	title    string
	input    textinput.Model
	onSubmit func(string) error
	items    []addItem
	cursor   int
	// launch sheet
	row     row
	agentID string
	options []agent.Option
	values  []int
}

type Options struct {
	ChooseDefault bool
}

type Hub struct {
	opts          Options
	width, height int
	apps          []config.App
	scope         string
	prevScope     *struct {
		scope  string
		cursor int
	}
	focusScope bool
	rows       []row
	cursor     int
	scroll     int
	limit      int
	more       bool
	live       map[string]int
	search     textinput.Model
	modal      *modal
	bodyRows   int
	frame      frame
	Result     Result
}

func New(opts Options) *Hub {
	h := &Hub{opts: opts, scope: "apps", limit: page, live: map[string]int{}, width: 100, height: 30,
		frame: frame{title: "topdown", minW: 16, maxW: 26}}
	h.search = textinput.New()
	h.search.Prompt = ""
	h.search.Focus()
	h.apps = config.LoadApps()
	var ids []string
	for _, a := range h.apps {
		if a.Agent != "" {
			ids = append(ids, a.Agent)
		}
	}
	theme.AssignAgents(ids)
	h.reload(false)
	return h
}

func (h *Hub) Init() tea.Cmd { return nil }

// ---------- data ----------

func (h *Hub) sessionAgents() []string {
	seen := map[string]bool{}
	var ids []string
	for _, a := range h.apps {
		if ad := agent.Get(a.Agent); ad != nil && ad.Sessions != nil && !seen[a.Agent] {
			seen[a.Agent] = true
			ids = append(ids, a.Agent)
		}
	}
	return ids
}

func (h *Hub) entries() []entry {
	es := []entry{{id: "apps", kind: "apps", label: "Apps", annotation: fmt.Sprint(len(h.apps))}, {id: "recent", kind: "recent", label: "Recent"}}
	agents := h.sessionAgents()
	if len(agents) > 0 {
		es = append(es, entry{id: "sep", kind: "separator"})
	}
	for _, id := range agents {
		ann := ""
		app, _ := config.AppFor(h.apps, id)
		if t, ok := omarchy.Tightest(omarchy.ReadUsage(omarchy.UsageID(app.Usage, id))); ok {
			ann = fmt.Sprintf("%.0f%%", t*100)
		}
		es = append(es, entry{id: "agent:" + id, kind: "agent", label: id, annotation: ann})
	}
	return es
}

func sameRow(a, b row) bool {
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case rowSession:
		return a.s.Agent == b.s.Agent && a.s.ID == b.s.ID
	case rowApp:
		return a.app == b.app
	}
	return true
}

func (h *Hub) reload(keepCursor bool) {
	query := strings.TrimSpace(h.search.Value())
	var prev *row
	if h.cursor < len(h.rows) {
		p := h.rows[h.cursor]
		prev = &p
	}
	h.rows = nil
	if h.scope == "apps" {
		idx := make([]int, len(h.apps))
		for i := range idx {
			idx[i] = i
		}
		if query != "" {
			idx = fuzzy.Rank(idx, query, func(i int) string { return h.apps[i].Label + " " + h.apps[i].Agent + h.apps[i].Command })
		}
		for _, i := range idx {
			h.rows = append(h.rows, row{kind: rowApp, app: i})
		}
		if query == "" {
			h.rows = append(h.rows, row{kind: rowAdd})
		}
	} else {
		agents := h.sessionAgents()
		if strings.HasPrefix(h.scope, "agent:") {
			agents = []string{strings.TrimPrefix(h.scope, "agent:")}
		}
		limit := h.limit
		if query != "" {
			limit = max(limit, searchLimit)
		}
		pg := catalog.Recent(agents, limit)
		h.live = agent.Live(agents...)
		sessions := pg.Sessions
		if query != "" {
			sessions = fuzzy.Rank(sessions, query, func(s agent.Session) string {
				return s.Agent + " " + util.Project(s.Cwd) + " " + s.Title
			})
		}
		h.more = query == "" && pg.More
		for _, s := range sessions {
			h.rows = append(h.rows, row{kind: rowSession, s: s})
		}
		if h.more {
			h.rows = append(h.rows, row{kind: rowMore})
		}
		if len(h.rows) == 0 {
			text := "no conversations yet"
			if query != "" {
				text = "no matches"
			}
			h.rows = append(h.rows, row{kind: rowEmpty, text: text})
		}
	}
	if keepCursor && prev != nil {
		for i, r := range h.rows {
			if sameRow(r, *prev) {
				h.cursor = i
			}
		}
	}
	h.cursor = max(0, min(h.cursor, len(h.rows)-1))
}

func (h *Hub) setScope(scope string, cursor int) {
	h.scope, h.limit, h.cursor, h.scroll = scope, page, cursor, 0
	h.search.SetValue("")
	h.reload(false)
}

func (h *Hub) current() (row, bool) {
	if h.cursor < len(h.rows) {
		return h.rows[h.cursor], true
	}
	return row{}, false
}

func (h *Hub) rowAgent(r row) string {
	switch r.kind {
	case rowApp:
		return h.apps[r.app].Agent
	case rowSession:
		return r.s.Agent
	}
	return ""
}

// ---------- input ----------

func (h *Hub) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width, h.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch {
		case h.modal != nil:
			return h, h.modalKey(msg)
		case h.focusScope:
			return h, h.scopeKey(msg.String())
		default:
			return h, h.bodyKey(msg)
		}
	case tea.PasteMsg:
		if h.modal != nil && h.modal.kind == "input" {
			var cmd tea.Cmd
			h.modal.input, cmd = h.modal.input.Update(msg)
			return h, cmd
		}
		return h, h.edit(msg)
	}
	return h, nil
}

func (h *Hub) done(r Result) tea.Cmd {
	h.Result = r
	return tea.Quit
}

func (h *Hub) scopeKey(key string) tea.Cmd {
	var es []entry
	for _, e := range h.entries() {
		if e.kind != "separator" {
			es = append(es, e)
		}
	}
	i := 0
	for j, e := range es {
		if e.id == h.scope {
			i = j
		}
	}
	switch key {
	case "up":
		if i > 0 {
			h.setScope(es[i-1].id, 0)
		}
	case "down":
		if i < len(es)-1 {
			h.setScope(es[i+1].id, 0)
		}
	case "enter", "right", "tab":
		h.focusScope = false
	case "esc", "ctrl+c":
		return h.done(Result{Action: "quit"})
	}
	return nil
}

// edit passes a key or paste to the search box and refilters when it changed.
func (h *Hub) edit(msg tea.Msg) tea.Cmd {
	before := h.search.Value()
	var cmd tea.Cmd
	h.search, cmd = h.search.Update(msg)
	if h.search.Value() != before {
		h.cursor, h.scroll = 0, 0
		h.reload(false)
	}
	return cmd
}

func (h *Hub) bodyKey(msg tea.KeyPressMsg) tea.Cmd {
	query := h.search.Value()
	r, _ := h.current()
	switch msg.String() {
	case "up":
		h.move(-1)
		return nil
	case "down":
		h.move(1)
		return nil
	case "pgup":
		h.move(-h.bodyRows)
		return nil
	case "pgdown":
		h.move(h.bodyRows)
		return nil
	case "tab":
		if h.scope == "apps" {
			h.setScope("recent", 0)
		} else {
			h.setScope("apps", 0)
		}
		return nil
	case "right":
		if query != "" {
			break
		}
		if r.kind == rowApp {
			if a := agent.Get(h.apps[r.app].Agent); a != nil && a.Sessions != nil {
				h.prevScope = &struct {
					scope  string
					cursor int
				}{h.scope, h.cursor}
				h.setScope("agent:"+a.ID, 0)
			}
		}
		return nil
	case "left":
		if strings.HasPrefix(h.scope, "agent:") && h.prevScope != nil {
			// Back out of an agent's view, search and all.
			p := *h.prevScope
			h.prevScope = nil
			h.setScope(p.scope, p.cursor)
			return nil
		}
		if query != "" {
			break
		}
		h.focusScope = true
		return nil
	case "esc":
		if query != "" {
			h.search.SetValue("")
			h.reload(false)
			return nil
		}
		if strings.HasPrefix(h.scope, "agent:") && h.prevScope != nil {
			p := *h.prevScope
			h.prevScope = nil
			h.setScope(p.scope, p.cursor)
			return nil
		}
		return h.done(Result{Action: "quit"})
	case "ctrl+c":
		return h.done(Result{Action: "quit"})
	case "enter":
		return h.activate(launch.Here, nil)
	case "alt+enter":
		return h.activate(launch.Window, nil)
	case "ctrl+o":
		h.openSheet()
		return nil
	case "ctrl+d":
		h.makeDefault()
		return nil
	case "ctrl+r":
		if id := h.rowAgent(r); id != "" {
			usage := ""
			if r.kind == rowApp {
				usage = h.apps[r.app].Usage
			}
			omarchy.RefreshUsage(omarchy.UsageID(usage, id))
		}
		return nil
	case "ctrl+a":
		h.openAdd()
		return nil
	case "ctrl+e", "f2":
		h.rename()
		return nil
	case "ctrl+x":
		h.delete()
		return nil
	case "delete":
		if query == "" {
			h.delete()
			return nil
		}
	}
	return h.edit(msg) // anything else edits the search
}

func (h *Hub) move(delta int) {
	h.cursor = max(0, min(len(h.rows)-1, h.cursor+delta))
	// Near the end of what's loaded: fetch the next page, keeping the cursor.
	if h.more && h.cursor >= len(h.rows)-4 {
		h.limit += page
		h.reload(true)
	}
}

// ---------- actions ----------

func (h *Hub) activate(where launch.Where, extra []string) tea.Cmd {
	r, ok := h.current()
	if !ok {
		return nil
	}
	switch r.kind {
	case rowAdd:
		h.openAdd()
	case rowMore:
		h.move(1)
	case rowApp:
		app := h.apps[r.app]
		if h.opts.ChooseDefault {
			if app.Agent == "" {
				h.message("Only agents can be the default agent.")
				return nil
			}
			config.SetDefault(app.Agent)
		}
		return h.done(Result{Action: "launch", App: app, Where: where, Extra: extra})
	case rowSession:
		if pid, open := h.live[r.s.ID]; open {
			// Already open: switch to its window instead of running it twice.
			if addr := hypr.WindowForPID(pid); addr != "" {
				return h.done(Result{Action: "focus", Address: addr})
			}
			h.message("That conversation is open in a terminal topdown can't switch to.")
			return nil
		}
		return h.done(Result{Action: "resume", Session: r.s, Where: where, Extra: extra})
	}
	return nil
}

func (h *Hub) message(lines ...string) { h.modal = &modal{kind: "message", lines: lines} }

func (h *Hub) makeDefault() {
	r, _ := h.current()
	if r.kind != rowApp {
		return
	}
	if h.apps[r.app].Agent == "" {
		h.message("Only agents can be the default agent.")
		return
	}
	config.SetDefault(h.apps[r.app].Agent)
}

func newInput(prompt, value string) textinput.Model {
	in := textinput.New()
	in.Prompt = prompt
	in.SetValue(value)
	in.Focus()
	return in
}

func (h *Hub) uniqueID(id string) string {
	out := id
	for n := 2; ; n++ {
		taken := false
		for _, a := range h.apps {
			if a.ID == out {
				taken = true
			}
		}
		if !taken {
			return out
		}
		out = fmt.Sprintf("%s-%d", id, n)
	}
}

func (h *Hub) openAdd() {
	listed := map[string]bool{}
	for _, a := range h.apps {
		listed[a.Agent] = true
	}
	var items []addItem
	for _, a := range agent.All {
		if !listed[a.ID] && a.Installed() {
			app := config.App{ID: a.ID, Label: a.Label, Agent: a.ID}
			items = append(items, addItem{label: a.Label + "  " + theme.Fg(theme.Dim, a.ID), app: &app})
		}
	}
	items = append(items, addItem{label: theme.Fg(theme.Accent, "Custom command…")})
	h.modal = &modal{kind: "add", items: items}
}

var nonID = regexp.MustCompile(`[^\w.-]`)

func (h *Hub) addCustom() {
	h.modal = &modal{kind: "input", title: "Command to run in a terminal (e.g. lazygit, aider --model sonnet)",
		input: newInput("command › ", ""),
		onSubmit: func(command string) error {
			c := strings.TrimSpace(command)
			if c == "" {
				return nil
			}
			id := nonID.ReplaceAllString(strings.Fields(c)[0], "")
			if id == "" {
				id = "app"
			}
			h.apps = append(h.apps, config.App{ID: h.uniqueID(id), Label: id, Command: c})
			h.reload(false)
			return config.SaveApps(h.apps)
		}}
}

func (h *Hub) rename() {
	r, _ := h.current()
	switch r.kind {
	case rowApp:
		i := r.app
		h.modal = &modal{kind: "input", title: "Rename " + h.apps[i].Label, input: newInput("name › ", h.apps[i].Label),
			onSubmit: func(v string) error {
				if v = strings.TrimSpace(v); v == "" {
					return nil
				}
				h.apps[i].Label = v
				h.reload(true)
				return config.SaveApps(h.apps)
			}}
	case rowSession:
		store := agent.Get(r.s.Agent).Sessions
		rn, ok := store.(agent.Renamer)
		if !ok {
			h.message(fmt.Sprintf("Renaming %s conversations isn't supported yet.", r.s.Agent))
			return
		}
		if _, open := h.live[r.s.ID]; open {
			h.message(fmt.Sprintf("%q is open right now; rename it there (/rename).", util.Oneline(r.s.Title, 60)))
			return
		}
		s := r.s
		h.modal = &modal{kind: "input", title: "Rename conversation", input: newInput("name › ", s.Title),
			onSubmit: func(v string) error {
				title := strings.Join(strings.Fields(v), " ")
				if title == "" || title == s.Title {
					return nil
				}
				if err := rn.Rename(s, title); err != nil {
					return err
				}
				h.reload(true)
				return nil
			}}
	}
}

func (h *Hub) delete() {
	r, _ := h.current()
	switch r.kind {
	case rowApp:
		i := r.app
		h.modal = &modal{kind: "confirm",
			lines: []string{"Remove " + theme.Bold(h.apps[i].Label) + " from the list?", theme.Fg(theme.Dim, "It stays installed.")},
			onYes: func() error {
				h.apps = append(h.apps[:i], h.apps[i+1:]...)
				h.reload(false)
				return config.SaveApps(h.apps)
			}}
	case rowSession:
		del, ok := agent.Get(r.s.Agent).Sessions.(agent.Deleter)
		if !ok {
			h.message(fmt.Sprintf("%s has no delete command, so its conversations can't be deleted here.", r.s.Agent))
			return
		}
		if _, open := h.live[r.s.ID]; open {
			h.message(fmt.Sprintf("%q is open right now.", util.Oneline(r.s.Title, 60)),
				"Close it first: a running agent writes it straight back.")
			return
		}
		s := r.s
		lines := []string{fmt.Sprintf("Delete %s (%s)?", theme.Bold(fmt.Sprintf("%q", util.Oneline(s.Title, 60))), s.Agent)}
		if util.Now()-s.When < 120 {
			lines = append(lines, theme.Fg(theme.Warning, "It changed "+util.Ago(s.When)+". If it's still open somewhere, the agent will write it back."))
		}
		where := "This uses the agent's own delete command."
		if s.Path != "" {
			where = "It goes to the Trash."
		}
		lines = append(lines, theme.Fg(theme.Dim, where))
		h.modal = &modal{kind: "confirm", lines: lines, onYes: func() error {
			if err := del.Delete(s); err != nil {
				return err
			}
			h.reload(false)
			return nil
		}}
	}
}

func (h *Hub) openSheet() {
	r, ok := h.current()
	id := h.rowAgent(r)
	a := agent.Get(id)
	if !ok || a == nil {
		h.message("No launch options for this row.")
		return
	}
	var opts []agent.Option
	for _, o := range a.Options {
		if !o.ResumeOnly || r.kind == rowSession {
			opts = append(opts, o)
		}
	}
	if len(opts) == 0 {
		h.message(fmt.Sprintf("No launch options for %s yet.", id))
		return
	}
	h.modal = &modal{kind: "sheet", row: r, agentID: id, options: opts, values: make([]int, len(opts))}
}

func sheetArgs(m *modal) []string {
	var extra []string
	for i, o := range m.options {
		if m.values[i] != 0 {
			extra = append(extra, o.Args(o.Choices[m.values[i]])...)
		}
	}
	return extra
}

func (h *Hub) modalKey(msg tea.KeyPressMsg) tea.Cmd {
	m := h.modal
	key := msg.String()
	closeModal := func() { h.modal = nil }
	fail := func(err error) {
		if err != nil {
			h.message(theme.Fg(theme.Error, err.Error()))
		}
	}
	switch m.kind {
	case "message":
		closeModal()
	case "confirm":
		switch key {
		case "y", "enter":
			closeModal()
			fail(m.onYes())
		case "n", "esc", "ctrl+c":
			closeModal()
		}
	case "input":
		switch key {
		case "esc", "ctrl+c":
			closeModal()
		case "enter":
			closeModal()
			fail(m.onSubmit(m.input.Value()))
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return cmd
		}
	case "add":
		switch key {
		case "esc", "ctrl+c":
			closeModal()
		case "up":
			m.cursor = max(0, m.cursor-1)
		case "down":
			m.cursor = min(len(m.items)-1, m.cursor+1)
		case "enter":
			it := m.items[m.cursor]
			if it.app == nil {
				h.addCustom()
				return nil
			}
			closeModal()
			app := *it.app
			app.ID = h.uniqueID(app.ID)
			h.apps = append(h.apps, app)
			h.reload(false)
			fail(config.SaveApps(h.apps))
		}
	case "sheet":
		o := m.options[m.cursor]
		switch key {
		case "esc", "ctrl+c":
			closeModal()
		case "up":
			m.cursor = max(0, m.cursor-1)
		case "down":
			m.cursor = min(len(m.options)-1, m.cursor+1)
		case "left":
			m.values[m.cursor] = (m.values[m.cursor] + len(o.Choices) - 1) % len(o.Choices)
		case "right", "space":
			m.values[m.cursor] = (m.values[m.cursor] + 1) % len(o.Choices)
		case "enter", "alt+enter":
			extra := sheetArgs(m)
			closeModal()
			where := launch.Here
			if key == "alt+enter" {
				where = launch.Window
			}
			return h.activate(where, extra)
		}
	}
	return nil
}
