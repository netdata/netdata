#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
#
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

# Next unused error code: U001F

set -e

PACKAGES_SCRIPT="https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/install-required-packages.sh"

NETDATA_STABLE_BASE_URL="${NETDATA_BASE_URL:-https://github.com/netdata/netdata/releases}"
NETDATA_NIGHTLY_BASE_URL="${NETDATA_BASE_URL:-https://github.com/netdata/netdata-nightlies/releases}"
NETDATA_STABLE_REPO_URL="${NETDATA_BASE_URL:-https://repository.netdata.cloud/repos/stable}"
NETDATA_NIGHTLY_REPO_URL="${NETDATA_BASE_URL:-https://repository.netdata.cloud/repos/edge}"
NETDATA_DEFAULT_ACCEPT_MAJOR_VERSIONS="1 2"

# Following variables are intended to be overridden by the updater config file.
NETDATA_UPDATER_JITTER=3600
NETDATA_NO_SYSTEMD_JOURNAL=0
NETDATA_ACCEPT_MAJOR_VERSIONS=''

script_dir="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"

if [ -x "${script_dir}/netdata-updater" ]; then
  script_source="${script_dir}/netdata-updater"
else
  script_source="${script_dir}/netdata-updater.sh"
fi

PATH="${PATH}:/usr/local/bin:/usr/local/sbin"

if [ ! -t 1 ]; then
  INTERACTIVE=0
else
  INTERACTIVE=1
fi

if [ -n "${script_source}" ]; then
  script_name="$(basename "${script_source}")"
else
  script_name="netdata-updater.sh"
fi

info() {
  echo >&3 "$(date) : INFO: ${script_name}: " "${1}"
}

warning() {
  echo >&3 "$(date) : WARNING: ${script_name}: " "${@}"
}

error() {
  echo >&3 "$(date) : ERROR: ${script_name}: " "${1}"
  if [ -n "${NETDATA_SAVE_WARNINGS}" ]; then
    NETDATA_WARNINGS="${NETDATA_WARNINGS}\n  - ${1}"
  fi
}

fatal() {
  echo >&3 "$(date) : FATAL: ${script_name}: FAILED TO UPDATE NETDATA: " "${1}"
  if [ -n "${NETDATA_SAVE_WARNINGS}" ]; then
    NETDATA_WARNINGS="${NETDATA_WARNINGS}\n  - ${1}"
  fi
  exit_reason "${1}" "${2}"
  exit 1
}

exit_reason() {
  if [ -n "${NETDATA_SAVE_WARNINGS}" ]; then
    EXIT_REASON="${1}"
    EXIT_CODE="${2}"
    if [ -n "${NETDATA_PROPAGATE_WARNINGS}" ]; then
      if [ -n "${NETDATA_SCRIPT_STATUS_PATH}" ]; then
        {
          echo "EXIT_REASON=\"${EXIT_REASON}\""
          echo "EXIT_CODE=\"${EXIT_CODE}\""
          echo "NETDATA_WARNINGS=\"${NETDATA_WARNINGS}\""
        } >> "${NETDATA_SCRIPT_STATUS_PATH}"
      else
        export EXIT_REASON
        export EXIT_CODE
        export NETDATA_WARNINGS
      fi
    fi
  fi
}

is_integer () {
  case "${1#[+-]}" in
    *[!0123456789]*) return 1 ;;
    '')              return 1 ;;
    *)               return 0 ;;
  esac
}

safe_pidof() {
  pidof_cmd="$(command -v pidof 2> /dev/null)"
  if [ -n "${pidof_cmd}" ]; then
    ${pidof_cmd} "${@}"
    return $?
  else
    ps -acxo pid,comm |
      sed "s/^ *//g" |
      grep netdata |
      cut -d ' ' -f 1
    return $?
  fi
}

issystemd() {
  # if there is no systemctl command, it is not systemd
  systemctl=$(command -v systemctl 2> /dev/null)
  if [ -z "${systemctl}" ] || [ ! -x "${systemctl}" ]; then
    return 1
  fi

  # Check the output of systemctl is-system-running.
  # If this reports 'offline', it’s not systemd. If it reports 'unknown'
  # or nothing at all (which indicates the command is not supported), it
  # may or may not be systemd, so continue to other checks. If it reports
  # anything else, it is systemd.
  #
  # This may return a non-zero exit status in cases when it actually
  # succeeded for our purposes (notably, if the state is `degraded`),
  # so we need to toggle set -e off here.
  set +e
  systemd_state="$(systemctl is-system-running)"
  set -e

  case "${systemd_state}" in
    offline) return 1 ;;
    unknown) : ;;
    "") : ;;
    *) return 0 ;;
  esac

  # if pid 1 is systemd, it is systemd
  [ "$(basename "$(readlink /proc/1/exe)" 2> /dev/null)" = "systemd" ] && return 0

  # if systemd is not running, it is not systemd
  pids=$(safe_pidof systemd 2> /dev/null)
  [ -z "${pids}" ] && return 1

  # check if the running systemd processes are not in our namespace
  myns="$(readlink /proc/self/ns/pid 2> /dev/null)"
  for p in ${pids}; do
    ns="$(readlink "/proc/${p}/ns/pid" 2> /dev/null)"

    # if pid of systemd is in our namespace, it is systemd
    [ -n "${myns}" ] && [ "${myns}" = "${ns}" ] && return 0
  done

  # else, it is not systemd
  return 1
}

systemd_unit_exists() {
  if systemctl list-unit-files "${1}" 2>&1 | tail -n 1 | grep -qv '^0 '; then
    return 0
  else
    return 1
  fi
}

# shellcheck disable=SC2009
running_under_anacron() {
    pid="${1:-$$}"
    iter="${2:-0}"

    [ "${iter}" -gt 50 ] && return 1

    if [ "$(uname -s)" = "Linux" ] && [ -r "/proc/${pid}/stat" ]; then
        ppid="$(cut -f 4 -d ' ' "/proc/${pid}/stat")"
        if [ -n "${ppid}" ]; then
            # The below case accounts for the hidepid mount option for procfs, as well as setups with LSM
            [ ! -r "/proc/${ppid}/comm" ] && return 1

            [ "${ppid}" -eq "${pid}" ] && return 1

            grep -q anacron "/proc/${ppid}/comm" && return 0

            running_under_anacron "${ppid}" "$((iter + 1))"

            return "$?"
        fi
    else
        ppid="$(ps -o pid= -o ppid= 2>/dev/null | grep -e "^ *${pid}" | xargs | cut -f 2 -d ' ')"
        if [ -n "${ppid}" ]; then
            [ "${ppid}" -eq "${pid}" ] && return 1

            ps -o pid= -o command= 2>/dev/null | grep -e "^ *${ppid}" | grep -q anacron && return 0

            running_under_anacron "${ppid}" "$((iter + 1))"

            return "$?"
        fi
    fi

    return 1
}

