// Package testhome gives tests a throwaway HOME with every XDG base dir inside
// it: the real session's XDG_* variables would otherwise reach the real
// config, cache and Trash.
package testhome

import (
	"os"
	"path/filepath"
	"testing"
)

// New creates the home inside the module (not /tmp, which gio and some
// desktops treat as a separate, untrashable mount) and points HOME and the
// XDG dirs at it for the test's duration.
func New(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	root := wd
	for root != "/" {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		root = filepath.Dir(root)
	}
	dir := filepath.Join(root, ".testhome")
	os.MkdirAll(dir, 0o755)
	home, err := os.MkdirTemp(dir, "home-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local/share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local/state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	return home
}
