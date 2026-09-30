package internal

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/JohnNeville/autorestic/internal/colors"
	"github.com/JohnNeville/autorestic/internal/flags"
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
	code, out, err := ExecuteCommand(options, append(optionsAsString, args...)...)
	if err != nil && clearStaleResticLock(options, optionsAsString, out) {
		return ExecuteCommand(options, append(optionsAsString, args...)...)
	}
	return code, out, err
}

// e.g. "repository is already locked exclusively by PID 3841541 on paulrevere by root (UID 0, GID 0)"
var resticLockedRe = regexp.MustCompile(`repository is already locked (?:exclusively )?by PID (\d+) on (\S+) by `)

// clearStaleResticLock handles a restic command that failed on a repository
// lock left behind by a restic process on this host that no longer exists
// (killed, crashed, or interrupted by a reboot). It runs "restic unlock", which
// only removes locks restic itself considers stale, and reports whether the
// failed command should be retried. Locks from other hosts or from a live
// process are left alone.
func clearStaleResticLock(options ExecuteOptions, globalOptions []string, out string) bool {
	m := resticLockedRe.FindStringSubmatch(out)
	if m == nil {
		return false
	}
	pid, err := strconv.Atoi(m[1])
	if err != nil || pid <= 0 {
		return false
	}
	host, _ := os.Hostname()
	if host == "" || m[2] != host || lock.ProcessExists(pid) {
		return false
	}

	colors.Secondary.Printf("Removing stale restic lock held by PID %d on %s (process no longer exists)\n", pid, host)
	_, unlockOut, err := ExecuteCommand(options, append(globalOptions, "unlock")...)
	if err != nil {
		colors.Error.Printf("restic unlock failed: %s%s\n", unlockOut, err)
		return false
	}
	return true
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