_get_intervaldir() {
  if [ -d /etc/cron.daily ]; then
    echo /etc/cron.daily
  elif [ -d /etc/periodic/daily ]; then
    echo /etc/periodic/daily
  else
    return 1
  fi

  return 0
}

_get_scheduler_type() {
  if _get_intervaldir > /dev/null ; then
    echo 'interval'
  elif issystemd ; then
    echo 'systemd'
  elif [ -d /etc/cron.d ] ; then
    echo 'crontab'
  else
    echo 'none'
  fi
}

confirm() {
  prompt="${1} [y/n]"

  while true; do
    echo "${prompt}"
    read -r yn

    case "$yn" in
      [Yy]*) return 0;;
      [Nn]*) return 1;;
      *) echo "Please answer yes or no.";;
    esac
  done
}

warn_major_update() {
  nmv_suffix="New major versions generally involve breaking changes, and may not work in the same way as older versions."

  if [ "${INTERACTIVE}" -eq 0 ]; then
    warning "Would update to a new major version of Netdata. ${nmv_suffix}"
    warning "To install the new major version anyway, either run the updater interactively, or include the new major version number in the NETDATA_ACCEPT_MAJOR_VERSIONS variable in ${UPDATER_CONFIG_PATH}."
    fatal "Aborting update to new major version to avoid breaking things." U001B
  else
    warning "This update will install a new major version of Netdata. ${nmv_suffix}"
    if confirm "Are you sure you want to update to a new major version of Netdata?"; then
      notice "User accepted update to new major version of Netdata."
    else
      fatal "Aborting update to new major version at user request." U001C
    fi
  fi
}

install_build_dependencies() {
  bash="$(command -v bash 2> /dev/null)"

  if [ -z "${bash}" ] || [ ! -x "${bash}" ]; then
    error "Unable to find a usable version of \`bash\` (required for local build)."
    return 1
  fi

  info "Fetching dependency handling script..."
  download "${PACKAGES_SCRIPT}" "./install-required-packages.sh" || true

  if [ ! -s "./install-required-packages.sh" ]; then
    error "Downloaded dependency installation script is empty."
  else
    info "Running dependency handling script..."

    opts="--dont-wait --non-interactive"

    # shellcheck disable=SC2086
    if ! "${bash}" "./install-required-packages.sh" ${opts} netdata >&3 2>&3; then
      error "Installing build dependencies failed. The update should still work, but you might be missing some features."
    fi
  fi
}

enable_netdata_updater() {
  updater_type="$(echo "${1}" | tr '[:upper:]' '[:lower:]')"
  case "${updater_type}" in
    systemd|interval|crontab)
      updater_type="${1}"
      ;;
    "")
      updater_type="$(_get_scheduler_type)"
      ;;
    *)
      fatal "Unrecognized updater type ${updater_type} requested. Supported types are 'systemd', 'interval', and 'crontab'." U0001
      ;;
  esac

  case "${updater_type}" in
    "systemd")
      if issystemd; then
        if systemd_unit_exists netdata-updater.timer; then
          systemctl enable netdata-updater.timer
          systemctl start netdata-updater.timer

          info "Auto-updating has been ENABLED using a systemd timer unit.\n"
          info "If the update process fails, the failure will be logged to the systemd journal just like a regular service failure."
          info "Successful updates should produce empty logs."
        else
          error "Systemd-based auto-update scheduling requested, but the required timer unit does not exist. Auto-updates have NOT been enabled."
          return 1
        fi
      else
        error "Systemd-based auto-update scheduling requested, but this does not appear to be a systemd system. Auto-updates have NOT been enabled."
        return 1
      fi
      ;;
    "interval")
      if _get_intervaldir > /dev/null; then
        ln -sf "${NETDATA_PREFIX}/usr/libexec/netdata/netdata-updater.sh" "$(_get_intervaldir)/netdata-updater"

        info "Auto-updating has been ENABLED through cron, updater script linked to $(_get_intervaldir)/netdata-updater\n"
        info "If the update process fails and you have email notifications set up correctly for cron on this system, you should receive an email notification of the failure."
        info "Successful updates will not send an email."
      else
        error "Interval-based auto-update scheduling requested, but I could not find an interval scheduling directory. Auto-updates have NOT been enabled."
        return 1
      fi
      ;;
    "crontab")
      if [ -d "/etc/cron.d" ]; then
        [ -f "/etc/cron.d/netdata-updater" ] && rm -f "/etc/cron.d/netdata-updater"
        install -p -m 0644 -o 0 -g 0 "${NETDATA_PREFIX}/usr/lib/netdata/system/cron/netdata-updater-daily" "/etc/cron.d/netdata-updater-daily"

        info "Auto-updating has been ENABLED through cron, using a crontab at /etc/cron.d/netdata-updater\n"
        info "If the update process fails and you have email notifications set up correctly for cron on this system, you should receive an email notification of the failure."
        info "Successful updates will not send an email."
      else
        error "Crontab-based auto-update scheduling requested, but there is no '/etc/cron.d'. Auto-updates have NOT been enabled."
        return 1
      fi
      ;;
    *)
      error "Unable to determine what type of auto-update scheduling to use. Auto-updates have NOT been enabled."
      return 1
  esac

  return 0
}

disable_netdata_updater() {
  if issystemd && systemd_unit_exists "netdata-updater.timer" ; then
    systemctl disable netdata-updater.timer
    systemctl stop netdata-updater.timer
  fi

  if [ -d /etc/cron.daily ]; then
    rm -f /etc/cron.daily/netdata-updater.sh
    rm -f /etc/cron.daily/netdata-updater
  fi

  if [ -d /etc/periodic/daily ]; then
    rm -f /etc/periodic/daily/netdata-updater.sh
    rm -f /etc/periodic/daily/netdata-updater
  fi

  if [ -d /etc/cron.d ]; then
    rm -f /etc/cron.d/netdata-updater
    rm -f /etc/cron.d/netdata-updater-daily
  fi

  info "Auto-updates have been DISABLED."

  return 0
}

