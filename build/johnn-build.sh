#!/bin/bash
# Build autorestic for John's hosts from the committed tree: gofmt, vet and
# tests first, then static Linux binaries in dist/, stamped with the commit
# ("autorestic --version" prints e.g. 1.8.7-johnn+76884bf6a1b2).
#
# Uses the Go image in Docker (rootless is fine), so no Go install is needed.
#
#   build/johnn-build.sh                  # linux/amd64
#   ARCHES="amd64 arm64" build/johnn-build.sh
#   ALLOW_DIRTY=1 build/johnn-build.sh    # test build with uncommitted changes
#
# Then install with: sudo build/johnn-install.sh
set -euo pipefail
cd "$(dirname "$0")/.."

GO_IMAGE=${GO_IMAGE:-golang:1.25-alpine}   # keep in step with the Dockerfile
ARCHES=${ARCHES:-amd64}
CACHE=${XDG_CACHE_HOME:-$HOME/.cache}

commit=$(git rev-parse --short=12 HEAD)
if ! git diff --quiet HEAD --; then
    if [[ ${ALLOW_DIRTY:-0} != 1 ]]; then
        echo "uncommitted changes; commit them first (or ALLOW_DIRTY=1 for a test build)" >&2
        git status --short >&2
        exit 1
    fi
    commit+="-dirty"
fi

mkdir -p dist "$CACHE/go-mod" "$CACHE/go-build"
go() {
    docker run --rm \
        -v "$PWD":/src -w /src \
        -v "$CACHE/go-mod":/go/pkg/mod -v "$CACHE/go-build":/root/.cache/go-build \
        -e CGO_ENABLED=0 "$@"
}

go "$GO_IMAGE" sh -euc '
    unformatted=$(gofmt -l .)
    [ -z "$unformatted" ] || { echo "gofmt needed:"; echo "$unformatted"; exit 1; }
    go vet ./...
    go test ./...
'

ldflags="-s -w -X github.com/JohnNeville/autorestic/internal.Commit=$commit"
for arch in $ARCHES; do
    go -e GOOS=linux -e GOARCH="$arch" "$GO_IMAGE" \
        go build -trimpath -ldflags "$ldflags" -o "dist/autorestic-linux-$arch" .
done

# What johnn-install.sh expects --version to print.
version=$(sed -n 's/^const VERSION = "\(.*\)"$/\1/p' internal/config.go)
echo "autorestic version $version+$commit" > dist/BUILD_VERSION
(cd dist && sha256sum autorestic-linux-* > SHA256SUMS)

echo
cat dist/BUILD_VERSION
ls -l dist/autorestic-linux-*
