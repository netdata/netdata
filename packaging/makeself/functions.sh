#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# -----------------------------------------------------------------------------

# allow running the jobs by hand
[ -z "${NETDATA_BUILD_WITH_DEBUG}" ] && export NETDATA_BUILD_WITH_DEBUG=0
[ -z "${NETDATA_INSTALL_PATH}" ] && export NETDATA_INSTALL_PATH="${1-/opt/netdata}"
[ -z "${NETDATA_MAKESELF_PATH}" ] && NETDATA_MAKESELF_PATH="$(dirname "${0}")/../.."
[ "${NETDATA_MAKESELF_PATH:0:1}" != "/" ] && NETDATA_MAKESELF_PATH="$(pwd)/${NETDATA_MAKESELF_PATH}"
[ -z "${NETDATA_SOURCE_PATH}" ] && export NETDATA_SOURCE_PATH="${NETDATA_MAKESELF_PATH}/../.."
export NETDATA_MAKESELF_PATH NETDATA_MAKESELF_PATH
export NULL=

# make sure the path does not end with /
if [ "${NETDATA_INSTALL_PATH:$((${#NETDATA_INSTALL_PATH} - 1)):1}" = "/" ]; then
  export NETDATA_INSTALL_PATH="${NETDATA_INSTALL_PATH:0:$((${#NETDATA_INSTALL_PATH} - 1))}"
fi

# find the parent directory
NETDATA_INSTALL_PARENT="$(dirname "${NETDATA_INSTALL_PATH}")"
export NETDATA_INSTALL_PARENT

# -----------------------------------------------------------------------------

# bash strict mode
set -euo pipefail

# -----------------------------------------------------------------------------

fetch() {
  local dir="${1}" url="${2}" sha256="${3}"
  local tar="${dir}.tar.gz"

  if [ ! -f "${NETDATA_MAKESELF_PATH}/tmp/${tar}" ]; then
    run wget -O "${NETDATA_MAKESELF_PATH}/tmp/${tar}" "${url}"
  fi

  # Check SHA256 of gzip'd tar file (apparently alpine's sha256sum requires
  # two empty spaces between the checksum and the file's path)
  set +e
  echo "${sha256}  ${NETDATA_MAKESELF_PATH}/tmp/${tar}" | sha256sum -c -s
  local rc=$?
  if [ ${rc} -ne 0 ]; then
      echo >&2 "SHA256 verification of tar file ${tar} failed (rc=${rc})"
      echo >&2 "expected: ${sha256}, got $(sha256sum "${NETDATA_MAKESELF_PATH}/tmp/${tar}")"
      exit 1
  fi
  set -e

  if [ ! -d "${NETDATA_MAKESELF_PATH}/tmp/${dir}" ]; then
    cd "${NETDATA_MAKESELF_PATH}/tmp"
    run tar -zxpf "${tar}"
    cd -
  fi

  run cd "${NETDATA_MAKESELF_PATH}/tmp/${dir}"
}

# -----------------------------------------------------------------------------

# load the functions of the netdata-installer.sh
# shellcheck source=packaging/installer/functions.sh
. "${NETDATA_SOURCE_PATH}/packaging/installer/functions.sh"

# -----------------------------------------------------------------------------

# debug
echo "ME=${0}"
echo "NETDATA_INSTALL_PARENT=${NETDATA_INSTALL_PARENT}"
echo "NETDATA_INSTALL_PATH=${NETDATA_INSTALL_PATH}"
echo "NETDATA_MAKESELF_PATH=${NETDATA_MAKESELF_PATH}"
echo "NETDATA_SOURCE_PATH=${NETDATA_SOURCE_PATH}"
echo "PROCESSORS=$(nproc)"
