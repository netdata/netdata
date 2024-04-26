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

# First run install-alpine-packages.sh under alpine linux to install
# the required packages. build-x86_64-static.sh will do this for you
# using docker.

mkdir -p /usr/src
cp -va /netdata /usr/src/netdata
chown -R root:root /usr/src/netdata

cd /usr/src/netdata/packaging/makeself || exit 1

cat >&2 << EOF
This program will create a self-extracting shell package containing
a statically linked netdata, able to run on any 64bit Linux system,
without any dependencies from the target system.

It can be used to have netdata running in no-time, or in cases the
target Linux system cannot compile netdata.
EOF

if [ ! -d tmp ]; then
  mkdir tmp || exit 1
else
  rm -rf tmp/*
fi

if [ -z "${GITHUB_ACTIONS}" ]; then
    export GITHUB_ACTIONS=false
fi

if ! ./run-all-jobs.sh "$@"; then
  printf >&2 "Build failed."
  exit 1
fi

mkdir -p /netdata/artifacts
cp -va /usr/src/netdata/artifacts/* /netdata/artifacts/
chown -R "$(stat -c '%u:%g' /netdata)" /netdata/artifacts/
