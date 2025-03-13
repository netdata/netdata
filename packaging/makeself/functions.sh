#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# -----------------------------------------------------------------------------

# allow running the jobs by hand
[ -z "${NETDATA_BUILD_WITH_DEBUG}" ] && export NETDATA_BUILD_WITH_DEBUG=0
[ -z "${NETDATA_INSTALL_PATH}" ] && export NETDATA_INSTALL_PATH="${1-/opt/netdata}"
[ -z "${NETDATA_MAKESELF_PATH}" ] && NETDATA_MAKESELF_PATH="$(
    self=${0}
    while [ -L "${self}" ]
    do
        cd "${self%/*}" || exit 1
        self=$(readlink "${self}")
    done
    cd "${self%/*}" || exit 1
    cd ../.. || exit 1
    echo "$(pwd -P)/${self##*/}"
)"
[ -z "${NETDATA_SOURCE_PATH}" ] && NETDATA_SOURCE_PATH="$(
    cd "${NETDATA_MAKESELF_PATH}/../.." || exit 1
    pwd -P
)"
export NETDATA_MAKESELF_PATH NETDATA_SOURCE_PATH
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

cache_path() {
  local key="${1}"
  echo "${NETDATA_SOURCE_PATH}/artifacts/cache/${BUILDARCH}/${key}"
}

build_path() {
  local dir="${1}"
  local prefix="/build"

  mkdir -p "${prefix}"

  echo "${prefix}/${dir}"
}

fetch() {
  local dir="${1}" url="${2}" sha256="${3}" key="${4}"
  local tar
  tar="$(basename "${2}")"
  local cache
  cache="$(cache_path "${key}")"
  local path
  path="$(build_path "${dir}")"

  if [ -d "${path}" ]; then
    rm -rf "${path}"
  fi

  if [ -d "${cache}/${dir}" ]; then
    echo "Found cached copy of build directory for ${key}, using it."
    cp -a "${cache}/${dir}" "${path}"
    CACHE_HIT=1
  else
    echo "No cached copy of build directory for ${key} found, fetching sources instead."

    if [ ! -f "/tmp/${tar}" ]; then
      run wget -O "/tmp/${tar}" "${url}"
    fi

    # Check SHA256 of gzip'd tar file (apparently alpine's sha256sum requires
    # two empty spaces between the checksum and the file's path)
    set +e
    echo "${sha256} /tmp/${tar}" | sha256sum --c --status
    local rc=$?
    if [ ${rc} -ne 0 ]; then
        echo >&2 "SHA256 verification of tar file ${tar} failed (rc=${rc})"
        echo >&2 "expected: ${sha256}, got $(sha256sum "/tmp/${tar}")"
        exit 1
    fi

    set -e
    run tar -axpf "/tmp/${tar}" -C "/build"

    CACHE_HIT=0
  fi

  run cd "${path}"
}

fetch_git() {
  local dir="${1}" url="${2}" tag="${3}" key="${4}" fetch_via_checkout="${5:-""}"
  local cache
  cache="$(cache_path "${key}")"
  local path
  path="$(build_path "${dir}")"

  if [ -d "${path}" ]; then
    rm -rf "${path}"
  fi

  if [ -d "${cache}/${dir}" ]; then
    echo "Found cached copy of build directory for ${key}, using it."
    cp -a "${cache}/${dir}" "${path}"
    CACHE_HIT=1
  else
    echo "No cached copy of build directory for ${key} found, fetching sources instead."
    if [ -n "${fetch_via_checkout}" ]; then
      run git clone "${url}" "${path}"
      cd "${path}" && run git checkout "${tag}"
    else
      run git clone --branch "${tag}" --single-branch --depth 1 "${url}" "${path}"
    fi
    CACHE_HIT=0
  fi

  run cd "${path}"
}

store_cache() {
  local key="${1}"
  local dir="${2}"
  local src
  src="$(build_path "${dir}")"

  local cache
  cache="$(cache_path "${key}")"

  if [ "${CACHE_HIT:-0}" -eq 0 ]; then
    if [ -d "${cache}" ]; then
      rm -rf "${cache}"
    fi

    mkdir -p "${cache}"

    cp -a "${src}" "${cache}"
    chown -R "$(stat -c '%u:%g' "${NETDATA_SOURCE_PATH}")" "${cache}"
  fi
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