auto_update_status() {
  case "$(_get_scheduler_type)" in
    systemd) info "The default auto-update scheduling method for this system is: systemd timer units" ;;
    crontab) info "The default auto-update scheduling method for this system is: drop-in crontab" ;;
    interval) info "The default auto-update scheduling method for this system is: drop-in periodic script" ;;
    *) info "No recognized auto-update scheduling method found" ; return ;;
  esac

  duplicate=""
  enabled=""

  if issystemd; then
    if systemd_unit_exists "netdata-updater.timer"; then
      if systemctl is-enabled netdata-updater.timer; then
        info "Auto-updates using a systemd timer unit are ENABLED"
        enabled="systemd"
      else
        info "Auto-updates using a systemd timer unit are DISABLED"
      fi
    else
      info "Auto-updates using a systemd timer unit are NOT SUPPORTED due to: Required unit files not installed"
    fi
  else
    info "Auto-updates using a systemd timer unit are NOT SUPPORTED due to: Systemd not present"
  fi

  interval_found=""

  if [ -d /etc/cron.daily ]; then
    interval_found="1"

    if [ -x /etc/cron.daily/netdata-updater.sh ] || [ -x /etc/cron.daily/netdata-updater ]; then
      info "Auto-updates using a drop-in periodic script in /etc/cron.daily are ENABLED"

      if [ -n "${enabled}" ]; then
        duplicate="1"
      else
        enabled="cron.daily"
      fi
    else
      info "Auto-updates using a drop-in periodic script in /etc/cron.daily are DISABLED"
    fi
  else
    info "Auto-updates using a drop-in periodic script in /etc/cron.daily are NOT SUPPORTED: due to: Directory does not exist"
  fi

  if [ -d /etc/periodic/daily ]; then
    if [ -x /etc/periodic/daily/netdata-updater.sh ] || [ -x /etc/periodic/daily/netdata-updater ]; then
      info "Auto-updates using a drop-in periodic script in /etc/periodic/daily are ENABLED"

      if [ -n "${enabled}" ]; then
        duplicate="1"
      else
        enabled="periodic/daily"
      fi
    else
      if [ -z "${interval_found}" ]; then
        info "Auto-updates using a drop-in periodic script in /etc/periodic/daily are DISABLED"
      fi
    fi
  elif [ -z "${interval_found}" ]; then
    info "Auto-updates using a drop-in periodic script in /etc/periodic/daily are NOT SUPPORTED due to: Directory does not exist"
  fi

  if [ -d /etc/cron.d ]; then
    if [ -f /etc/cron.d/netdata-updater ] || [ -f /etc/cron.d/netdata-updater-daily ]; then
      info "Auto-updates using a drop-in crontab are ENABLED"

      if [ -n "${enabled}" ]; then
        duplicate="1"
      else
        enabled="cron.d"
      fi
    else
      info "Auto-updates using a drop-in crontab are DISABLED"
    fi
  else
    info "Auto-updates using a drop-in crontab are NOT SUPPORTED due to: Directory does not exist"
  fi

  if [ -n "${duplicate}" ]; then
    warning "More than one method of auto-updates is enabled! Please disable and re-enable auto-updates to correct this."
  fi
}

str_in_list() {
  printf "%s\n" "${2}" | tr ' ' "\n" | grep -qE "^${1}\$"
  return $?
}

safe_sha256sum() {
  # Within the context of the installer, we only use -c option that is common between the two commands
  # We will have to reconsider if we start non-common options
  if command -v shasum > /dev/null 2>&1; then
    shasum -a 256 "$@"
  elif command -v sha256sum > /dev/null 2>&1; then
    sha256sum "$@"
  else
    fatal "I could not find a suitable checksum binary to use" U0002
  fi
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
      if [ "$("${testfile}" 2>/dev/null)" = "SUCCESS" ] ; then
        ret=1
      fi
    fi
  fi

  rm -f "${testfile}"
  return "${ret}"
}

create_exec_tmp_directory() {
  if [ -n "${NETDATA_TMPDIR_PATH}" ]; then
    echo "${NETDATA_TMPDIR_PATH}"
    return
  fi

  root_dir=""

  if [ -n "${NETDATA_TMPDIR}" ] && ! _cannot_use_tmpdir "${NETDATA_TMPDIR}"; then
    root_dir="${NETDATA_TMPDIR}"
  elif [ -n "${TMPDIR}" ] && ! _cannot_use_tmpdir "${TMPDIR}"; then
    root_dir="${TMPDIR}"
  elif ! _cannot_use_tmpdir /tmp; then
    root_dir="/tmp"
  elif ! _cannot_use_tmpdir "${PWD}"; then
    root_dir="${PWD}"
  else
    fatal "Unable to find a usable temporary directory. Please set \$TMPDIR to a path that is both writable and allows execution of files and try again." U0003
  fi

  TMPDIR="${root_dir}"

  mktemp -d -p "${root_dir}" -t netdata-updater-XXXXXXXXXX
}

check_for_curl() {
  if [ -z "${curl}" ]; then
    curl="$(PATH="${PATH}:/opt/netdata/bin" command -v curl 2>/dev/null && true)"
  fi
}

_safe_download() {
  url="${1}"
  dest="${2}"
  succeeded=0
  checked=0

  if echo "${url}" | grep -Eq "^file:///"; then
    cp "${url#file://}" "${dest}" || return 1
    return 0
  fi

  check_for_curl

  if [ -n "${curl}" ]; then
    checked=1

    if "${curl}" -fsSL --connect-timeout 10 --retry 3 "${url}" > "${dest}"; then
      succeeded=1
    elif [ "${dest}" != "/dev/null" ]; then
      rm -f "${dest}"
    fi
  fi

  if [ "${succeeded}" -eq 0 ]; then
    if command -v wget > /dev/null 2>&1; then
      checked=1

      if wget -T 15 -O - "${url}" > "${dest}"; then
        succeeded=1
      elif [ "${dest}" != "/dev/null" ]; then
        rm -f "${dest}"
      fi
    fi
  fi

  if [ "${succeeded}" -eq 1 ]; then
    return 0
  elif [ "${checked}" -eq 1 ]; then
    return 1
  else
    return 255
  fi
}

download() {
  url="${1}"
  dest="${2}"

  set +e
  _safe_download "${url}" "${dest}"
  ret=$?
  set -e

  if [ ${ret} -eq 0 ]; then
    return 0
  elif [ ${ret} -eq 255 ]; then
    fatal "I need curl or wget to proceed, but neither is available on this system." U0004
  else
    fatal "Cannot download ${url}" U0005
  fi
}

get_netdata_latest_tag() {
  url="${1}/latest"

  check_for_curl

  if [ -n "${curl}" ]; then
    tag=$("${curl}" "${url}" -s -L -I -o /dev/null -w '%{url_effective}')
  fi

  if [ -z "${tag}" ]; then
    if command -v wget >/dev/null 2>&1; then
      tag=$(wget -S -O /dev/null "${url}" 2>&1 | grep Location)
    fi
  fi

  if [ -z "${tag}" ]; then
    fatal "I need curl or wget to proceed, but neither of them are available on this system." U0006
  fi

  tag="$(echo "${tag}" | grep -Eom 1 '[^/]*/?$')"

  # Fallback case for simpler local testing.
  if echo "${tag}" | grep -Eq 'latest/?$'; then
    if _safe_download "${url}/latest-version.txt" ./ndupdate-version.txt; then
      tag="$(cat ./ndupdate-version.txt)"

      if grep -q 'Not Found' ./ndupdate-version.txt; then
        tag="latest"
      fi

      rm -f ./ndupdate-version.txt
    else
      tag="latest"
    fi
  fi

  echo "${tag}"
}

