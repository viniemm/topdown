package agent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/viniemm/topdown/internal/agent"
	"github.com/viniemm/topdown/internal/catalog"
	"github.com/viniemm/topdown/internal/testhome"
)

func write(t *testing.T, home, rel string, lines []any, ageMin int) string {
	t.Helper()
	f := filepath.Join(home, rel)
	os.MkdirAll(filepath.Dir(f), 0o755)
	var b strings.Builder
	for _, l := range lines {
		j, _ := json.Marshal(l)
		b.Write(j)
		b.WriteByte('\n')
	}
	os.WriteFile(f, []byte(b.String()), 0o644)
	at := time.Now().Add(-time.Duration(ageMin) * time.Minute)
	os.Chtimes(f, at, at)
	return f
}

func user(text string) map[string]any {
	return map[string]any{"type": "user", "cwd": "/w/a", "message": map[string]any{"role": "user", "content": text}}
}

func fixture(t *testing.T) string {
	home := testhome.New(t)
	agent.ResetCache()
	write(t, home, ".claude/projects/-w-a/c1.jsonl", []any{user("newest prompt")}, 1)
	write(t, home, ".claude/projects/-w-a/c2.jsonl", []any{user("<command-name>/clear</command-name>"), user("real prompt")}, 10)
	write(t, home, ".claude/projects/-w-b/c3.jsonl", []any{user("old"), map[string]any{"type": "ai-title", "aiTitle": "AI title"}}, 60)
	write(t, home, ".claude/projects/-w-b/c4.jsonl", []any{user("x"), map[string]any{"type": "ai-title", "aiTitle": "AI"},
		map[string]any{"type": "custom-title", "customTitle": "Mine"}}, 120)
	write(t, home, ".pi/agent/sessions/--w--/2026_p1.jsonl", []any{
		map[string]any{"type": "session", "id": "p1", "cwd": "/w/p"},
		map[string]any{"type": "message", "id": "aa11bb22", "parentId": nil, "message": map[string]any{"role": "user", "content": "pi prompt"}},
	}, 5)
	return home
}

func TestNewestFirstWithPaging(t *testing.T) {
	fixture(t)
	page := catalog.Recent([]string{"claude", "pi"}, 3)
	var ids []string
	for _, s := range page.Sessions {
		ids = append(ids, filepath.Base(s.ID))
	}
	if !slices.Equal(ids, []string{"c1", "2026_p1.jsonl", "c2"}) || !page.More {
		t.Fatalf("got %v more=%v", ids, page.More)
	}
	if catalog.Recent([]string{"claude", "pi"}, 10).More {
		t.Fatal("no more expected")
	}
}

func TestTitlePrecedence(t *testing.T) {
	fixture(t)
	got := map[string]string{}
	for _, s := range agent.Claude.Sessions.List(10) {
		got[s.ID] = s.Title
	}
	want := map[string]string{"c1": "newest prompt", "c2": "real prompt", "c3": "AI title", "c4": "Mine"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q want %q", k, got[k], v)
		}
	}
}

func lastLine(t *testing.T, f string) map[string]any {
	b, _ := os.ReadFile(f)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestClaudeRename(t *testing.T) {
	fixture(t)
	s, _ := agent.Claude.Sessions.Get("c1")
	if err := agent.Claude.Sessions.(agent.Renamer).Rename(s, "Renamed"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(s.Path)
	if !strings.HasSuffix(string(b), `{"type":"custom-title","customTitle":"Renamed","sessionId":"c1"}`+"\n") {
		t.Fatalf("last line: %q", string(b))
	}
	if s2, _ := agent.Claude.Sessions.Get("c1"); s2.Title != "Renamed" {
		t.Fatalf("title %q", s2.Title)
	}
}

func TestPiRename(t *testing.T) {
	fixture(t)
	s := agent.Pi.Sessions.List(1)[0]
	if err := agent.Pi.Sessions.(agent.Renamer).Rename(s, "Pi name"); err != nil {
		t.Fatal(err)
	}
	m := lastLine(t, s.Path)
	if m["type"] != "session_info" || m["parentId"] != "aa11bb22" || m["name"] != "Pi name" {
		t.Fatalf("entry %v", m)
	}
	if s2, _ := agent.Pi.Sessions.Get(s.ID); s2.Title != "Pi name" {
		t.Fatalf("title %q", s2.Title)
	}
	if _, ok := agent.Omp.Sessions.(agent.Renamer); ok {
		t.Fatal("omp rename should stay unsupported")
	}
}

func TestClaudeLive(t *testing.T) {
	home := fixture(t)
	write(t, home, ".claude/sessions/1.json", []any{map[string]any{"pid": os.Getpid(), "sessionId": "c2"}}, 0)
	write(t, home, ".claude/sessions/2.json", []any{map[string]any{"pid": 1 << 22, "sessionId": "c3"}}, 0) // no such process
	live := agent.Live("claude")
	if live["c2"] != os.Getpid() {
		t.Fatalf("live %v", live)
	}
	if _, ok := live["c3"]; ok {
		t.Fatal("dead pid counted as live")
	}
}

func TestDeleteToTrash(t *testing.T) {
	home := fixture(t)
	s, _ := agent.Claude.Sessions.Get("c4")
	os.MkdirAll(strings.TrimSuffix(s.Path, ".jsonl"), 0o755)
	if err := agent.Claude.Sessions.(agent.Deleter).Delete(s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Path); err == nil {
		t.Fatal("still there")
	}
	entries, _ := os.ReadDir(filepath.Join(home, ".local/share/Trash/files"))
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{"c4", "c4.jsonl"}) {
		t.Fatalf("trash: %v", names)
	}
	info, _ := os.ReadFile(filepath.Join(home, ".local/share/Trash/info/c4.jsonl.trashinfo"))
	if !strings.Contains(string(info), "Path="+s.Path) {
		t.Fatalf("trashinfo: %s", info)
	}
}
