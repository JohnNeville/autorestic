# Unlock

## Stale locks are cleared automatically

The lock file (`.autorestic.lock.yml`, next to the config) records which process holds it: its PID, hostname and, on Linux, the boot ID and the process start time. When a new run finds the lock held, it takes the lock over with a message like

```
removing stale lock (pid 4242 on myhost, locked 2026-09-29T21:27:21-04:00): process 4242 no longer exists
```

if it can prove the owner is gone: the system has rebooted since the lock was taken, the PID no longer exists, or the PID now belongs to a different process. Otherwise it exits with `an instance is already running (pid … on …). exiting`.

A lock is never cleared automatically when that can't be proven: if it was written by an older autorestic (no owner recorded), or taken on a different host (for example a config on shared storage). Those still need `autorestic unlock`.

Only the process holding the lock releases it on exit, so an autorestic command that fails while another instance is running no longer clears that instance's lock.

This only covers autorestic's own lock. A restic repository lock left by a killed `restic` process is separate; see [Unlock a locked repository](/examples#unlock-a-locked-repository).

## Manual unlock

In case autorestic throws the error message `an instance is already running. exiting`, but there is no instance running you can unlock the lock.

To verify that there is no instance running you can use `ps aux | grep autorestic`.

Example with no instance running:

```bash
> ps aux | grep autorestic
root       39260  0.0  0.0   6976  2696 pts/11   S+   19:41   0:00 grep autorestic
```

Example with an instance running:

```bash
> ps aux | grep autorestic
root       29465  0.0  0.0 1162068 7380 pts/7    Sl+  19:28   0:00 autorestic --ci backup -a
root       39260  0.0  0.0   6976  2696 pts/11   S+   19:41   0:00 grep autorestic
```

**If an instance is running you should not unlock as it could lead to data loss!**

```bash
autorestic unlock
```

Use the `--force` to prevent the confirmation prompt if an instance is running.

```bash
autorestic unlock --force
```