newer_commit_date() {
  info "Checking if a newer version of the updater script is available."

  ndtmpdir="$(create_exec_tmp_directory)"
  commit_check_file="${ndtmpdir}/latest-commit.json"
  commit_check_url="https://api.github.com/repos/netdata/netdata/commits?path=packaging%2Finstaller%2Fnetdata-updater.sh&page=1&per_page=1"
  python_version_check="
from __future__ import print_function
import sys, json

try:
    data = json.load(sys.stdin)
except:
    print('')
else:
    print(data[0]['commit']['committer']['date'] if isinstance(data, list) and data else '')
"

  _safe_download "${commit_check_url}" "${commit_check_file}"

  if command -v jq > /dev/null 2>&1; then
    commit_date="$(jq '.[0].commit.committer.date' 2>/dev/null < "${commit_check_file}" | tr -d '"')"
  elif command -v python > /dev/null 2>&1;then
    commit_date="$(python -c "${python_version_check}" < "${commit_check_file}")"
  elif command -v python3 > /dev/null 2>&1;then
    commit_date="$(python3 -c "${python_version_check}" < "${commit_check_file}")"
  fi

  if [ -z "${NETDATA_TMPDIR_PATH}" ]; then
    rm -rf "${ndtmpdir}" >&3 2>&3
  fi

  if [ -z "${commit_date}" ] ; then
    return 0
  elif [ "$(uname)" = "Linux" ]; then
    commit_date="$(date -d "${commit_date}" +%s)"
  else # assume BSD-style `date` if we are not on Linux
    commit_date="$(/bin/date -j -f "%Y-%m-%dT%H:%M:%SZ" "${commit_date}" +%s 2>/dev/null)"

    if [ -z "${commit_date}" ]; then
        return 0
    fi
  fi

  if [ -e "${script_source}" ]; then
    script_date="$(date -r "${script_source}" +%s)"
  else
    script_date="$(date +%s)"
  fi

  [ "${commit_date}" -ge "${script_date}" ]
}

self_update() {
  if [ -z "${NETDATA_NO_UPDATER_SELF_UPDATE}" ] && newer_commit_date; then
    info "Downloading newest version of updater script."

    ndtmpdir=$(create_exec_tmp_directory)
    cd "$ndtmpdir" || exit 1

    if _safe_download "https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/netdata-updater.sh" ./netdata-updater.sh; then
      chmod +x ./netdata-updater.sh || exit 1
      export ENVIRONMENT_FILE="${ENVIRONMENT_FILE}"

      cmd="./netdata-updater.sh --not-running-from-cron --no-updater-self-update"
      [ "$NETDATA_FORCE_UPDATE" = "1" ] && cmd="$cmd --force-update"
      [ "$INTERACTIVE" = "0" ] && cmd="$cmd --non-interactive"
      cmd="$cmd --tmpdir-path $(pwd)"

      exec $cmd
    else
      error "Failed to download newest version of updater script, continuing with current version."
    fi
  fi
}

parse_version() {
  r="${1}"
  if [ "${r}" = "latest" ]; then
    # If we get ‘latest’ as a version, return the largest possible
    # version value.
    printf "999999999999999"
    return 0
  elif echo "${r}" | grep -q '^v.*'; then
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

  printf "%04d%03d%03d%05d" "${maj}" "${min}" "${patch}" "${b}"
}

get_latest_tag() {
  if [ -z "${_latest_tag}" ]; then
    if [ "${RELEASE_CHANNEL}" = "stable" ]; then
        _latest_tag="$(get_netdata_latest_tag "${NETDATA_STABLE_BASE_URL}")"
    else
        _latest_tag="$(get_netdata_latest_tag "${NETDATA_NIGHTLY_BASE_URL}")"
    fi
  fi

  echo "${_latest_tag}"
}

validate_environment_file() {
  if [ -n "${NETDATA_PREFIX+SET_BUT_NULL}" ] && [ -n "${REINSTALL_OPTIONS+SET_BUT_NULL}" ]; then
    return 0
  else
    fatal "Environment file located at ${ENVIRONMENT_FILE} is not valid, unable to update." U0007
  fi
}

get_current_version() {
  basepath="$(dirname "$(dirname "$(dirname "${NETDATA_LIB_DIR}")")")"
  searchpath="${basepath}/bin:${basepath}/sbin:${basepath}/usr/bin:${basepath}/usr/sbin:${PATH}"
  searchpath="${basepath}/netdata/bin:${basepath}/netdata/sbin:${basepath}/netdata/usr/bin:${basepath}/netdata/usr/sbin:${searchpath}"
  ndbinary="$(PATH="${searchpath}" command -v netdata 2>/dev/null)"

  if [ -z "${ndbinary}" ]; then
    _current_version=0
  else
    _current_version="$(parse_version "$(${ndbinary} -V | cut -f 2 -d ' ')")"
  fi

  echo "${_current_version:-0}"
}

get_latest_version() {
  parse_version "$(get_latest_tag)"
}

update_available() {
  if [ "$NETDATA_FORCE_UPDATE" = "1" ]; then
     info "Force update requested"
     return 0
  fi

  current_version="$(get_current_version)"
  latest_version="$(get_latest_version)"

  info "Current Version: ${current_version}"
  info "Latest Version: ${latest_version}"

  if [ -z "${latest_version}" ] || [ -z "${current_version}" ] ; then
    info "Unable to compare versions for update check, assuming an update is required."
    return 0
  elif [ "${latest_version}" -gt 0 ] && [ "${current_version}" -gt 0 ] && [ "${current_version}" -ge "${latest_version}" ]; then
    info "Newest version (current=${current_version} >= latest=${latest_version}) is already installed"
    return 1
  else
    info "Update available"

    if [ "${current_version}" -ne 0 ] && [ "${latest_version}" -ne 0 ]; then
      current_major="$(echo "${current_version}" | head -c 4)"
      latest_major="$(echo "${latest_version}" | head -c 4)"

      if [ "${current_major}" -ne "${latest_major}" ]; then
        update_safe=0

        for v in ${NETDATA_ACCEPT_MAJOR_VERSIONS}; do
          if [ "${latest_major}" -eq "${v}" ]; then
            update_safe=1
            break
          fi
        done

        if [ "${update_safe}" -eq 0 ]; then
          warn_major_update
        fi
      fi
    fi

    return 0
  fi
}

