#!/bin/sh

# Netdata updater utility
#
# Variables needed by script:
#  - PATH
#  - CFLAGS
#  - LDFLAGS
#  - MAKEOPTS
#  - IS_NETDATA_STATIC_BINARY
#  - NETDATA_CONFIGURE_OPTIONS
#  - REINSTALL_OPTIONS
#  - NETDATA_TARBALL_URL
#  - NETDATA_TARBALL_CHECKSUM_URL
#  - NETDATA_TARBALL_CHECKSUM
#  - NETDATA_PREFIX
#  - NETDATA_LIB_DIR
#
# Optional environment options:
#
#  - TMPDIR (set to a usable temporary directory)
#  - NETDATA_NIGHTLIES_BASEURL (set the base url for downloading the dist tarball)
#
# Copyright: 2018-2020 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Author: Paweł Krupa <paulfantom@gmail.com>
# Author: Pavlos Emm. Katsoulakis <paul@netdata.cloud>
# Author: Austin S. Hemmelgarn <austin@netdata.cloud>

set -e

script_dir="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"

if [ -x "${script_dir}/netdata-updater" ]; then
  script_source="${script_dir}/netdata-updater"
else
  script_source="${script_dir}/netdata-updater.sh"
fi

info() {
  echo >&3 "$(date) : INFO: " "${@}"
}

error() {
  echo >&3 "$(date) : ERROR: " "${@}"
}

: "${ENVIRONMENT_FILE:=THIS_SHOULD_BE_REPLACED_BY_INSTALLER_SCRIPT}"

if [ "${ENVIRONMENT_FILE}" = "THIS_SHOULD_BE_REPLACED_BY_INSTALLER_SCRIPT" ]; then
  if [ -r "${script_dir}/../../../etc/netdata/.environment" ]; then
    ENVIRONMENT_FILE="${script_dir}/../../../etc/netdata/.environment"
  elif [ -r "/etc/netdata/.environment" ]; then
    ENVIRONMENT_FILE="/etc/netdata/.environment"
  elif [ -r "/opt/netdata/etc/netdata/.environment" ]; then
    ENVIRONMENT_FILE="/opt/netdata/etc/netdata/.environment"
  else
    envpath="$(find / -type d \( -path /sys -o -path /proc -o -path /dev \) -prune -false -o -path '*netdata/.environment' -type f  2> /dev/null | head -n 1)"
    if [ -r "${envpath}" ]; then
      ENVIRONMENT_FILE="${envpath}"
    else
      error "Cannot find environment file, unable to update."
      exit 1
    fi
  fi
fi

safe_sha256sum() {
  # Within the context of the installer, we only use -c option that is common between the two commands
  # We will have to reconsider if we start non-common options
  if command -v sha256sum > /dev/null 2>&1; then
    sha256sum "$@"
  elif command -v shasum > /dev/null 2>&1; then
    shasum -a 256 "$@"
  else
    fatal "I could not find a suitable checksum binary to use"
  fi
}

# this is what we will do if it fails (head-less only)
fatal() {
  error "FAILED TO UPDATE NETDATA : ${1}"
  exit 1
}

cleanup() {
  if [ -n "${logfile}" ]; then
    cat >&2 "${logfile}"
    rm "${logfile}"
  fi

  if [ -n "$ndtmpdir" ] && [ -d "$ndtmpdir" ]; then
    rm -rf "$ndtmpdir"
  fi
}

_cannot_use_tmpdir() {
  testfile="$(TMPDIR="${1}" mktemp -q -t netdata-test.XXXXXXXXXX)"
  ret=0

  if [ -z "${testfile}" ] ; then
    return "${ret}"
  fi

  if printf '#!/bin/sh\necho SUCCESS\n' > "${testfile}" ; then
    if chmod +x "${testfile}" ; then
      if [ "$("${testfile}")" = "SUCCESS" ] ; then
        ret=1
      fi
    fi
  fi

  rm -f "${testfile}"
  return "${ret}"
}

