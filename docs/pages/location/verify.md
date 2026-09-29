# Verify

Backups that are never checked can quietly rot. A location can run `restic check` on its own schedule with `verify-cron`, and report the result with `verify-hooks`, the same way `cron` and `hooks` work for backups.

```yaml | .autorestic.yml
locations:
  my-location:
    from: /data
    to: my-backend
    cron: '0 3 * * *'
    verify-cron: '0 4 15 * *' # 15th of every month at 4:00
    options:
      check:
        read-data-subset: 5%
    verify-hooks:
      before:
        - curl -fsS "$HC_VERIFY/start"
      success:
        - curl -fsS "$HC_VERIFY"
      failure:
        - curl -fsS --data-raw "$AUTORESTIC_FAILURE_REASON" "$HC_VERIFY/fail"
```

- The check runs inside `autorestic cron`, so other locations wait until it finishes. Pick a quiet time.
- The first time `autorestic cron` sees a `verify-cron`, it only starts the schedule; the first check runs at the next scheduled time. Use `autorestic verify -l my-location` to run one now.
- The check covers the whole repository. If several locations share a backend, set `verify-cron` on only one of them.

## Hooks

`verify-hooks` supports `before`, `after`, `success` and `failure`. All of them get `AUTORESTIC_LOCATION` and the location's `envs`; `failure` also gets `AUTORESTIC_FAILURE_REASON` with restic's output. If `before` fails, the check is skipped and `failure` runs.