set_tarball_urls() {
  filename="netdata-latest.tar.gz"

  if [ "$2" = "yes" ]; then
    if [ -e /opt/netdata/etc/netdata/.install-type ]; then
      # shellcheck disable=SC1091
      . /opt/netdata/etc/netdata/.install-type
      [ -z "${PREBUILT_ARCH:-}" ] && PREBUILT_ARCH="$(uname -m)"
      filename="netdata-${PREBUILT_ARCH}-latest.gz.run"
    else
      filename="netdata-x86_64-latest.gz.run"
    fi
  fi

  if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ]; then
    path="$(cd "${NETDATA_OFFLINE_INSTALL_SOURCE}" || exit 1; pwd)"
    export NETDATA_TARBALL_URL="file://${path}/${filename}"
    export NETDATA_TARBALL_CHECKSUM_URL="file://${path}/sha256sums.txt"
  elif [ "$1" = "stable" ]; then
    latest="$(get_netdata_latest_tag "${NETDATA_STABLE_BASE_URL}")"
    export NETDATA_TARBALL_URL="${NETDATA_STABLE_BASE_URL}/download/$latest/${filename}"
    export NETDATA_TARBALL_CHECKSUM_URL="${NETDATA_STABLE_BASE_URL}/download/$latest/sha256sums.txt"
  else
    tag="$(get_netdata_latest_tag "${NETDATA_NIGHTLY_BASE_URL}")"
    export NETDATA_TARBALL_URL="${NETDATA_NIGHTLY_BASE_URL}/download/${tag}/${filename}"
    export NETDATA_TARBALL_CHECKSUM_URL="${NETDATA_NIGHTLY_BASE_URL}/download/${tag}/sha256sums.txt"
  fi
}

update_build() {
  [ -z "${logfile}" ] && info "Running on a terminal - (this script also supports running headless from crontab)"

  RUN_INSTALLER=0
  ndtmpdir=$(create_exec_tmp_directory)
  cd "$ndtmpdir" || fatal "Failed to change current working directory to ${ndtmpdir}" U0016

  install_build_dependencies

  if update_available; then
    download "${NETDATA_TARBALL_CHECKSUM_URL}" "${ndtmpdir}/sha256sum.txt" >&3 2>&3
    download "${NETDATA_TARBALL_URL}" "${ndtmpdir}/netdata-latest.tar.gz"
    if [ -n "${NETDATA_TARBALL_CHECKSUM}" ] &&
      grep "${NETDATA_TARBALL_CHECKSUM}" sha256sum.txt >&3 2>&3 &&
      [ "$NETDATA_FORCE_UPDATE" != "1" ]; then
      info "Newest version is already installed"
    else
      if ! grep netdata-latest.tar.gz sha256sum.txt | safe_sha256sum -c - >&3 2>&3; then
        fatal "Tarball checksum validation failed. Stopping netdata upgrade and leaving tarball in ${ndtmpdir}\nUsually this is a result of an older copy of the tarball or checksum file being cached somewhere upstream and can be resolved by retrying in an hour." U0008
      fi
      NEW_CHECKSUM="$(safe_sha256sum netdata-latest.tar.gz 2> /dev/null | cut -d' ' -f1)"
      tar -xf netdata-latest.tar.gz >&3 2>&3
      rm netdata-latest.tar.gz >&3 2>&3
      if [ -z "$path_version" ]; then
        latest_tag="$(get_latest_tag)"
        path_version="$(echo "${latest_tag}" | cut -f 1 -d "-")"
      fi
      cd "$(find . -maxdepth 1 -type d -name "netdata-${path_version}*" | head -n 1)" || fatal "Failed to switch to build directory" U0017
      RUN_INSTALLER=1
    fi
  fi

  # We got the sources, run the update now
  if [ ${RUN_INSTALLER} -eq 1 ]; then
    # signal netdata to start saving its database
    # this is handy if your database is big
    possible_pids=$(pidof netdata)
    do_not_start=
    if [ -n "${possible_pids}" ]; then
      # shellcheck disable=SC2086
      kill -USR1 ${possible_pids}
    else
      # netdata is currently not running, so do not start it after updating
      do_not_start="--dont-start-it"
    fi

    env="env TMPDIR=${TMPDIR}"

    if [ -n "${NETDATA_SELECTED_DASHBOARD}" ]; then
      env="${env} NETDATA_SELECTED_DASHBOARD=${NETDATA_SELECTED_DASHBOARD}"
    fi

    if [ ! -x ./netdata-installer.sh ]; then
      if [ "$(find . -mindepth 1 -maxdepth 1 -type d | wc -l)" -eq 1 ] && [ -x "$(find . -mindepth 1 -maxdepth 1 -type d)/netdata-installer.sh" ]; then
        cd "$(find . -mindepth 1 -maxdepth 1 -type d)" || fatal "Failed to switch to build directory" U0018
      fi
    fi

    if [ -e "${NETDATA_PREFIX}/etc/netdata/.install-type" ] ; then
      install_type="$(cat "${NETDATA_PREFIX}"/etc/netdata/.install-type)"
    else
      install_type="INSTALL_TYPE='legacy-build'"
    fi

    if [ "${INSTALL_TYPE}" = "custom" ] && [ -f "${NETDATA_PREFIX}" ]; then
      install_type="INSTALL_TYPE='legacy-build'"
    fi

    info "Re-installing netdata..."
    export NETDATA_SAVE_WARNINGS=1
    export NETDATA_PROPAGATE_WARNINGS=1
    export NETDATA_WARNINGS="${NETDATA_WARNINGS}"
    export NETDATA_SCRIPT_STATUS_PATH="${NETDATA_SCRIPT_STATUS_PATH}"
    # shellcheck disable=SC2086
    if ! ${env} ./netdata-installer.sh ${REINSTALL_OPTIONS} --dont-wait ${do_not_start} >&3 2>&3; then
      if [ -r "${NETDATA_SCRIPT_STATUS_PATH}" ]; then
        # shellcheck disable=SC1090
        . "${NETDATA_SCRIPT_STATUS_PATH}"
        rm -f "${NETDATA_SCRIPT_STATUS_PATH}"
      fi
      if [ -n "${EXIT_REASON}" ]; then
        fatal "Failed to rebuild existing netdata install: ${EXIT_REASON}" "U${EXIT_CODE}"
      else
        fatal "Failed to rebuild existing netdata reinstall." UI0000
      fi
    fi

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

update_static() {
  ndtmpdir="$(create_exec_tmp_directory)"
  PREVDIR="$(pwd)"

  info "Entering ${ndtmpdir}"
  cd "${ndtmpdir}" || fatal "Failed to change current working directory to ${ndtmpdir}" U0019

  if update_available; then
    sysarch="${PREBUILT_ARCH}"
    [ -z "$sysarch" ] && sysarch="$(uname -m)"
    download "${NETDATA_TARBALL_CHECKSUM_URL}" "${ndtmpdir}/sha256sum.txt"
    download "${NETDATA_TARBALL_URL}" "${ndtmpdir}/netdata-${sysarch}-latest.gz.run"
    if ! grep "netdata-${sysarch}-latest.gz.run" "${ndtmpdir}/sha256sum.txt" | safe_sha256sum -c - > /dev/null 2>&1; then
      fatal "Static binary checksum validation failed. Stopping netdata installation and leaving binary in ${ndtmpdir}\nUsually this is a result of an older copy of the file being cached somewhere and can be resolved by simply retrying in an hour." U000A
    fi

    if [ -e /opt/netdata/etc/netdata/.install-type ] ; then
      install_type="$(cat /opt/netdata/etc/netdata/.install-type)"
    else
      install_type="INSTALL_TYPE='legacy-static'"
    fi

    # Do not pass any options other than the accept, for now
    # shellcheck disable=SC2086
    if sh "${ndtmpdir}/netdata-${sysarch}-latest.gz.run" --accept -- ${REINSTALL_OPTIONS} >&3 2>&3; then
      rm -r "${ndtmpdir}"
    else
      info "NOTE: did not remove: ${ndtmpdir}"
    fi

    echo "${install_type}" > /opt/netdata/etc/netdata/.install-type
  fi

  if [ -e "${PREVDIR}" ]; then
    info "Switching back to ${PREVDIR}"
    cd "${PREVDIR}"
  fi
  [ -n "${logfile}" ] && rm "${logfile}" && logfile=

  return 0
}

get_new_binpkg_major() {
  case "${pm_cmd}" in
    apt-get) apt-get --just-print upgrade 2>&1 | grep Inst | grep ' netdata ' | cut -f 3 -d ' ' | tr -d '[]' | cut -f 1 -d '.' ;;
    yum) yum check-update netdata | grep -E '^netdata ' | awk '{print $2}' | cut -f 1 -d '.' ;;
    dnf) dnf check-update netdata | grep -E '^netdata ' | awk '{print $2}' | cut -f 1 -d '.' ;;
    zypper) zypper list-updates | grep '| netdata |' | cut -f 5 -d '|' | tr -d ' ' | cut -f 1 -d '.' ;;
  esac
}

