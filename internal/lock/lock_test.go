package lock

import (
	"log"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/spf13/viper"
)

var testDirectory = "autorestic_test_tmp"

// All tests must share the same lock file as it is only initialized once
func setup(t *testing.T) {
	d, err := os.MkdirTemp("", testDirectory)
	if err != nil {
		log.Fatalf("error creating temp dir: %v", err)
		return
	}
	// set config file location
	viper.SetConfigFile(d + "/.autorestic.yml")

	t.Cleanup(func() {
		os.RemoveAll(d)
		viper.Reset()
	})
}

func TestLock(t *testing.T) {
	setup(t)

	t.Run("getLock", func(t *testing.T) {
		result := getLock().GetBool(RUNNING)

		if result {
			t.Errorf("got %v, want %v", result, false)
		}
	})

	t.Run("lock", func(t *testing.T) {
		lock, err := setLockValue(RUNNING, true)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		result := lock.GetBool(RUNNING)
		if !result {
			t.Errorf("got %v, want %v", result, true)
		}
	})

	t.Run("unlock", func(t *testing.T) {
		lock, err := setLockValue(RUNNING, false)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		result := lock.GetBool(RUNNING)
		if result {
			t.Errorf("got %v, want %v", result, false)
		}
	})

	// locking a locked instance exits the instance
	// this trick to capture os.Exit(1) is discussed here:
	// https://talks.golang.org/2014/testing.slide#23
	t.Run("lock twice", func(t *testing.T) {
		if os.Getenv("CRASH") == "1" {
			err := Lock()
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			// should fail
			Lock()
		}

		cmd := exec.Command(os.Args[0], "-test.run=TestLock/lock_twice")
		cmd.Env = append(os.Environ(), "CRASH=1")
		err := cmd.Run()

		err, ok := err.(*exec.ExitError)
		if !ok {
			t.Error("unexpected error")
		}
		expected := "exit status 1"
		if err.Error() != expected {
			t.Errorf("got %q, want %q", err.Error(), expected)
		}
	})

	t.Run("lock records owner", func(t *testing.T) {
		if _, err := setLockValue(RUNNING, true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := getOwner().PID; got != os.Getpid() {
			t.Errorf("owner pid = %d, want %d", got, os.Getpid())
		}
		if err := Unlock(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if getLock().GetBool(RUNNING) || getOwner().PID != 0 {
			t.Errorf("own lock not released: running=%v owner=%+v", getLock().GetBool(RUNNING), getOwner())
		}
	})

	t.Run("unlock does not release another live owner", func(t *testing.T) {
		fakeProc(t, "boot-a", "h", map[int]string{os.Getpid() + 1: "1"})
		getLock().Set(RUNNING, true)
		setOwner(Owner{PID: os.Getpid() + 1, Hostname: "h", BootID: "boot-a", StartTime: "1"})
		if err := Unlock(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !getLock().GetBool(RUNNING) {
			t.Error("Unlock released a lock held by another process")
		}
		if err := ForceUnlock(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if getLock().GetBool(RUNNING) || getOwner().PID != 0 {
			t.Error("ForceUnlock did not release the lock")
		}
	})

	t.Run("lock takes over a stale lock", func(t *testing.T) {
		fakeProc(t, "boot-b", "h", map[int]string{}) // owner gone, and rebooted
		getLock().Set(RUNNING, true)
		setOwner(Owner{PID: 999999, Hostname: "h", BootID: "boot-a", StartTime: "1"})
		if err := Lock(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := getOwner().PID; got != os.Getpid() {
			t.Errorf("owner pid = %d, want %d (stale lock not taken over)", got, os.Getpid())
		}
		if err := Unlock(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// A live owner (or a legacy lock with no owner recorded) still makes a
	// second instance exit, as before.
	t.Run("lock held by live owner exits", func(t *testing.T) {
		if os.Getenv("CRASH_LIVE") == "1" {
			fakeProc(t, "boot-a", "h", map[int]string{os.Getpid() + 1: "1"})
			getLock().Set(RUNNING, true)
			setOwner(Owner{PID: os.Getpid() + 1, Hostname: "h", BootID: "boot-a", StartTime: "1"})
			Lock() // should exit 1
			return
		}
		cmd := exec.Command(os.Args[0], "-test.run=TestLock/lock_held_by_live_owner_exits")
		cmd.Env = append(os.Environ(), "CRASH_LIVE=1")
		err, ok := cmd.Run().(*exec.ExitError)
		if !ok || err.Error() != "exit status 1" {
			t.Errorf("got %v, want exit status 1", err)
		}
	})

	t.Run("legacy lock without owner exits", func(t *testing.T) {
		if os.Getenv("CRASH_LEGACY") == "1" {
			getLock().Set(RUNNING, true)
			clearOwner()
			Lock() // should exit 1
			return
		}
		cmd := exec.Command(os.Args[0], "-test.run=TestLock/legacy_lock_without_owner_exits")
		cmd.Env = append(os.Environ(), "CRASH_LEGACY=1")
		err, ok := cmd.Run().(*exec.ExitError)
		if !ok || err.Error() != "exit status 1" {
			t.Errorf("got %v, want exit status 1", err)
		}
	})

	t.Run("set cron", func(t *testing.T) {
		expected := int64(5)
		SetCron("foo", expected)

		result, err := strconv.ParseInt(getLock().GetString("cron.foo"), 10, 64)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != expected {
			t.Errorf("got %d, want %d", result, expected)
		}
	})

	t.Run("get cron", func(t *testing.T) {
		expected := int64(5)
		result := GetCron("foo")

		if result != expected {
			t.Errorf("got %d, want %d", result, expected)
		}
	})
}
