package lock

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// fakeProc points procRoot at a temp dir with a boot_id and per-PID stat files,
// and stubs hostname/processExists. Restores everything on cleanup.
func fakeProc(t *testing.T, boot string, host string, live map[int]string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sys/kernel/random"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sys/kernel/random/boot_id"), []byte(boot+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for pid, start := range live {
		dir := filepath.Join(root, strconv.Itoa(pid))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Real layout: pid (comm) state ppid ... ; field 22 is starttime.
		// The comm deliberately contains ") " to exercise the last-')' parsing.
		stat := strconv.Itoa(pid) + " (auto) restic) S 1 1 1 0 -1 4194560 0 0 0 0 0 0 0 0 20 0 1 0 " + start + " 0 0\n"
		if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	oldRoot, oldExists, oldHost := procRoot, processExists, hostname
	procRoot = root
	processExists = func(pid int) bool { _, ok := live[pid]; return ok }
	hostname = func() (string, error) { return host, nil }
	t.Cleanup(func() { procRoot, processExists, hostname = oldRoot, oldExists, oldHost })
}

func TestProcessStartTime(t *testing.T) {
	fakeProc(t, "boot-a", "h", map[int]string{42: "123456"})
	if got := processStartTime(42); got != "123456" {
		t.Errorf("processStartTime(42) = %q, want %q", got, "123456")
	}
	if got := processStartTime(43); got != "" {
		t.Errorf("processStartTime(missing) = %q, want empty", got)
	}
}

func TestStaleReason(t *testing.T) {
	live := map[int]string{100: "5000"}
	owner := Owner{PID: 100, Hostname: "h", BootID: "boot-a", StartTime: "5000"}

	cases := []struct {
		name  string
		boot  string
		host  string
		owner Owner
		stale bool
	}{
		{"live owner", "boot-a", "h", owner, false},
		{"process gone", "boot-a", "h", Owner{PID: 101, Hostname: "h", BootID: "boot-a", StartTime: "5000"}, true},
		{"rebooted", "boot-b", "h", owner, true},
		{"pid reused", "boot-a", "h", Owner{PID: 100, Hostname: "h", BootID: "boot-a", StartTime: "4999"}, true},
		{"legacy lock without owner", "boot-a", "h", Owner{}, false},
		{"other host", "boot-b", "h", Owner{PID: 101, Hostname: "other", BootID: "boot-a"}, false},
		{"no boot/start info, process gone", "boot-a", "h", Owner{PID: 101, Hostname: "h"}, true},
		{"no boot/start info, process alive", "boot-a", "h", Owner{PID: 100, Hostname: "h"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeProc(t, c.boot, c.host, live)
			reason := staleReason(c.owner)
			if (reason != "") != c.stale {
				t.Errorf("staleReason(%+v) = %q, want stale=%v", c.owner, reason, c.stale)
			}
		})
	}
}

func TestDefaultProcessExists(t *testing.T) {
	if !defaultProcessExists(os.Getpid()) {
		t.Error("own process reported as not existing")
	}
	// A child that has exited and been reaped no longer exists.
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Skip("cannot run 'true':", err)
	}
	if defaultProcessExists(cmd.Process.Pid) {
		t.Errorf("reaped child %d reported as existing", cmd.Process.Pid)
	}
}