update_binpkg() {
  os_release_file=
  if [ -s "/etc/os-release" ] && [ -r "/etc/os-release" ]; then
    os_release_file="/etc/os-release"
  elif [ -s "/usr/lib/os-release" ] && [ -r "/usr/lib/os-release" ]; then
    os_release_file="/usr/lib/os-release"
  else
    fatal "Cannot find an os-release file ..." U000B
  fi

  # shellcheck disable=SC1090
  . "${os_release_file}"

  DISTRO="${ID}"
  SYSVERSION="${VERSION_ID}"

  supported_compat_names="debian ubuntu centos fedora opensuse ol amzn"

  if str_in_list "${DISTRO}" "${supported_compat_names}"; then
    DISTRO_COMPAT_NAME="${DISTRO}"
  else
    case "${DISTRO}" in
      opensuse-leap|opensuse-tumbleweed)
        DISTRO_COMPAT_NAME="opensuse"
        ;;
      cloudlinux|almalinux|centos-stream|rocky|rhel)
        DISTRO_COMPAT_NAME="centos"
        ;;
      raspbian)
        SYSARCH="$(uname -m)"
        if [ "$SYSARCH" = "armv7l" ] || [ "$SYSARCH" = "aarch64" ]; then
          DISTRO_COMPAT_NAME="debian"
        fi
        ;;
      *)
        DISTRO_COMPAT_NAME="unknown"
        ;;
    esac
  fi

  interactive_opts=""
  env=""

  case "${DISTRO_COMPAT_NAME}" in
    debian|ubuntu)
      if [ "${INTERACTIVE}" = "0" ]; then
        upgrade_subcmd="-o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold --only-upgrade install"
        interactive_opts="-y"
        env="DEBIAN_FRONTEND=noninteractive"
      else
        upgrade_subcmd="--only-upgrade install"
      fi
      pm_cmd="apt-get"
      repo_subcmd="update"
      install_subcmd="install"
      mark_auto_cmd="apt-mark auto"
      pkg_install_opts="${interactive_opts}"
      repo_update_opts="${interactive_opts}"
      pkg_installed_check="dpkg-query -s"
      INSTALL_TYPE="binpkg-deb"
      if [ -n "${VERSION_CODENAME}" ]; then
        repo_path="${DISTRO_COMPAT_NAME}/${VERSION_CODENAME}"
      fi
      ;;
    centos|fedora|ol|amzn)
      if [ "${INTERACTIVE}" = "0" ]; then
        interactive_opts="-y"
      fi
      if command -v dnf > /dev/null; then
        pm_cmd="dnf"
        repo_subcmd="makecache"
        mark_auto_cmd="dnf mark remove"
      else
        pm_cmd="yum"
        mark_auto_cmd="yumdb set reason dep"
      fi
      upgrade_subcmd="upgrade"
      install_subcmd="install"
      pkg_install_opts="${interactive_opts}"
      repo_update_opts="${interactive_opts}"
      pkg_installed_check="rpm -q"
      INSTALL_TYPE="binpkg-rpm"
      case "${DISTRO_COMPAT_NAME}" in
        amzn) repo_path="amazonlinux/${SYSVERSION}/$(uname -m)" ;;
        fedora) repo_path="fedora/${SYSVERSION}/$(uname -m)" ;;
        ol) repo_path="ol/${SYSVERSION}/$(uname -m)" ;;
        *) repo_path="el/${SYSVERSION}/$(uname -m)" ;;
      esac
      ;;
    opensuse)
      if [ "${INTERACTIVE}" = "0" ]; then
        upgrade_subcmd="--non-interactive update"
      else
        upgrade_subcmd="update"
      fi
      pm_cmd="zypper"
      repo_subcmd="--gpg-auto-import-keys refresh"
      install_subcmd="install"
      mark_auto_cmd=""
      pkg_install_opts=""
      repo_update_opts=""
      pkg_installed_check="rpm -q"
      INSTALL_TYPE="binpkg-rpm"
      repo_path="${DISTRO_COMPAT_NAME}/${SYSVERSION}/$(uname -m)"
      ;;
    *)
      warning "We do not provide native packages for ${DISTRO}."
      return 2
      ;;
  esac

  initial_version="$(get_current_version)"

  if [ -n "${repo_subcmd}" ]; then
    # shellcheck disable=SC2086
    env ${env} ${pm_cmd} ${repo_subcmd} ${repo_update_opts} >&3 2>&3 || fatal "Failed to update repository metadata." U000C
  fi

  if ${pkg_installed_check} netdata-repo > /dev/null 2>&1; then
    RELEASE_CHANNEL="stable"
    repopkg="netdata-repo"
  elif ${pkg_installed_check} netdata-repo-edge > /dev/null 2>&1; then
    RELEASE_CHANNEL="nightly"
    repopkg="netdata-repo-edge"
  elif echo "${initial_version}" | grep -Eq -- '^[0-9]*[1-9][0-9]*0{5}$'; then # All five final digits are zero and at least one preceeding digit is non-zero.
    RELEASE_CHANNEL="stable"
  elif echo "${initial_version}" | grep -Eq -- '^[0-9]*[1-9][0-9]{0,4}$'; then # At least one of the final five digits is non-zero.
    RELEASE_CHANNEL="nightly"
  else
    RELEASE_CHANNEL="none"
    warning "Unable to determine which release channel is being used on this system, cannot check if packages are still being published."
  fi

  if [ -n "${repo_path}" ]; then
    case "${RELEASE_CHANNEL}" in
      stable) check_url="${NETDATA_STABLE_REPO_URL}/${repo_path}/.currently.published" ;;
      nightly) check_url="${NETDATA_NIGHTLY_REPO_URL}/${repo_path}/.currently.published" ;;
    esac
  fi

  if [ -n "${check_url}" ]; then
    info "Checking if native packages are still being published for this platform."

    set +e
    _safe_download "${check_url}" /dev/null
    ret=$?
    set -e

    case "${ret}" in
      0) info "Native packages are still being published for this platform." ;;
      1)
        error ""
        error "NETDATA CANNOT BE UPDATED ON THIS SYSTEM!"
        error ""
        error "Native packages are no longer being published for this platform."
        error ""
        error "To update to the latest version of Netdata, you will need to switch to a different install type."
        error "For details on how to do so, see https://learn.netdata.cloud/docs/netdata-agent/installation/linux/switch-install-types-and-release-channels"
        error ""
        fatal "Unable to update due to native packages no longer being published for this platform" U001E
        ;;
      255) warning "Unable to check whether native packages are being published, wget or curl is required." ;;
    esac
  fi

  if [ -n "${repopkg}" ]; then
    # shellcheck disable=SC2086
    env ${env} ${pm_cmd} ${upgrade_subcmd} ${pkg_install_opts} ${repopkg} >&3 2>&3 || fatal "Failed to update Netdata repository config." U000D
    # shellcheck disable=SC2086
    if [ -n "${repo_subcmd}" ]; then
      env ${env} ${pm_cmd} ${repo_subcmd} ${repo_update_opts} >&3 2>&3 || fatal "Failed to update repository metadata." U000E
    fi
  fi

  current_major="$(echo "${nd_version}" | cut -f 1 -d '.' | tr -d 'v')"
  latest_major="$(get_new_binpkg_major)"

  if [ -n "${latest_major}" ] && [ "${latest_major}" -ne "${current_major}" ]; then
    update_safe=0

    for v in ${NETDATA_ACCEPT_MAJOR_VERSIONS}; do
      if [ "${latest_major}" -eq "${v}" ]; then
        update_safe=1
        break
      fi
    done

    if [ "${update_safe}" -eq 0 ]; then
      warn_major_update
    fi
  fi

  # shellcheck disable=SC2086
  env ${env} ${pm_cmd} ${upgrade_subcmd} ${pkg_install_opts} netdata >&3 2>&3 || fatal "Failed to update Netdata package." U000F

  if ${pkg_installed_check} systemd > /dev/null 2>&1; then
    if [ "${NETDATA_NO_SYSTEMD_JOURNAL}" -eq 0 ]; then
      if ! ${pkg_installed_check} netdata-plugin-systemd-journal > /dev/null 2>&1; then
        env ${env} ${pm_cmd} ${install_subcmd} ${pkg_install_opts} netdata-plugin-systemd-journal >&3 2>&3

        if [ -n "${mark_auto_cmd}" ]; then
          # shellcheck disable=SC2086
          env ${env} ${mark_auto_cmd} netdata-plugin-systemd-journal >&3 2>&3
        fi
      fi
    fi
  fi

  current_version="$(get_current_version)"
  latest_version="$(get_latest_version)"

  if [ "${RELEASE_CHANNEL}" != "none" ] && [ "${current_version}" -ne 0 ] && [ "${latest_version}" -ne 0 ]; then
    if [ "${current_version}" -lt "${latest_version}" ] && [ "${initial_version}" -eq "${current_version}" ]; then
      error ""
      error "NETDATA WAS NOT UPDATED!"
      error ""
      error "A newer version of Netdata is available, but the system package manager does not appear to have updated to that version."
      error ""
      error "Most likely, your system is not up to date, and you have it configured in a way that prevents updating one or more of Netdata's dependencies."
      error "Please try updating your system manually and then re-running the Netdata updater before reporting an issue with the update process."
      error ""
      fatal "Package manager did not fully update Netdata despite not reporting a failure." U001D
    fi
  fi

  [ -n "${logfile}" ] && rm "${logfile}" && logfile=
  return 0
}

