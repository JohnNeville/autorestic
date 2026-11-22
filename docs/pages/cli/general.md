# General

## `-c, --config`

Specify the config file to be used (must use .yml as an extension).
If omitted `autorestic` will search for for a `.autorestic.yml` in the current directory and your home directory.

```bash
autorestic -c /path/to/my/config.yml
```

## `--ci`

Run the CLI in CI Mode, which means there will be no interactivity, no colors and automatically sets the `--verbose` flag.

This can be useful when you want to run cron e.g. as all the output will be saved.

CI mode also assumes it runs from a systemd service or timer. Errors and lock events are sent to the systemd journal (`SYSLOG_IDENTIFIER=autorestic`, with syslog priorities), even when stdout and stderr go to a log file:

- the final `Error: ...` of a failed run (priority err)
- a restic repository that is locked, with the holder, why the lock was kept and the command to clear it (err)
- a stale restic lock being removed and the command retried (notice): the lock was taken by a restic process on this host that no longer exists, or before this host last booted
- an autorestic instance that is already running (warning), or its stale lock being removed (notice)

Lock messages carry `AUTORESTIC_BACKEND`, `RESTIC_COMMAND`, `RESTIC_LOCK_PID` and `RESTIC_LOCK_HOST` fields. Without a journal they are written to stderr instead.

```bash
journalctl -t autorestic -p notice
```

```bash
autorestic --ci backup -a
```

## `-v, --verbose`

Verbose mode will show the output of the native restic commands that are otherwise not printed out. Useful for debugging or logging in automated tasks.

```bash
autorestic --verbose backup -a
```

## `--restic-bin`

With `--restic-bin` you can specify to run a specific restic binary. This can be useful if you want to [create a custom binary with root access that can be executed by any user](https://restic.readthedocs.io/en/stable/080_examples.html#full-backup-without-root).

```bash
autorestic --restic-bin /some/path/to/my/custom/restic/binary
```
