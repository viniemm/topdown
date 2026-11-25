// Package proc reads the process tree from /proc.
package proc

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Parent is pid's parent (0 if unknown).
func Parent(pid int) int {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	// "pid (comm) state ppid …": comm may contain spaces, so split after the last ')'.
	s := string(stat)
	fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
	if len(fields) < 2 {
		return 0
	}
	ppid, _ := strconv.Atoi(fields[1])
	return ppid
}

// Ancestors is pid and its parents, nearest first, stopping before init.
func Ancestors(pid int) []int {
	var out []int
	for hops := 0; pid > 1 && hops < 64; hops++ {
		out = append(out, pid)
		pid = Parent(pid)
	}
	return out
}

// Argv is pid's command line.
func Argv(pid int) []string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil || len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
}

// Named lists the processes whose executable is called name.
func Named(name string) []int {
	var out []int
	dirs, _ := os.ReadDir("/proc")
	for _, d := range dirs {
		pid, err := strconv.Atoi(d.Name())
		if err != nil {
			continue
		}
		if exe, err := os.Readlink("/proc/" + d.Name() + "/exe"); err == nil && filepath.Base(exe) == name {
			out = append(out, pid)
		}
	}
	return out
}
