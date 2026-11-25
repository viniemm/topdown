package ui

// The main screen, in the layout of omp's model picker:
//
//	sidebar  Apps · Recent · one entry per agent with conversations
//	body     scope title, search line, list, detail panel
//	footer   key hints for whatever has focus

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/viniemm/topdown/internal/agent"
	"github.com/viniemm/topdown/internal/catalog"
	"github.com/viniemm/topdown/internal/config"
	"github.com/viniemm/topdown/internal/folders"
	"github.com/viniemm/topdown/internal/fuzzy"
	"github.com/viniemm/topdown/internal/hypr"
	"github.com/viniemm/topdown/internal/launch"
	"github.com/viniemm/topdown/internal/mux"
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
	rowNew // a new conversation on an agent's page, in dir
)

type row struct {
	kind rowKind
	app  int // index into Hub.apps
	s    agent.Session
	text string
	// rowNew: where it starts, whether that folder has to be made, and what
	// topdown knows about it
	dir    string
	create bool
	folder folders.Folder
}

// Result is what the hub decided; the caller acts on it after the UI is gone.
type Result struct {
	Action  string // "quit", "launch", "resume", "focus", "attach"
	App     config.App
	Session agent.Session
	Where   launch.Where
	Extra   []string
	Cwd     string    // launch: the folder to start in
	Address string    // focus: the window
	Pane    *mux.Pane // focus/attach: the herdr pane to select first
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
	// folder picker: the choices for what's typed, and the launch to make
	choices []row
	app     int
	where   launch.Where
	extra   []string
}

type Options struct {
	ChooseDefault bool
	Version       string
}

// refreshDoneMsg: a background usage refresh finished.
type refreshDoneMsg struct{ id string }

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
	anim       animState
	refreshing map[string]bool // agent ids whose usage is being refreshed
	scene      *scene          // the header's landscape, built for its width
	cache      viewCache
	// an agent's page: the folders its New rows can start in (zoxide first)
	cands      []folders.Folder
	candsAgent string
	Result     Result
}

