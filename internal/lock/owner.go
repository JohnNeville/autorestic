package lock

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Owner identifies the process holding the autorestic lock, so a later run can
// tell a live lock from one left behind by a process that crashed, was killed,
// or was running when the machine rebooted.
type Owner struct {
	PID       int
	Hostname  string
	BootID    string // Linux: /proc/sys/kernel/random/boot_id, "" elsewhere
	StartTime string // Linux: start time of PID from /proc/<pid>/stat, "" elsewhere
	Started   int64  // unix seconds when the lock was taken (informational)
}

func (o Owner) String() string {
	s := fmt.Sprintf("pid %d on %s", o.PID, o.Hostname)
	if o.Started > 0 {
		s += ", locked " + time.Unix(o.Started, 0).Format(time.RFC3339)
	}
	return s
}

// Seams for tests.
var (
	procRoot      = "/proc"
	processExists = defaultProcessExists
	hostname      = os.Hostname
)

func currentOwner() Owner {
	pid := os.Getpid()
	host, _ := hostname()
	return Owner{
		PID:       pid,
		Hostname:  host,
		BootID:    bootID(),
		StartTime: processStartTime(pid),
		Started:   time.Now().Unix(),
	}
}

// staleReason returns why the recorded owner can no longer be holding the
// lock, or "" if it may still be (or if that cannot be proven).
func staleReason(o Owner) string {
	if o.PID <= 0 {
		return "" // lock written by an older autorestic: no owner recorded
	}
	host, _ := hostname()
	if o.Hostname != "" && host != "" && o.Hostname != host {
		return "" // another machine (e.g. config on shared storage): can't check its processes
	}
	if o.BootID != "" {
		if cur := bootID(); cur != "" && cur != o.BootID {
			return "the system has rebooted since it was taken"
		}
	}
	if !processExists(o.PID) {
		return fmt.Sprintf("process %d no longer exists", o.PID)
	}
	if o.StartTime != "" {
		if cur := processStartTime(o.PID); cur != "" && cur != o.StartTime {
			return fmt.Sprintf("process %d is now a different process (PID reused)", o.PID)
		}
	}
	return ""
}

// ProcessExists reports whether a process with this PID exists on this host.
func ProcessExists(pid int) bool {
	return processExists(pid)
}

func defaultProcessExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	// EPERM: the process exists but belongs to another user.
	return err == nil || errors.Is(err, syscall.EPERM)
}

func bootID() string {
	b, err := os.ReadFile(procRoot + "/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// processStartTime returns field 22 (starttime, in clock ticks since boot) of
// /proc/<pid>/stat. Together with the boot ID it identifies one process
// instance, so a reused PID is not mistaken for the original owner.
func processStartTime(pid int) string {
	b, err := os.ReadFile(procRoot + "/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return ""
	}
	// The command name (field 2) is in parentheses and may contain spaces, so
	// split after the last ')'. What follows starts at field 3.
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return ""
	}
	fields := strings.Fields(s[i+1:])
	const starttime = 22 - 3
	if len(fields) <= starttime {
		return ""
	}
	return fields[starttime]
}