create_tmp_directory() {
  if [ -n "${NETDATA_TMPDIR_PATH}" ]; then
    echo "${NETDATA_TMPDIR_PATH}"
  else
    if [ -z "${NETDATA_TMPDIR}" ] || _cannot_use_tmpdir "${NETDATA_TMPDIR}" ; then
      if [ -z "${TMPDIR}" ] || _cannot_use_tmpdir "${TMPDIR}" ; then
        if _cannot_use_tmpdir /tmp ; then
          if _cannot_use_tmpdir "${PWD}" ; then
            echo >&2
            echo >&2 "Unable to find a usable temporary directory. Please set \$TMPDIR to a path that is both writable and allows execution of files and try again."
            exit 1
          else
            TMPDIR="${PWD}"
          fi
        else
          TMPDIR="/tmp"
        fi
      fi
    else
      TMPDIR="${NETDATA_TMPDIR}"
    fi

    mktemp -d -t netdata-updater-XXXXXXXXXX
  fi
}

_safe_download() {
  url="${1}"
  dest="${2}"
  if command -v curl > /dev/null 2>&1; then
    curl -sSL --connect-timeout 10 --retry 3 "${url}" > "${dest}"
    return $?
  elif command -v wget > /dev/null 2>&1; then
    wget -T 15 -O - "${url}" > "${dest}"
    return $?
  else
    return 255
  fi
}

download() {
  url="${1}"
  dest="${2}"

  _safe_download "${url}" "${dest}"
  ret=$?

  if [ ${ret} -eq 0 ]; then
    return 0
  elif [ ${ret} -eq 255 ]; then
    fatal "I need curl or wget to proceed, but neither is available on this system."
  else
    fatal "Cannot download ${url}"
  fi
}

get_netdata_latest_tag() {
  dest="${1}"
  url="https://github.com/netdata/netdata/releases/latest"

  if command -v curl >/dev/null 2>&1; then
    tag=$(curl "${url}" -s -L -I -o /dev/null -w '%{url_effective}' | grep -m 1 -o '[^/]*$')
  elif command -v wget >/dev/null 2>&1; then
    tag=$(wget --max-redirect=0 "${url}" 2>&1 | grep Location | cut -d ' ' -f2 | grep -m 1 -o '[^/]*$')
  else
    fatal "I need curl or wget to proceed, but neither of them are available on this system."
  fi

  if echo "${tag}" | grep -vEq "^v[0-9]+\..+"; then
    fatal "Cannot download latest stable tag from ${url}"
  fi

  echo "${tag}" >"${dest}"
}

newer_commit_date() {
  echo >&3 "Checking if a newer version of the updater script is available."

  if command -v jq > /dev/null 2>&1; then
    commit_date="$(_safe_download "https://api.github.com/repos/netdata/netdata/commits?path=packaging%2Finstaller%2Fnetdata-updater.sh&page=1&per_page=1" /dev/stdout | jq '.[0].commit.committer.date' | tr -d '"')"
  elif command -v python > /dev/null 2>&1;then
    commit_date="$(_safe_download "https://api.github.com/repos/netdata/netdata/commits?path=packaging%2Finstaller%2Fnetdata-updater.sh&page=1&per_page=1" /dev/stdout | python -c 'from __future__ import print_function;import sys,json;print(json.load(sys.stdin)[0]["commit"]["committer"]["date"])')"
  elif command -v python3 > /dev/null 2>&1;then
    commit_date="$(_safe_download "https://api.github.com/repos/netdata/netdata/commits?path=packaging%2Finstaller%2Fnetdata-updater.sh&page=1&per_page=1" /dev/stdout | python3 -c 'from __future__ import print_function;import sys,json;print(json.load(sys.stdin)[0]["commit"]["committer"]["date"])')"
  fi

  if [ -z "${commit_date}" ] ; then
    commit_date="9999-12-31T23:59:59Z"
  fi

  if [ -e "${script_source}" ]; then
    script_date="$(date -r "${script_source}" +%s)"
  else
    script_date="$(date +%s)"
  fi

  [ "$(date -d "${commit_date}" +%s)" -ge "${script_date}" ]
}