func New(opts Options) *Hub {
	h := &Hub{opts: opts, scope: "apps", limit: page, live: map[string]int{}, width: 100, height: 30,
		frame: frame{title: "topdown", minW: 16, maxW: 26}, refreshing: map[string]bool{}}
	h.anim.init(time.Now())
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

func (h *Hub) Init() tea.Cmd { return h.schedule() }

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
	case rowNew:
		return a.dir == b.dir
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
	h.live = agent.Live(h.sessionAgents()...) // for ● markers and the header's count
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
		if i := h.appIndex(agents[0]); strings.HasPrefix(h.scope, "agent:") && query == "" && i >= 0 {
			h.rows = append(h.rows, row{kind: rowNew, app: i, dir: config.Workdir()})
		}
		pg := catalog.Recent(agents, limit)
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
		if len(h.rows) == 0 || h.rows[len(h.rows)-1].kind == rowNew {
			text := "no conversations yet"
			if query != "" {
				text = "no conversations match"
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

// folderChoices are the folder picker's rows: the workdir and the most-used
// folders, or, for what's typed, the folders that match (zoxide's, the agent's
// projects, or a typed path), then a new folder when it isn't one yet.
func (h *Hub) folderChoices(i int, query string) []row {
	agentID := h.apps[i].Agent
	work := config.Workdir()
	if h.candsAgent != agentID {
		convs := map[string]int{}
		for _, s := range catalog.Recent([]string{agentID}, 300).Sessions {
			convs[s.Cwd]++
		}
		h.cands, h.candsAgent = folders.Candidates(convs), agentID
	}
	var out []row
	seen := map[string]bool{}
	add := func(f folders.Folder) {
		if !seen[f.Path] && len(out) < folderChoicesMax {
			seen[f.Path] = true
			out = append(out, row{kind: rowNew, app: i, dir: f.Path, folder: f})
		}
	}
	if query == "" {
		add(h.knownFolder(work))
		for _, f := range h.cands {
			add(f)
		}
		return out
	}
	// A typed path that's already a folder comes first, then what it completes to.
	if p := folders.Expand(query, work); folders.IsPath(query) && folders.Exists(p) {
		add(h.knownFolder(p))
	}
	for _, f := range folders.Match(h.cands, query, work) {
		add(f)
	}
	if p := folders.Expand(query, work); !folders.Exists(p) && validName(query) {
		out = append(out, row{kind: rowNew, app: i, dir: p, create: true})
	}
	return out
}

// folderChoicesMax is how many folders the picker offers.
const folderChoicesMax = 8

// knownFolder is p with what topdown knows about it (zoxide's score, conversations).
func (h *Hub) knownFolder(p string) folders.Folder {
	for _, c := range h.cands {
		if c.Path == p {
			return c
		}
	}
	return folders.Folder{Path: p}
}

// openFolders is the folder picker for a new conversation of app i.
func (h *Hub) openFolders(i int, where launch.Where, extra []string) {
	m := &modal{kind: "folders", app: i, where: where, extra: extra,
		input: newInput("", "")}
	m.input.Placeholder = "search folders (zoxide), a path, or a new name"
	m.choices = h.folderChoices(i, "")
	h.modal = m
}

// startNew starts a new conversation in a folder row, making the folder first if needed.
func (h *Hub) startNew(r row, where launch.Where, extra []string) tea.Cmd {
	if r.create {
		if err := os.MkdirAll(r.dir, 0o755); err != nil {
			h.message(theme.Fg(theme.Error, err.Error()))
			return nil
		}
	}
	folders.Added(r.dir)
	return h.done(Result{Action: "launch", App: h.apps[r.app], Where: where, Extra: extra, Cwd: r.dir})
}

// validName: what's typed could be a folder to make (not a search with
// characters no one puts in a folder name).
func validName(q string) bool {
	return strings.TrimSpace(q) == q && !strings.ContainsAny(q, "*?\"|<>:\x00")
}

// appIndex is the app that runs agentID (-1: none in the list).
func (h *Hub) appIndex(agentID string) int {
	for i, a := range h.apps {
		if a.Agent == agentID {
			return i
		}
	}
	return -1
}

// openPage shows an agent's page: New conversation on top, its conversations below.
func (h *Hub) openPage(app int) {
	h.prevScope = &struct {
		scope  string
		cursor int
	}{h.scope, h.cursor}
	h.setScope("agent:"+h.apps[app].Agent, 0)
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
	case rowApp, rowNew:
		return h.apps[r.app].Agent
	case rowSession:
		return r.s.Agent
	}
	return ""
}

// ---------- input ----------

func (h *Hub) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tickMsg); !ok {
		h.cache.ok = false // anything but a frame tick may change what is below the band
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width, h.height = msg.Width, msg.Height
		// The landscape may have come into view; the next frame says if not.
		h.anim.scenery = theme.Animated()
		return h, h.touch()
	case tickMsg:
		return h, h.onTick(msg)
	case refreshDoneMsg:
		delete(h.refreshing, msg.id)
		return h, h.touch()
	case tea.KeyPressMsg:
		scope, cursor := h.scope, h.cursor
		var cmd tea.Cmd
		switch {
		case h.modal != nil:
			cmd = h.modalKey(msg)
		case h.focusScope:
			cmd = h.scopeKey(msg.String())
		default:
			cmd = h.bodyKey(msg)
		}
		if h.scope != scope || h.cursor != cursor {
			h.anim.selAt = time.Now() // the selection bar fades in at its new row
		}
		return h, tea.Batch(cmd, h.touch())
	case tea.PasteMsg:
		if m := h.modal; m != nil && (m.kind == "input" || m.kind == "folders") {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			if m.kind == "folders" {
				m.choices, m.cursor = h.folderChoices(m.app, strings.TrimSpace(m.input.Value())), 0
			}
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
		// → is Enter everywhere in the list (← goes back), except while it
		// can still move the cursor through what's typed in the search.
		if h.search.Position() < len([]rune(query)) {
			break
		}
		return h.activate(launch.Here, nil)
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
		// Refreshing can take seconds: run it in the background with a spinner.
		if id := h.rowAgent(r); id != "" && !h.refreshing[id] {
			usage := ""
			if r.kind == rowApp {
				usage = h.apps[r.app].Usage
			}
			h.refreshing[id] = true
			uid := omarchy.UsageID(usage, id)
			return func() tea.Msg {
				omarchy.RefreshUsage(uid)
				return refreshDoneMsg{id: id}
			}
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
		} else if app.Agent != "" && where == launch.Here && extra == nil {
			// An agent: its page, to pick a folder or a conversation.
			// (Alt-Enter and the launch sheet still start it right away.)
			h.openPage(r.app)
			return nil
		}
		return h.done(Result{Action: "launch", App: app, Where: where, Extra: extra})
	case rowNew:
		h.openFolders(r.app, where, extra)
	case rowSession:
		if pid, open := h.live[r.s.ID]; open {
			// Already open: switch to it instead of running it twice.
			if p, ok := mux.Find(pid); ok {
				// Inside herdr: select its pane, then show a client that's
				// already attached, or attach one.
				for _, c := range p.Clients() {
					if addr := hypr.WindowForPID(c); addr != "" {
						return h.done(Result{Action: "focus", Address: addr, Pane: &p})
					}
				}
				return h.done(Result{Action: "attach", Pane: &p, Where: where})
			}
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
	case "folders":
		switch key {
		case "esc", "ctrl+c":
			closeModal()
		case "up":
			m.cursor = max(0, m.cursor-1)
		case "down":
			m.cursor = min(len(m.choices)-1, m.cursor+1)
		case "tab":
			// Complete to the highlighted folder and look inside it, like a shell.
			if m.cursor < len(m.choices) && !m.choices[m.cursor].create {
				m.input.SetValue(util.ShortPath(m.choices[m.cursor].dir) + "/")
				m.input.CursorEnd()
				m.choices, m.cursor = h.folderChoices(m.app, m.input.Value()), 0
			}
		case "enter", "alt+enter":
			if m.cursor >= len(m.choices) {
				return nil
			}
			where := m.where
			if key == "alt+enter" {
				where = launch.Window
			}
			r := m.choices[m.cursor]
			closeModal()
			return h.startNew(r, where, m.extra)
		default:
			before := m.input.Value()
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			if v := m.input.Value(); v != before {
				m.choices, m.cursor = h.folderChoices(m.app, strings.TrimSpace(v)), 0
			}
			return cmd
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
			// Non-nil, even with every option at its default: from the
			// sheet, an app starts right away rather than opening its page.
			extra := append([]string{}, sheetArgs(m)...)
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
