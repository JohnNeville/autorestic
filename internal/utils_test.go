package internal

import (
	"fmt"
	"testing"
	"time"

	"github.com/JohnNeville/autorestic/internal/flags"
	"github.com/JohnNeville/autorestic/internal/journal"
)

func TestResticLockedRe(t *testing.T) {
	cases := map[string][]string{
		"unable to create lock in backend: repository is already locked exclusively by PID 3841541 on paulrevere by root (UID 0, GID 0)\nlock was created at ...": {"3841541", "paulrevere"},
		"repository is already locked by PID 12 on host-a.lan by john (UID 1000, GID 1000)":                                                                       {"12", "host-a.lan"},
		"Fatal: wrong password or no key found": nil,
	}
	for in, want := range cases {
		m := resticLockedRe.FindStringSubmatch(in)
		if want == nil {
			if m != nil {
				t.Errorf("%q: unexpected match %v", in, m)
			}
			continue
		}
		if m == nil || m[1] != want[0] || m[2] != want[1] {
			t.Errorf("%q: got %v, want pid %s host %s", in, m, want[0], want[1])
		}
	}
}

func TestParseResticLock(t *testing.T) {
	out := "unable to create lock in backend: repository is already locked exclusively by PID 23356 on samuelwhittemore by root (UID 0, GID 0)\n" +
		"lock was created at 2026-11-16 21:24:06 (122h41m50.006972043s ago)\nstorage ID cea0f912\n"
	l, ok := parseResticLock(out)
	if !ok || l.PID != 23356 || l.Host != "samuelwhittemore" {
		t.Fatalf("got %+v %v", l, ok)
	}
	want := time.Date(2026, 11, 16, 21, 24, 6, 0, time.Local)
	if !l.Created.Equal(want) {
		t.Errorf("created %v, want %v", l.Created, want)
	}
	if _, ok := parseResticLock("Fatal: wrong password or no key found"); ok {
		t.Error("matched a non-lock error")
	}
}

func TestStaleResticLockReason(t *testing.T) {
	oldHost, oldExists, oldBoot := osHostname, processExists, bootTime
	defer func() { osHostname, processExists, bootTime = oldHost, oldExists, oldBoot }()
	boot := time.Date(2026, 11, 21, 23, 56, 0, 0, time.Local)
	osHostname = func() (string, error) { return "here", nil }
	bootTime = func() (time.Time, bool) { return boot, true }

	cases := []struct {
		name    string
		l       resticLock
		running bool
		stale   bool
	}{
		{"other host", resticLock{PID: 1, Host: "there", Created: boot.Add(-time.Hour)}, false, false},
		{"live process this boot", resticLock{PID: 1, Host: "here", Created: boot.Add(time.Hour)}, true, false},
		{"dead process this boot", resticLock{PID: 1, Host: "here", Created: boot.Add(time.Hour)}, false, true},
		{"before boot, PID reused", resticLock{PID: 1, Host: "here", Created: boot.Add(-time.Hour)}, true, true},
		{"no created time, live", resticLock{PID: 1, Host: "here"}, true, false},
	}
	for _, c := range cases {
		processExists = func(int) bool { return c.running }
		if got := staleResticLockReason(c.l) != ""; got != c.stale {
			t.Errorf("%s: stale=%v, want %v", c.name, got, c.stale)
		}
	}
}

func TestAnnounceSendsToJournalInCI(t *testing.T) {
	oldSend, oldCI := journalSend, flags.CI
	defer func() { journalSend, flags.CI = oldSend, oldCI }()

	var got []string
	journalSend = func(msg string, p journal.Priority, f map[string]string) error {
		got = append(got, fmt.Sprintf("%d %s %s", p, msg, f["AUTORESTIC_BACKEND"]))
		return nil
	}
	flags.CI = false
	Announce(journal.Err, map[string]string{"AUTORESTIC_BACKEND": "nas"}, "locked %d", 1)
	if len(got) != 0 {
		t.Fatalf("sent outside CI: %v", got)
	}
	flags.CI = true
	Announce(journal.Err, map[string]string{"AUTORESTIC_BACKEND": "nas"}, "locked %d", 1)
	if len(got) != 1 || got[0] != "3 locked 1 nas" {
		t.Errorf("got %v", got)
	}
}