self_update() {
  if [ -z "${NETDATA_NO_UPDATER_SELF_UPDATE}" ] && newer_commit_date; then
    echo >&3 "Downloading newest version of updater script."

    ndtmpdir=$(create_tmp_directory)
    cd "$ndtmpdir" || exit 1

    if _safe_download "https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/netdata-updater.sh" ./netdata-updater.sh; then
      chmod +x ./netdata-updater.sh || exit 1
      export ENVIRONMENT_FILE="${ENVIRONMENT_FILE}"
      exec ./netdata-updater.sh --not-running-from-cron --no-updater-self-update --tmpdir-path "$(pwd)"
    else
      echo >&3 "Failed to download newest version of updater script, continuing with current version."
    fi
  fi
}

parse_version() {
  r="${1}"
  if echo "${r}" | grep -q '^v.*'; then
    # shellcheck disable=SC2001
    # XXX: Need a regex group substitution here.
    r="$(echo "${r}" | sed -e 's/^v\(.*\)/\1/')"
  fi

  tmpfile="$(mktemp)"
  echo "${r}" | tr '-' ' ' > "${tmpfile}"
  read -r v b _ < "${tmpfile}"

  if echo "${b}" | grep -vEq "^[0-9]+$"; then
    b="0"
  fi

  echo "${v}" | tr '.' ' ' > "${tmpfile}"
  read -r maj min patch _ < "${tmpfile}"

  rm -f "${tmpfile}"

  printf "%03d%03d%03d%05d" "${maj}" "${min}" "${patch}" "${b}"
}

get_latest_version() {
  if [ "${RELEASE_CHANNEL}" = "stable" ]; then
    get_netdata_latest_tag /dev/stdout
  else
    download "$NETDATA_NIGHTLIES_BASEURL/latest-version.txt" /dev/stdout
  fi
}

set_tarball_urls() {
  extension="tar.gz"

  if [ "$2" = "yes" ]; then
    extension="gz.run"
  fi

  if [ "$1" = "stable" ]; then
    latest="$(get_netdata_latest_tag /dev/stdout)"
    export NETDATA_TARBALL_URL="https://github.com/netdata/netdata/releases/download/$latest/netdata-$latest.${extension}"
    export NETDATA_TARBALL_CHECKSUM_URL="https://github.com/netdata/netdata/releases/download/$latest/sha256sums.txt"
  else
    export NETDATA_TARBALL_URL="$NETDATA_NIGHTLIES_BASEURL/netdata-latest.${extension}"
    export NETDATA_TARBALL_CHECKSUM_URL="$NETDATA_NIGHTLIES_BASEURL/sha256sums.txt"
  fi
}

