#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -x
set -e

LC_ALL=C
umask 002

# -----------------------------------------------------------------------------
# prepare the environment for the jobs

# installation directory
export NETDATA_INSTALL_PATH="${1-/opt/netdata}"

# our source directory
NETDATA_MAKESELF_PATH="$(
    self=${0}
    while [ -L "${self}" ]
    do
        cd "${self%/*}" || exit 1
        self=$(readlink "${self}")
    done
    cd "${self%/*}" || exit 1
    pwd -P
)"
export NETDATA_MAKESELF_PATH

# netdata source directory
NETDATA_SOURCE_PATH="$(
    cd "${NETDATA_MAKESELF_PATH}/../.." || exit 1
    pwd -P
)"
export NETDATA_SOURCE_PATH

# make sure ${NULL} is empty
export NULL=

# -----------------------------------------------------------------------------

cd "${NETDATA_MAKESELF_PATH}" || exit 1

# shellcheck source=packaging/makeself/functions.sh
. ./functions.sh "${@}" || exit 1

for x in jobs/*.sh; do
  progress "running ${x}"
  "${x}" "${NETDATA_INSTALL_PATH}"
done

echo >&2 "All jobs for static packaging done successfully."
exit 0
