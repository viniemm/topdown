package agent

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/viniemm/topdown/internal/util"
)

// Trash moves files to the home Trash (restorable from any file manager),
// following the freedesktop.org Trash spec directly: no gvfs daemon needed,
// and it uses this process's own $XDG_DATA_HOME. Files on another filesystem
// go through `gio trash`, which knows the per-mount .Trash-$UID folders.
func Trash(files ...string) error {
	bin := filepath.Join(util.DataHome(), "Trash")
	for _, d := range []string{"files", "info"} {
		if err := os.MkdirAll(filepath.Join(bin, d), 0o700); err != nil {
			return err
		}
	}
	for _, file := range files {
		if err := trashOne(bin, file); err != nil {
			return err
		}
	}
	return nil
}

func trashOne(bin, file string) error {
	abs, err := filepath.Abs(file)
	if err != nil {
		return err
	}
	base := filepath.Base(abs)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for n := 1; ; n++ {
		name := base
		if n > 1 {
			name = fmt.Sprintf("%s.%d%s", stem, n, ext)
		}
		info := filepath.Join(bin, "info", name+".trashinfo")
		fh, err := os.OpenFile(info, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // claims the name atomically
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		parts := strings.Split(abs, "/")
		for i, p := range parts {
			parts[i] = url.PathEscape(p)
		}
		fmt.Fprintf(fh, "[Trash Info]\nPath=%s\nDeletionDate=%s\n", strings.Join(parts, "/"), time.Now().Format("2006-01-02T15:04:05"))
		fh.Close()
		if err := os.Rename(abs, filepath.Join(bin, "files", name)); err != nil {
			os.Remove(info)
			if !errors.Is(err, syscall.EXDEV) {
				return err
			}
			if out, err := exec.Command("gio", "trash", abs).CombinedOutput(); err != nil {
				return fmt.Errorf("gio trash failed: %s", strings.TrimSpace(string(out)))
			}
		}
		return nil
	}
}