update() {
  [ -z "${logfile}" ] && info "Running on a terminal - (this script also supports running headless from crontab)"

  RUN_INSTALLER=0
  ndtmpdir=$(create_tmp_directory)
  cd "$ndtmpdir" || exit 1

  download "${NETDATA_TARBALL_CHECKSUM_URL}" "${ndtmpdir}/sha256sum.txt" >&3 2>&3

  current_version="$(command -v netdata > /dev/null && parse_version "$(netdata -v | cut -f 2 -d ' ')")"
  latest_tag="$(get_latest_version)"
  latest_version="$(parse_version "${latest_tag}")"
  path_version="$(echo "${latest_tag}" | cut -f 1 -d "-")"

  # If we can't get the current version for some reason assume `0`
  current_version="${current_version:-0}"

  # If we can't get the latest version for some reason assume `0`
  latest_version="${latest_version:-0}"

  info "Current Version: ${current_version}"
  info "Latest Version: ${latest_version}"

  if [ "${latest_version}" -gt 0 ] && [ "${current_version}" -gt 0 ] && [ "${current_version}" -ge "${latest_version}" ]; then
    info "Newest version (current=${current_version} >= latest=${latest_version}) is already installed"
  elif [ -n "${NETDATA_TARBALL_CHECKSUM}" ] && grep "${NETDATA_TARBALL_CHECKSUM}" sha256sum.txt >&3 2>&3; then
    info "Newest version is already installed"
  else
    download "${NETDATA_TARBALL_URL}" "${ndtmpdir}/netdata-latest.tar.gz"
    if ! grep netdata-latest.tar.gz sha256sum.txt | safe_sha256sum -c - >&3 2>&3; then
      fatal "Tarball checksum validation failed. Stopping netdata upgrade and leaving tarball in ${ndtmpdir}\nUsually this is a result of an older copy of the tarball or checksum file being cached somewhere upstream and can be resolved by retrying in an hour."
    fi
    NEW_CHECKSUM="$(safe_sha256sum netdata-latest.tar.gz 2> /dev/null | cut -d' ' -f1)"
    tar -xf netdata-latest.tar.gz >&3 2>&3
    rm netdata-latest.tar.gz >&3 2>&3
    cd "$(find . -maxdepth 1 -name "netdata-${path_version}*" | head -n 1)" || exit 1
    RUN_INSTALLER=1
    cd "${NETDATA_LOCAL_TARBALL_OVERRIDE}" || exit 1
  fi

  # We got the sources, run the update now
  if [ ${RUN_INSTALLER} -eq 1 ]; then
    # signal netdata to start saving its database
    # this is handy if your database is big
    possible_pids=$(pidof netdata)
    do_not_start=
    if [ -n "${possible_pids}" ]; then
      kill -USR1 "${possible_pids}"
    else
      # netdata is currently not running, so do not start it after updating
      do_not_start="--dont-start-it"
    fi

    env="TMPDIR='${TMPDIR}'"

    if [ -n "${NETDATA_SELECTED_DASHBOARD}" ]; then
      env="${env} NETDATA_SELECTED_DASHBOARD=${NETDATA_SELECTED_DASHBOARD}"
    fi

    if [ ! -x ./netdata-installer.sh ]; then
      if [ "$(find . -mindepth 1 -maxdepth 1 -type d | wc -l)" -eq 1 ] && [ -x "$(find . -mindepth 1 -maxdepth 1 -type d)/netdata-installer.sh" ]; then
        cd "$(find . -mindepth 1 -maxdepth 1 -type d)" || exit 1
      fi
    fi

    if [ -e "${NETDATA_PREFIX}/etc/netdata/.install-type" ] ; then
      install_type="$(cat "${NETDATA_PREFIX}"/etc/netdata/.install-type)"
    else
      install_type="INSTALL_TYPE='legacy-build'"
    fi

    info "Re-installing netdata..."
    eval "${env} ./netdata-installer.sh ${REINSTALL_OPTIONS} --dont-wait ${do_not_start}" >&3 2>&3 || fatal "FAILED TO COMPILE/INSTALL NETDATA"

    # We no longer store checksum info here. but leave this so that we clean up all environment files upon next update.
    sed -i '/NETDATA_TARBALL/d' "${ENVIRONMENT_FILE}"

    info "Updating tarball checksum info"
    echo "${NEW_CHECKSUM}" > "${NETDATA_LIB_DIR}/netdata.tarball.checksum"

    echo "${install_type}" > "${NETDATA_PREFIX}/etc/netdata/.install-type"
  fi

  rm -rf "${ndtmpdir}" >&3 2>&3
  [ -n "${logfile}" ] && rm "${logfile}" && logfile=

  return 0
}

logfile=
ndtmpdir=

trap cleanup EXIT

while [ -n "${1}" ]; do
  if [ "${1}" = "--not-running-from-cron" ]; then
    NETDATA_NOT_RUNNING_FROM_CRON=1
    shift 1
  elif [ "${1}" = "--no-updater-self-update" ]; then
    NETDATA_NO_UPDATER_SELF_UPDATE=1
    shift 1
  elif [ "${1}" = "--tmpdir-path" ]; then
    NETDATA_TMPDIR_PATH="${2}"
    shift 2
  else
    break
  fi