# Simple function to encapsulate original updater behavior.
update_legacy() {
  set_tarball_urls "${RELEASE_CHANNEL}" "${IS_NETDATA_STATIC_BINARY}"
  case "${IS_NETDATA_STATIC_BINARY}" in
    yes) update_static && exit 0 ;;
    *) update_build && exit 0 ;;
  esac
}

logfile=
ndtmpdir=

trap cleanup EXIT

if [ -t 2 ] || [ "${GITHUB_ACTIONS}" ]; then
  # we are running on a terminal or under CI
  # open fd 3 and send it to stderr
  exec 3>&2
else
  # we are headless
  # create a temporary file for the log
  logfile="$(mktemp -t netdata-updater.log.XXXXXX)"
  # open fd 3 and send it to logfile
  exec 3> "${logfile}"
fi

: "${ENVIRONMENT_FILE:=THIS_SHOULD_BE_REPLACED_BY_INSTALLER_SCRIPT}"

if [ "${ENVIRONMENT_FILE}" = "THIS_SHOULD_BE_REPLACED_BY_INSTALLER_SCRIPT" ]; then
  if [ -r "${script_dir}/../../../etc/netdata/.environment" ] || [ -r "${script_dir}/../../../etc/netdata/.install-type" ]; then
    ENVIRONMENT_FILE="${script_dir}/../../../etc/netdata/.environment"
  elif [ -r "/etc/netdata/.environment" ] || [ -r "/etc/netdata/.install-type" ]; then
    ENVIRONMENT_FILE="/etc/netdata/.environment"
  elif [ -r "/opt/netdata/etc/netdata/.environment" ] || [ -r "/opt/netdata/etc/netdata/.install-type" ]; then
    ENVIRONMENT_FILE="/opt/netdata/etc/netdata/.environment"
  else
    envpath="$(find / -type d \( -path /sys -o -path /proc -o -path /dev \) -prune -false -o -path '*netdata/.environment' -type f  2> /dev/null | head -n 1)"
    itpath="$(find / -type d \( -path /sys -o -path /proc -o -path /dev \) -prune -false -o -path '*netdata/.install-type' -type f  2> /dev/null | head -n 1)"
    if [ -r "${envpath}" ]; then
      ENVIRONMENT_FILE="${envpath}"
    elif [ -r "${itpath}" ]; then
      ENVIRONMENT_FILE="$(dirname "${itpath}")/.environment"
    else
      fatal "Cannot find environment file or install type file, unable to update." U0010
    fi
  fi
