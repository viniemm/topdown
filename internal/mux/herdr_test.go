package mux

import "testing"

func TestClientSession(t *testing.T) {
	for _, c := range []struct {
		argv []string
		want string
	}{
		{[]string{"herdr"}, "default"},
		{[]string{"/usr/bin/herdr", "--session", "work"}, "work"},
		{[]string{"herdr", "session", "attach", "notes"}, "notes"},
		{[]string{"herdr", "--remote", "box"}, "default"},
	} {
		if got := clientSession(c.argv); got != c.want {
			t.Errorf("%v: got %q, want %q", c.argv, got, c.want)
		}
	}
	if a := (Pane{Session: "work"}).AttachArgv(); len(a) != 4 || a[3] != "work" {
		t.Errorf("attach argv %v", a)
	}
}
