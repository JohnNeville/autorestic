#!/bin/bash
# Install the binary built by build/johnn-build.sh as /usr/local/bin/autorestic.
#
#   sudo build/johnn-install.sh
#
# Refuses a binary whose checksum or --version doesn't match that build, so an
# older build can't be installed by mistake. The replaced binary is kept as
# /usr/local/bin/autorestic.<its version>. Doesn't use git, so it works as root
# on a checkout owned by another user.
set -euo pipefail
cd "$(dirname "$0")/../dist"
[[ $EUID -eq 0 ]] || { echo "run with sudo" >&2; exit 1; }

case $(uname -m) in
    x86_64) arch=amd64 ;;
    aarch64) arch=arm64 ;;
    *) echo "unsupported machine $(uname -m)" >&2; exit 1 ;;
esac
bin=autorestic-linux-$arch
dest=/usr/local/bin/autorestic

[[ -f $bin && -f BUILD_VERSION && -f SHA256SUMS ]] || { echo "no build in $PWD; run build/johnn-build.sh first" >&2; exit 1; }
grep " $bin\$" SHA256SUMS | sha256sum -c --quiet - || { echo "$bin doesn't match SHA256SUMS" >&2; exit 1; }
want=$(cat BUILD_VERSION)
got=$(./"$bin" --version)
[[ $got == "$want" ]] || { echo "$bin reports '$got', build says '$want'" >&2; exit 1; }
[[ $want != *-dirty ]] || { echo "refusing a -dirty test build; commit and rebuild" >&2; exit 1; }

if [[ -x $dest ]]; then
    old=$("$dest" --version 2>/dev/null | awk '{print $3}')
    if [[ "autorestic version $old" == "$want" ]]; then
        echo "$dest is already $want"
        exit 0
    fi
    cp -p "$dest" "$dest.${old:-old}"
    echo "kept previous $dest as $dest.${old:-old}"
fi
install -m 755 "$bin" "$dest.new"
mv -f "$dest.new" "$dest"   # atomic: a running autorestic keeps its old file
"$dest" --version
