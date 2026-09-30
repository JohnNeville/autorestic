package lock

import (
	"os"
	"path"
	"sync"

	"github.com/JohnNeville/autorestic/internal/colors"
	"github.com/JohnNeville/autorestic/internal/flags"
	"github.com/spf13/viper"
)

var lock *viper.Viper
var file string
var once sync.Once

const (
	RUNNING = "running"

	// Owner of the running lock (see owner.go).
	ownerPID       = "owner.pid"
	ownerHostname  = "owner.hostname"
	ownerBootID    = "owner.boot_id"
	ownerStartTime = "owner.start_time"
	ownerStarted   = "owner.started"
)

func getLock() *viper.Viper {
	if lock == nil {

		once.Do(func() {
			lock = viper.New()
			lock.SetDefault("running", false)
			p := viper.ConfigFileUsed()
			if p == "" {
				colors.Error.Println("cannot lock before reading config location")
				os.Exit(1)
			}
			file = path.Join(path.Dir(p), ".autorestic.lock.yml")
			if !flags.CRON_LEAN {
				colors.Faint.Println("Using lock:\t", file)
			}
			lock.SetConfigFile(file)
			lock.SetConfigType("yml")
			lock.ReadInConfig()
		})
	}
	return lock
}

func setLockValue(key string, value interface{}) (*viper.Viper, error) {
	lock := getLock()

	if key == RUNNING {
		value := value.(bool)
		if value && lock.GetBool(key) {
			owner := getOwner()
			reason := staleReason(owner)
			if reason == "" {
				if owner.PID > 0 {
					colors.Error.Printf("an instance is already running (%s). exiting\n", owner)
				} else {
					colors.Error.Println("an instance is already running. exiting")
				}
				os.Exit(1)
			}
			colors.Secondary.Printf("removing stale lock (%s): %s\n", owner, reason)
		}
		if value {
			setOwner(currentOwner())
		} else {
			clearOwner()
		}
	}

	lock.Set(key, value)
	if err := lock.WriteConfigAs(file); err != nil {
		return nil, err
	}
	return lock, nil
}

func getOwner() Owner {
	l := getLock()
	return Owner{
		PID:       l.GetInt(ownerPID),
		Hostname:  l.GetString(ownerHostname),
		BootID:    l.GetString(ownerBootID),
		StartTime: l.GetString(ownerStartTime),
		Started:   l.GetInt64(ownerStarted),
	}
}

func setOwner(o Owner) {
	l := getLock()
	l.Set(ownerPID, o.PID)
	l.Set(ownerHostname, o.Hostname)
	l.Set(ownerBootID, o.BootID)
	l.Set(ownerStartTime, o.StartTime)
	l.Set(ownerStarted, o.Started)
}

func clearOwner() {
	setOwner(Owner{})
}

func GetCron(location string) int64 {
	return getLock().GetInt64("cron." + location)
}

func SetCron(location string, value int64) {
	setLockValue("cron."+location, value)
}

func GetVerifyCron(location string) int64 {
	return getLock().GetInt64("verify." + location)
}

func SetVerifyCron(location string, value int64) {
	setLockValue("verify."+location, value)
}

// Lock marks autorestic as running, recording this process as the owner. If
// the lock is held by a process that provably no longer exists (see
// staleReason), the stale lock is taken over; otherwise it exits.
func Lock() error {
	_, err := setLockValue(RUNNING, true)
	return err
}

// Unlock releases the lock if this process holds it. It is called on every
// error exit, including from processes that never took the lock, so it must
// not release a lock owned by another running instance.
func Unlock() error {
	l := getLock()
	if !l.GetBool(RUNNING) {
		return nil
	}
	if owner := getOwner(); owner.PID > 0 && owner.PID != os.Getpid() {
		return nil
	}
	_, err := setLockValue(RUNNING, false)
	return err
}

// ForceUnlock releases the lock regardless of which process holds it
// (`autorestic unlock`).
func ForceUnlock() error {
	_, err := setLockValue(RUNNING, false)
	return err
}
