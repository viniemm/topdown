package agent

// Agents whose sessions live in SQLite: Hermes and Antigravity (read-only access).

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/viniemm/topdown/internal/util"
)

// query runs a read-only query; a store changing shape must never break the picker.
func query(file, q string, args []any, scan func(*sql.Rows) error) {
	if _, err := os.Stat(file); err != nil {
		return
	}
	db, err := sql.Open("sqlite", "file:"+file+"?mode=ro&_pragma=busy_timeout(1000)")
	if err != nil {
		return
	}
	defer db.Close()
	rows, err := db.Query(q, args...)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		if scan(rows) != nil {
			return
		}
	}
}

func runTool(argv ...string) error {
	out, err := exec.Command(util.UserBin(argv[0]), argv[1:]...).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("%s failed: %w", argv[0], err)
	}
	return nil
}

// only is the one session a lookup by id found.
func only(ss []Session) (Session, bool) {
	if len(ss) > 0 {
		return ss[0], true
	}
	return Session{}, false
}

// ---- Hermes ----

func hermesSessions(limit int, sid string) []Session {
	file := filepath.Join(util.Home(), ".hermes/state.db")
	where := ""
	args := []any{limit}
	if sid != "" {
		where, args = "and s.id = ?", []any{sid, 1}
	}
	q := `select s.id, s.cwd, s.title, coalesce(s.last_activity_at, s.started_at),
	  (select content from messages m where m.session_id = s.id and m.role = 'user' order by m.id limit 1)
	 from sessions s where coalesce(s.hidden,0) = 0 and coalesce(s.archived,0) = 0
	  and s.source in ('cli','tui','local') and s.message_count > 0 ` + where + ` order by 4 desc limit ?`
	var out []Session
	query(file, q, args, func(r *sql.Rows) error {
		var id string
		var cwd, title, first sql.NullString
		var ts any
		if err := r.Scan(&id, &cwd, &title, &ts, &first); err != nil {
			return err
		}
		if title.String == "" && !util.IsUserText(first.String) {
			return nil
		}
		s := Session{Agent: "hermes", ID: id, Cwd: cwd.String, Title: cmp.Or(title.String, first.String), When: util.Epoch(ts)}
		if first.String != "" {
			s.Prompts = []string{first.String}
		}
		out = append(out, s)
		return nil
	})
	return out
}

type hermesStore struct{}

func (hermesStore) List(limit int) []Session      { return hermesSessions(limit, "") }
func (hermesStore) Get(id string) (Session, bool) { return only(hermesSessions(1, id)) }
func (hermesStore) ResumeArgs(id string) []string { return []string{"--resume", id} }
func (hermesStore) Rename(s Session, title string) error {
	return runTool("hermes", "sessions", "rename", s.ID, title)
}
func (hermesStore) Delete(s Session) error {
	return runTool("hermes", "sessions", "delete", "--yes", s.ID)
}

var Hermes = &Adapter{
	ID: "hermes", Label: "Hermes", Bin: "hermes",
	// Omarchy starts Hermes in its classic REPL; --tui is Hermes' modern TUI.
	DefaultArgs: []string{"--tui"},
	LaunchArgs:  []string{"--yolo"},
	Sessions:    hermesStore{},
}

// ---- Antigravity (no delete or rename command, so those stay unsupported) ----

func agySessions(limit int, sid string) []Session {
	file := filepath.Join(util.Home(), ".gemini/antigravity-cli/conversation_summaries.db")
	where := ""
	args := []any{limit}
	if sid != "" {
		where, args = "where conversation_id = ?", []any{sid, 1}
	}
	q := `select conversation_id, title, preview, last_modified_time, workspace_uris
	 from conversation_summaries ` + where + ` order by last_modified_time desc limit ?`
	var out []Session
	query(file, q, args, func(r *sql.Rows) error {
		var id string
		var title, preview, ws sql.NullString
		var ts any
		if err := r.Scan(&id, &title, &preview, &ts, &ws); err != nil {
			return err
		}
		if title.String == "" && preview.String == "" {
			return nil
		}
		var uris []string
		cwd := ""
		if json.Unmarshal([]byte(ws.String), &uris) == nil && len(uris) > 0 {
			cwd = strings.TrimPrefix(uris[0], "file://")
		}
		s := Session{Agent: "agy", ID: id, Cwd: cwd, Title: cmp.Or(title.String, preview.String), When: util.Epoch(ts)}
		if preview.String != "" {
			s.Prompts = []string{preview.String}
		}
		out = append(out, s)
		return nil
	})
	return out
}

type agyStore struct{}

func (agyStore) List(limit int) []Session      { return agySessions(limit, "") }
func (agyStore) Get(id string) (Session, bool) { return only(agySessions(1, id)) }
func (agyStore) ResumeArgs(id string) []string { return []string{"--conversation", id} }

var Agy = &Adapter{ID: "agy", Label: "Antigravity", Bin: "agy", Sessions: agyStore{}}
