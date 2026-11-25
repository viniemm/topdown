package folders

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatch(t *testing.T) {
	base := t.TempDir()
	for _, d := range []string{"mintern", "merk", "topdown", ".hidden"} {
		os.Mkdir(filepath.Join(base, d), 0o755)
	}
	os.WriteFile(filepath.Join(base, "mfile"), nil, 0o644)
	cands := []Folder{{Path: filepath.Join(base, "topdown"), Rank: 9}, {Path: filepath.Join(base, "mintern"), Rank: 2}}

	// Words: fuzzy over the candidates.
	if got := Match(cands, "mint", base); len(got) != 1 || filepath.Base(got[0].Path) != "mintern" {
		t.Errorf("fuzzy: %v", got)
	}
	if got := Match(cands, "", base); len(got) != 2 || filepath.Base(got[0].Path) != "topdown" {
		t.Errorf("empty keeps rank order: %v", got)
	}
	// Paths: directories completing the last component, known ones first, no files or dotdirs.
	got := Match(cands, base+"/m", base)
	if len(got) != 2 || filepath.Base(got[0].Path) != "mintern" || got[0].Rank != 2 {
		t.Errorf("path: %v", got)
	}
	if got := Match(cands, base+"/", base); len(got) != 3 {
		t.Errorf("children: %v", got)
	}
	if got := Match(cands, "./to", base); len(got) != 1 {
		t.Errorf("relative: %v", got)
	}
}

func TestExpand(t *testing.T) {
	t.Setenv("HOME", "/h")
	for q, want := range map[string]string{"~": "/h", "~/a/b": "/h/a/b", "new": "/base/new", "/x/../y": "/y", "./z": "/base/z"} {
		if got := Expand(q, "/base"); got != want {
			t.Errorf("%s: %s, want %s", q, got, want)
		}
	}
	if IsPath("mint") || !IsPath("~/x") || !IsPath("/") {
		t.Error("IsPath")
	}
}