done

# Random sleep to alleviate stampede effect of Agents upgrading
# and disconnecting/reconnecting at the same time (or near to).
# But only we're not a controlling terminal (tty)
# Randomly sleep between 1s and 60m
if [ ! -t 1 ] && [ -z "${NETDATA_NOT_RUNNING_FROM_CRON}" ]; then
    rnd="$(awk '
      BEGIN { srand()
              printf("%d\n", 3600 * rand())
      }')"
    sleep $(((rnd % 3600) + 1))
fi

# shellcheck source=/dev/null
. "${ENVIRONMENT_FILE}" || exit 1

# We dont expect to find lib dir variable on older installations, so load this path if none found
export NETDATA_LIB_DIR="${NETDATA_LIB_DIR:-${NETDATA_PREFIX}/var/lib/netdata}"

# Source the tarball checksum, if not already available from environment (for existing installations with the old logic)
[ -z "${NETDATA_TARBALL_CHECKSUM}" ] && [ -f "${NETDATA_LIB_DIR}/netdata.tarball.checksum" ] && NETDATA_TARBALL_CHECKSUM="$(cat "${NETDATA_LIB_DIR}/netdata.tarball.checksum")"

# Grab the nightlies baseurl (defaulting to our Google Storage bucket)
export NETDATA_NIGHTLIES_BASEURL="${NETDATA_NIGHTLIES_BASEURL:-https://storage.googleapis.com/netdata-nightlies}"

if [ "${INSTALL_UID}" != "$(id -u)" ]; then
  fatal "You are running this script as user with uid $(id -u). We recommend to run this script as root (user with uid 0)"
fi

if [ -t 2 ]; then
  # we are running on a terminal
  # open fd 3 and send it to stderr
  exec 3>&2
else
  # we are headless
  # create a temporary file for the log
  logfile="$(mktemp -t netdata-updater.log.XXXXXX)"
  # open fd 3 and send it to logfile
  exec 3> "${logfile}"
fi

self_update

set_tarball_urls "${RELEASE_CHANNEL}" "${IS_NETDATA_STATIC_BINARY}"

if [ "${IS_NETDATA_STATIC_BINARY}" = "yes" ]; then
  ndtmpdir="$(create_tmp_directory)"
  PREVDIR="$(pwd)"

  echo >&2 "Entering ${ndtmpdir}"
  cd "${ndtmpdir}" || exit 1

  download "${NETDATA_TARBALL_CHECKSUM_URL}" "${ndtmpdir}/sha256sum.txt"
  download "${NETDATA_TARBALL_URL}" "${ndtmpdir}/netdata-latest.gz.run"
  if ! grep netdata-latest.gz.run "${ndtmpdir}/sha256sum.txt" | safe_sha256sum -c - > /dev/null 2>&1; then
    fatal "Static binary checksum validation failed. Stopping netdata installation and leaving binary in ${ndtmpdir}\nUsually this is a result of an older copy of the file being cached somewhere and can be resolved by simply retrying in an hour."
  fi

  if [ -e /opt/netdata/etc/netdata/.install-type ] ; then
    install_type="$(cat /opt/netdata/etc/netdata/.install-type)"
  else
    install_type="INSTALL_TYPE='legacy-static'"
  fi

  # Do not pass any options other than the accept, for now
  # shellcheck disable=SC2086
  if sh "${ndtmpdir}/netdata-latest.gz.run" --accept -- ${REINSTALL_OPTIONS}; then
    rm -r "${ndtmpdir}"
  else
    echo >&2 "NOTE: did not remove: ${ndtmpdir}"
  fi

  echo "${install_type}" > /opt/netdata/etc/netdata/.install-type

  if [ -e "${PREVDIR}" ]; then
    echo >&2 "Switching back to ${PREVDIR}"
    cd "${PREVDIR}"
  fi
else
  # the installer updates this script - so we run and exit in a single line
  update && exit 0
fi
