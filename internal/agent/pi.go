package agent

import (
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/viniemm/topdown/internal/util"
)

// parsePi reads a Pi / Oh My Pi session (JSONL entries forming a tree). A name
// is the latest session_info entry (Pi's /name); omp may write a "title" entry.
func parsePi(file string) Parsed {
	var cwd, title, name string
	var prompts []string
	eachLine(file, func(line string, n int) bool {
		if n >= 300 && !strings.Contains(line, `"session_info"`) {
			return true // past the start, only renames matter
		}
		var d struct {
			Type    string `json:"type"`
			Name    string `json:"name"`
			Title   string `json:"title"`
			Cwd     string `json:"cwd"`
			Message struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &d) != nil {
			return true
		}
		switch d.Type {
		case "session_info":
			if d.Name != "" {
				name = d.Name
			}
		case "title":
			title = d.Title
		case "session":
			cwd = d.Cwd
		case "message":
			if t := util.TextOf(d.Message.Content); d.Message.Role == "user" && len(prompts) < 5 && util.IsUserText(t) {
				prompts = append(prompts, util.Clip(strings.TrimSpace(t), 600))
			}
		}
		return true
	})
	return Parsed{Title: cmp.Or(name, title, first(prompts)), Cwd: cwd, Prompts: prompts}
}

// renamePi appends a session_info entry on the current leaf (the last entry), as Pi's /name writes.
func renamePi(file, title string) error {
	var last any
	eachLine(file, func(line string, _ int) bool {
		if strings.Contains(line, `"id"`) {
			var d struct {
				ID string `json:"id"`
			}
			if json.Unmarshal([]byte(line), &d) == nil && d.ID != "" {
				last = d.ID
			}
		}
		return true
	})
	id := make([]byte, 4)
	rand.Read(id)
	return appendJSONL(file, []kv{{"type", "session_info"}, {"id", hex.EncodeToString(id)}, {"parentId", last},
		{"timestamp", time.Now().UTC().Format("2006-01-02T15:04:05.000Z")}, {"name", title}})
}

type piStore struct {
	fileStore
	resumeFlag string
}

func (p piStore) ResumeArgs(id string) []string { return []string{p.resumeFlag, id} }

func (p piStore) Delete(s Session) error {
	file := p.fileOf(s)
	if _, err := os.Stat(file); err != nil {
		return errors.New("that conversation is already gone")
	}
	return Trash(file)
}

// piRenamer adds rename to Pi only: omp's on-disk naming isn't confirmed yet.
type piRenamer struct{ piStore }

func (p piRenamer) Rename(s Session, title string) error { return renamePi(p.fileOf(s), title) }

func piFiles(id, dir string) fileStore {
	return fileStore{agent: id, pattern: dir + "/agent/sessions/*/*.jsonl", parse: parsePi, byPath: true}
}

var (
	Pi  = &Adapter{ID: "pi", Label: "Pi", Bin: "pi", Sessions: piRenamer{piStore{piFiles("pi", ".pi"), "--session"}}}
	Omp = &Adapter{ID: "omp", Label: "Oh My Pi", Bin: "omp", Sessions: piStore{piFiles("omp", ".omp"), "--resume"}}
)
