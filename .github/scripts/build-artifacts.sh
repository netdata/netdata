#!/bin/sh
#
# Builds the netdata-vX.y.Z-xxxx.tar.gz source tarball (dist)
# and netdata-vX.Y.Z-xxxx.gz.run (static x86_64) artifacts.

set -e

# shellcheck source=.github/scripts/functions.sh
. "$(dirname "$0")/functions.sh"

NAME="${NAME:-netdata}"
VERSION="${VERSION:-"$(git describe)"}"
BASENAME="$NAME-$VERSION"

prepare_build() {
  progress "Preparing build"
  (
    test -d artifacts || mkdir -p artifacts
    echo "${VERSION}" > packaging/version
  ) >&2
}

build_dist() {
  progress "Building dist"
  (
    command -v git > /dev/null && [ -d .git ] && git clean -d -f
    autoreconf -ivf
    ./configure \
      --prefix=/usr \
      --sysconfdir=/etc \
      --localstatedir=/var \
      --libexecdir=/usr/libexec \
      --with-zlib \
      --with-math \
      --with-user=netdata \
      --disable-dependency-tracking \
      CFLAGS=-O2
    make dist
    mv "${BASENAME}.tar.gz" artifacts/
  ) >&2
}

build_static_x86_64() {
  progress "Building static x86_64"
  (
    command -v git > /dev/null && [ -d .git ] && git clean -d -f
    USER="" ./packaging/makeself/build-x86_64-static.sh
  ) >&2
}

prepare_assets() {
  progress "Preparing assets"
  (
    cp packaging/version artifacts/latest-version.txt

    cd artifacts || exit 1
    ln -f "${BASENAME}.tar.gz" netdata-latest.tar.gz
    ln -f "${BASENAME}.gz.run" netdata-latest.gz.run
    sha256sum -b ./* > "sha256sums.txt"
  ) >&2
}

steps="prepare_build build_dist build_static_x86_64"
steps="$steps prepare_assets"

_main() {
  for step in $steps; do
    if ! run "$step"; then
      if [ -t 1 ]; then
        debug
      else
        fail "Build failed"
      fi
    fi
  done

  echo "🎉 All Done!"
}

if [ -n "$0" ] && [ x"$0" != x"-bash" ]; then
  _main "$@"
fi
