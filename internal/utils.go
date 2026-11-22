package internal

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/JohnNeville/autorestic/internal/colors"
	"github.com/JohnNeville/autorestic/internal/flags"
	"github.com/JohnNeville/autorestic/internal/journal"
	"github.com/JohnNeville/autorestic/internal/lock"
	"github.com/fatih/color"
)

func CheckIfCommandIsCallable(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func CheckIfResticIsCallable() bool {
	return CheckIfCommandIsCallable(flags.RESTIC_BIN)
}

type ExecuteOptions struct {
	Command string
	Envs    map[string]string
	Dir     string
	Silent  bool
	Backend string // backend name, for messages about it
}

type ColoredWriter struct {
	target io.Writer
	color  *color.Color
}

func (w ColoredWriter) Write(p []byte) (n int, err error) {
	colored := []byte(w.color.Sprint(string(p)))
	w.target.Write(colored)
	return len(p), nil
}

func ExecuteCommand(options ExecuteOptions, args ...string) (int, string, error) {
	cmd := exec.Command(options.Command, args...)
	env := os.Environ()
	for k, v := range options.Envs {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env
	cmd.Dir = options.Dir

	if flags.VERBOSE {
		colors.Faint.Printf("> Executing: %s\n", cmd)
	}

	var out bytes.Buffer
	var error bytes.Buffer
	if flags.VERBOSE && !options.Silent {
		var colored ColoredWriter = ColoredWriter{
			target: os.Stdout,
			color:  colors.Faint,
		}
		mw := io.MultiWriter(colored, &out)
		cmd.Stdout = mw
	} else {
		cmd.Stdout = &out
	}
	cmd.Stderr = &error
	err := cmd.Run()
	if err != nil {
		code := -1
		if exitError, ok := err.(*exec.ExitError); ok {
			code = exitError.ExitCode()
		}
		return code, error.String(), err
	}
	return 0, out.String(), nil
}

func ExecuteResticCommand(options ExecuteOptions, args ...string) (int, string, error) {
	options.Command = flags.RESTIC_BIN
	var c = GetConfig()
	var optionsAsString = getOptions(c.Global, []string{"all"})
	full := append(append([]string{}, optionsAsString...), args...)
	code, out, err := ExecuteCommand(options, full...)
	if err == nil {
		return code, out, err
	}
	l, locked := parseResticLock(out)
	if !locked {
		return code, out, err
	}

	subcommand := ""
	if len(args) > 0 {
		subcommand = args[0]
	}
	fields := map[string]string{
		"AUTORESTIC_BACKEND": options.Backend,
		"RESTIC_COMMAND":     subcommand,
		"RESTIC_LOCK_PID":    strconv.Itoa(l.PID),
		"RESTIC_LOCK_HOST":   l.Host,
	}
	where := "restic repository"
	hint := "restic unlock"
	if options.Backend != "" {
		where = fmt.Sprintf("backend \"%s\"", options.Backend)
		hint = fmt.Sprintf("autorestic exec -b %s unlock", options.Backend)
	}

	var kept string
	if reason := staleResticLockReason(l); reason == "" {
		kept = keptResticLockReason(l)
	} else {
		Announce(journal.Notice, fields, "%s: removing stale restic lock held by %s (%s)", where, l, reason)
		_, unlockOut, uerr := ExecuteCommand(options, append(append([]string{}, optionsAsString...), "unlock")...)
		if uerr != nil {
			kept = fmt.Sprintf("restic unlock failed: %s%s", strings.TrimSpace(unlockOut), uerr)
		} else {
			code, out, err = ExecuteCommand(options, full...)
			if err == nil {
				return code, out, err
			}
			l2, still := parseResticLock(out)
			if !still {
				return code, out, err
			}
			l = l2
			kept = "still locked after restic unlock"
		}
	}

	Announce(journal.Err, fields, "%s: restic %s failed: repository is locked by %s; not removed: %s. If no restic process is using the repository, clear it with: %s",
		where, subcommand, l, kept, hint)
	return code, out, err
}

// resticLock is the lock holder restic reports when a command can't lock the
// repository.
type resticLock struct {
	PID     int
	Host    string
	Created time.Time // zero if restic didn't say
}

func (l resticLock) String() string {
	s := fmt.Sprintf("PID %d on %s", l.PID, l.Host)
	if !l.Created.IsZero() {
		s += ", created " + l.Created.Format("2006-01-02 15:04:05")
	}
	return s
}

// e.g. "repository is already locked exclusively by PID 3841541 on paulrevere by root (UID 0, GID 0)"
var resticLockedRe = regexp.MustCompile(`repository is already locked (?:exclusively )?by PID (\d+) on (\S+) by `)

// e.g. "lock was created at 2026-11-16 21:24:06 (122h41m50s ago)", in local time.
var resticLockCreatedRe = regexp.MustCompile(`lock was created at (\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})`)

func parseResticLock(out string) (resticLock, bool) {
	m := resticLockedRe.FindStringSubmatch(out)
	if m == nil {
		return resticLock{}, false
	}
	pid, err := strconv.Atoi(m[1])
	if err != nil || pid <= 0 {
		return resticLock{}, false
	}
	l := resticLock{PID: pid, Host: m[2]}
	if c := resticLockCreatedRe.FindStringSubmatch(out); c != nil {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", c[1], time.Local); err == nil {
			l.Created = t
		}
	}
	return l, true
}

// Seams for tests.
var (
	osHostname    = os.Hostname
	processExists = lock.ProcessExists
	bootTime      = lock.BootTime
)

// staleResticLockReason returns why a lock held by a restic process on this
// host can no longer be in use, or "" if it may be. Locks from other hosts are
// never considered stale: their processes can't be checked from here.
func staleResticLockReason(l resticLock) string {
	host, _ := osHostname()
	if host == "" || l.Host != host {
		return ""
	}
	// Checked first: after a reboot the PID may belong to an unrelated process.
	if bt, ok := bootTime(); ok && !l.Created.IsZero() && l.Created.Before(bt) {
		return "created before this host last booted"
	}
	if !processExists(l.PID) {
		return fmt.Sprintf("process %d no longer exists", l.PID)
	}
	return ""
}

func keptResticLockReason(l resticLock) string {
	host, _ := osHostname()
	if host == "" || l.Host != host {
		return "it is held by another host"
	}
	return fmt.Sprintf("process %d is still running", l.PID)
}

func CopyFile(from, to string) error {
	original, err := os.Open(from)
	if err != nil {
		return nil
	}
	defer original.Close()

	new, err := os.Create(to)
	if err != nil {
		return nil
	}
	defer new.Close()

	if _, err := io.Copy(new, original); err != nil {
		return err
	}
	return nil
}

func CheckIfVolumeExists(volume string) bool {
	_, _, err := ExecuteCommand(ExecuteOptions{Command: "docker"}, "volume", "inspect", volume)
	return err == nil
}

func ArrayContains[T comparable](arr []T, needle T) bool {
	for _, item := range arr {
		if item == needle {
			return true
		}
	}
	return false
}
