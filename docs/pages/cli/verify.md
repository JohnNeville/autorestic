# Verify

```bash
autorestic verify [-l, --location] [-a, --all]
```

Runs [`restic check`](https://restic.readthedocs.io/en/latest/045_working_with_repos.html#checking-integrity-and-consistency) against every backend the location backs up to, including `copy` targets, wrapped in the location's `verify-hooks`.

Extra flags come from the `check` key in the location, backend or global options, e.g. to also read back a sample of the data:

```yaml | .autorestic.yml
locations:
  my-location:
    from: /data
    to: my-backend
    options:
      check:
        read-data-subset: 5%
```

Not to be confused with [`autorestic check`](/cli/check), which only validates the config.
