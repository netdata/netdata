#!/bin/sh
#
# Builds the netdata-vX.Y.Z-xxxx.gz.run (static x86_64) artifact.

set -e

# shellcheck source=.github/scripts/functions.sh
. "$(dirname "$0")/functions.sh"

BUILDARCH="${1}"
NAME="${NAME:-netdata}"
VERSION="${VERSION:-"$(git describe)"}"
BASENAME="$NAME-$BUILDARCH-$VERSION"

prepare_build() {
  test -d artifacts || mkdir -p artifacts
}

build_static() {
  EXTRA_INSTALL_FLAGS="${EXTRA_INSTALL_FLAGS}" USER="" ./packaging/makeself/build-static.sh "${BUILDARCH}"
}

prepare_assets() {
  cp packaging/version artifacts/latest-version.txt

  cd artifacts || exit 1
  ln -s "${BASENAME}.gz.run" "netdata-${BUILDARCH}-latest.gz.run"
  if [ "${BUILDARCH}" = "x86_64" ]; then
    ln -s "${BASENAME}.gz.run" netdata-latest.gz.run
  fi
}

steps="prepare_build build_static"
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
