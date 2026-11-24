package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/viniemm/topdown/internal/util"
)

// ParseClaude reads a Claude Code transcript: title (custom > ai > first real
// prompt), latest working directory, first prompts.
func ParseClaude(file string) Parsed {
	var cwd, custom, ai string
	var prompts []string
	eachLine(file, func(line string, _ int) bool {
		// Cheap substring checks first; only promising lines are JSON-decoded.
		if i := strings.Index(line, `"cwd":"`); i != -1 {
			if j := strings.IndexByte(line[i+7:], '"'); j != -1 {
				cwd = line[i+7 : i+7+j] // the latest wins: a moved session resumes where it lives now
			}
		}
		isTitle := strings.Contains(line, `"type":"custom-title"`) || strings.Contains(line, `"type":"ai-title"`)
		if !isTitle && !(len(prompts) < 5 && strings.Contains(line, `"type":"user"`)) {
			return true
		}
		var d struct {
			Type        string `json:"type"`
			CustomTitle string `json:"customTitle"`
			AITitle     string `json:"aiTitle"`
			IsMeta      bool   `json:"isMeta"`
			Message     struct {
				Content any `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &d) != nil {
			return true
		}
		switch d.Type {
		case "custom-title":
			if d.CustomTitle != "" {
				custom = d.CustomTitle
			}
		case "ai-title":
			if d.AITitle != "" {
				ai = d.AITitle
			}
		case "user":
			if t := util.TextOf(d.Message.Content); !d.IsMeta && util.IsUserText(t) {
				prompts = append(prompts, clip(strings.TrimSpace(t), 600))
			}
		}
		return true
	})
	title := custom
	if title == "" {
		title = ai
	}
	if title == "" && len(prompts) > 0 {
		title = prompts[0]
	}
	return Parsed{Title: title, Cwd: cwd, Prompts: prompts}
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

type claudeStore struct{ files fileStore }

var claudeFiles = fileStore{agent: "claude", pattern: ".claude/projects/*/*.jsonl", parse: ParseClaude}

func (c claudeStore) List(limit int) []Session      { return c.files.list(limit) }
func (c claudeStore) Get(id string) (Session, bool) { return c.files.get(id) }
func (c claudeStore) ResumeArgs(id string) []string { return []string{"--resume", id} }
func (c claudeStore) Live() map[string]int          { return ClaudeLive() }

// Rename appends what Claude's own /rename writes.
func (c claudeStore) Rename(s Session, title string) error {
	file := s.Path
	if file == "" {
		file = c.files.fileFor(s.ID)
	}
	return appendJSONL(file, []kv{{"type", "custom-title"}, {"customTitle", title}, {"sessionId", s.ID}})
}

// Delete moves the transcript, and the sibling folder of subagent transcripts, to the Trash.
func (c claudeStore) Delete(s Session) error {
	file := s.Path
	if file == "" {
		file = c.files.fileFor(s.ID)
	}
	if file == "" {
		return errors.New("that conversation is already gone")
	}
	files := []string{file}
	if side := strings.TrimSuffix(file, ".jsonl"); isDir(side) {
		files = append(files, side)
	}
	return Trash(files...)
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// ClaudeLive lists running sessions: Claude Code records each in ~/.claude/sessions/<pid>.json.
func ClaudeLive() map[string]int {
	live := map[string]int{}
	files, _ := filepath.Glob(filepath.Join(util.Home(), ".claude/sessions/*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var rec struct {
			PID       json.Number `json:"pid"`
			SessionID string      `json:"sessionId"`
		}
		if json.Unmarshal(b, &rec) != nil || rec.SessionID == "" {
			continue
		}
		pid, err := strconv.Atoi(rec.PID.String())
		if err != nil || pid <= 0 || syscall.Kill(pid, 0) != nil { // signal 0: is it alive?
			continue
		}
		live[rec.SessionID] = pid
	}
	return live
}

// Claude Code flags (from `claude --help`, 2.1.330). The first choice is Claude's default.
var claudeOptions = []Option{
	{Key: "model", Label: "Model", Choices: []string{"default", "opus", "sonnet", "haiku", "fable"},
		Help: "--model (alias or full id)", Args: func(v string) []string { return []string{"--model", v} }},
	{Key: "effort", Label: "Effort", Choices: []string{"default", "low", "medium", "high", "xhigh", "max"},
		Help: "--effort", Args: func(v string) []string { return []string{"--effort", v} }},
	{Key: "permission", Label: "Permissions",
		Choices: []string{"default", "auto", "manual", "acceptEdits", "plan", "dontAsk", "bypassPermissions"},
		Help:    "--permission-mode", Args: func(v string) []string { return []string{"--permission-mode", v} }},
	{Key: "fork", Label: "On resume", Choices: []string{"continue", "fork"}, ResumeOnly: true,
		Help: "--fork-session: resume into a new session id",
		Args: func(v string) []string {
			if v == "fork" {
				return []string{"--fork-session"}
			}
			return nil
		}},
	{Key: "chrome", Label: "Chrome", Choices: []string{"default", "on", "off"}, Help: "--chrome / --no-chrome",
		Args: func(v string) []string {
			if v == "on" {
				return []string{"--chrome"}
			}
			return []string{"--no-chrome"}
		}},
}

var Claude = &Adapter{
	ID: "claude", Label: "Claude Code", Bin: "claude",
	// omarchy-agent adds this too; without Omarchy topdown adds it itself.
	LaunchArgs: []string{"--permission-mode", "auto"},
	Options:    claudeOptions,
	Sessions:   claudeStore{claudeFiles},
}
