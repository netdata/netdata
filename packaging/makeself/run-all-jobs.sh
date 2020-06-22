#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -e

LC_ALL=C
umask 002

# -----------------------------------------------------------------------------
# prepare the environment for the jobs

# installation directory
export NETDATA_INSTALL_PATH="${1-/opt/netdata}"

# our source directory
NETDATA_MAKESELF_PATH="$(dirname "${0}")"
export NETDATA_MAKESELF_PATH
if [ "${NETDATA_MAKESELF_PATH:0:1}" != "/" ]; then
  NETDATA_MAKESELF_PATH="$(pwd)/${NETDATA_MAKESELF_PATH}"
  export NETDATA_MAKESELF_PATH
fi

# netdata source directory
export NETDATA_SOURCE_PATH="${NETDATA_MAKESELF_PATH}/../.."

# make sure ${NULL} is empty
export NULL=

# -----------------------------------------------------------------------------

cd "${NETDATA_MAKESELF_PATH}" || exit 1

# shellcheck source=packaging/makeself/functions.sh
. ./functions.sh "${@}" || exit 1

for x in jobs/*.install.sh; do
  progress "running ${x}"
  "${x}" "${NETDATA_INSTALL_PATH}"
done

echo >&2 "All jobs for static packaging done successfully."
exit 0
