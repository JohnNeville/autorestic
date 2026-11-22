# Development

## Coding

The easiest way (imo) is to run [`gowatch`](https://github.com/silenceper/gowatch) in a separate terminal and the simply run `./autorestic ...`. `gowatch` will watch the code and automatically rebuild the binary when changes are saved to disk.

## Building

```bash
go run build/build.go
```

This will build and compress binaries for multiple platforms. The output will be put in the `dist` folder.

## Releasing

Releases are automatically built by the github workflow and uploaded to the release.

1. Bump `VERSION` in `internal/config.go`.
2. Update `CHANGELOG.md`
3. Commit to master
4. Create a new release with the `v1.2.3` tag and mark as pre-release.
5. The Github action will build the binaries, upload and mark the release as ready when done.

### Brew

1. Download the latest release.
2. Check the checksum with `shasum -a 256 autorestic-1.2.3.tar.gz`
3. Update `url` and `sha256` in the brew repo.
4. Submit PR

## John's fork

`johnn` is the branch that gets built and deployed. It is upstream `master` plus
every fork change; feature branches are merged into it and then deleted. Keep
the `-johnn` suffix on `VERSION` in `internal/config.go`, and bump it with each
change that gets deployed.

Build and install (Go runs in Docker, so no local Go is needed):

```bash
build/johnn-build.sh                 # gofmt, vet, tests, then dist/autorestic-linux-amd64
sudo build/johnn-install.sh          # installs /usr/local/bin/autorestic, keeps the old one
autorestic --version                 # e.g. 1.8.7-johnn+76884bf6a1b2
```

Builds are stamped with their commit, and the install refuses a binary that
doesn't match the last build, so an older build can't be installed by mistake.
For another architecture: `ARCHES="amd64 arm64" build/johnn-build.sh`.

Upstream sync: `git fetch https://github.com/cupcakearmy/autorestic.git master`,
then merge `FETCH_HEAD` into `johnn`. Drop fork changes that upstream now ships
an equivalent of, and tag `johnn` (`backup/johnn-YYYYMMDD`) before any rebase.

Don't run `autorestic upgrade`: it replaces this build with an upstream release.