fi

if [ -r "${ENVIRONMENT_FILE}" ] ; then
  # shellcheck source=/dev/null
  . "${ENVIRONMENT_FILE}" || fatal "Failed to source ${ENVIRONMENT_FILE}" U0014
fi

if [ -r "$(dirname "${ENVIRONMENT_FILE}")/.install-type" ]; then
  # shellcheck source=/dev/null
  . "$(dirname "${ENVIRONMENT_FILE}")/.install-type" || fatal "Failed to source $(dirname "${ENVIRONMENT_FILE}")/.install-type" U0015
fi

UPDATER_CONFIG_PATH="$(dirname "${ENVIRONMENT_FILE}")/netdata-updater.conf"
if [ -r "${UPDATER_CONFIG_PATH}" ]; then
  # shellcheck source=/dev/null
  . "${UPDATER_CONFIG_PATH}"
fi

[ -z "${NETDATA_ACCEPT_MAJOR_VERSIONS}" ] && NETDATA_ACCEPT_MAJOR_VERSIONS="${NETDATA_DEFAULT_ACCEPT_MAJOR_VERSIONS}"

while [ -n "${1}" ]; do
  case "${1}" in
    --not-running-from-cron) NETDATA_NOT_RUNNING_FROM_CRON=1 ;;
    --no-updater-self-update) NETDATA_NO_UPDATER_SELF_UPDATE=1 ;;
    --force-update) NETDATA_FORCE_UPDATE=1 ;;
    --non-interactive) INTERACTIVE=0 ;;
    --interactive) INTERACTIVE=1 ;;
    --offline-install-source)
      NETDATA_OFFLINE_INSTALL_SOURCE="${2}"
      shift 1
      ;;
    --tmpdir-path)
      NETDATA_TMPDIR_PATH="${2}"
      shift 1
      ;;
    --enable-auto-updates)
      enable_netdata_updater "${2}"
      exit $?
      ;;
    --disable-auto-updates)
      disable_netdata_updater
      exit $?
      ;;
    --auto-update-status)
      auto_update_status
      exit 0
      ;;
    *) fatal "Unrecognized option ${1}" U001A ;;
  esac

  shift 1
done

if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ]; then
  NETDATA_NO_UPDATER_SELF_UPDATE=1
  NETDATA_UPDATER_JITTER=0
  NETDATA_FORCE_UPDATE=1
fi

# If we seem to be running under anacron, act as if we’re not running from cron.
# This is mostly to disable jitter, which should not be needed when run from anacron.
if running_under_anacron; then
  NETDATA_NOT_RUNNING_FROM_CRON="${NETDATA_NOT_RUNNING_FROM_CRON:-1}"
fi

# Random sleep to alleviate stampede effect of Agents upgrading
# and disconnecting/reconnecting at the same time (or near to).
# But only we're not a controlling terminal (tty)
# Randomly sleep between 1s and 60m
if [ ! -t 1 ] && \
   [ -z "${GITHUB_ACTIONS}" ] && \
   [ -z "${NETDATA_NOT_RUNNING_FROM_CRON}" ] && \
   is_integer "${NETDATA_UPDATER_JITTER}" && \
   [ "${NETDATA_UPDATER_JITTER}" -gt 1 ]; then
    rnd="$(awk "
      BEGIN { srand()
              printf(\"%d\\n\", ${NETDATA_UPDATER_JITTER} * rand())
      }")"
    sleep $(((rnd % NETDATA_UPDATER_JITTER) + 1))
fi

# We dont expect to find lib dir variable on older installations, so load this path if none found
export NETDATA_LIB_DIR="${NETDATA_LIB_DIR:-${NETDATA_PREFIX}/var/lib/netdata}"

# Source the tarball checksum, if not already available from environment (for existing installations with the old logic)
[ -z "${NETDATA_TARBALL_CHECKSUM}" ] && [ -f "${NETDATA_LIB_DIR}/netdata.tarball.checksum" ] && NETDATA_TARBALL_CHECKSUM="$(cat "${NETDATA_LIB_DIR}/netdata.tarball.checksum")"

if echo "$INSTALL_TYPE" | grep -qv ^binpkg && [ "${INSTALL_UID}" != "$(id -u)" ]; then
  fatal "You are running this script as user with uid $(id -u). We recommend to run this script as root (user with uid 0)" U0011
fi

self_update

# shellcheck disable=SC2153
case "${INSTALL_TYPE}" in
    *-build)
      validate_environment_file
      set_tarball_urls "${RELEASE_CHANNEL}" "${IS_NETDATA_STATIC_BINARY}"
      update_build && exit 0
      ;;
    *-static*)
      validate_environment_file
      set_tarball_urls "${RELEASE_CHANNEL}" "${IS_NETDATA_STATIC_BINARY}"
      update_static && exit 0
      ;;
    *binpkg*) update_binpkg && exit 0 ;;
    "") # Fallback case for no `.install-type` file. This just works like the old install type detection.
      validate_environment_file
      update_legacy
      ;;
    custom)
      # At this point, we _should_ have a valid `.environment` file, but it's best to just check.
      # If we do, then behave like the legacy updater.
      if validate_environment_file && [ -n "${IS_NETDATA_STATIC_BINARY}" ]; then
        update_legacy
      else
        fatal "This script does not support updating custom installations without valid environment files." U0012
      fi
      ;;
    oci) fatal "This script does not support updating Netdata inside our official Docker containers, please instead update the container itself." U0013 ;;
    *) fatal "Unrecognized installation type (${INSTALL_TYPE}), unable to update." U0014 ;;
esac
