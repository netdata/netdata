#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later

# -----------------------------------------------------------------------------
# parse command line arguments

set -e

export NETDATA_BUILD_WITH_DEBUG=0

while [ -n "${1}" ]; do
  case "${1}" in
    debug)
      export NETDATA_BUILD_WITH_DEBUG=1
      ;;

    *) ;;

  esac

  shift
done

# -----------------------------------------------------------------------------

cd /netdata/packaging/makeself || exit 1

cat >&2 << EOF
This program will create a self-extracting shell package containing
a statically linked netdata, able to run on any 64bit Linux system,
without any dependencies from the target system.

It can be used to have netdata running in no-time, or in cases the
target Linux system cannot compile netdata.
EOF

if [ -z "${GITHUB_ACTIONS}" ]; then
    export GITHUB_ACTIONS=false
fi

mkdir -p /netdata/artifacts
chown "$(stat -c '%u:%g' /netdata)" /netdata/artifacts/

if ! ./run-all-jobs.sh "$@"; then
  printf >&2 "Build failed."

  if [ -n "${DEBUG_BUILD_INFRA}" ]; then
    exec /bin/bash
  else
    exit 1
  fi
fi
