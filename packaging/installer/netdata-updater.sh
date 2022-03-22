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

PACKAGES_SCRIPT="https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/install-required-packages.sh"

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

info() {
  echo >&3 "$(date) : INFO: " "${@}"
}

error() {
  echo >&3 "$(date) : ERROR: " "${@}"
}

fatal() {
  error "FAILED TO UPDATE NETDATA : ${1}"
  exit 1
}

issystemd() {
  # if the directory /lib/systemd/system OR /usr/lib/systemd/system (SLES 12.x) does not exit, it is not systemd
  if [ ! -d /lib/systemd/system ] && [ ! -d /usr/lib/systemd/system ]; then
    return 1
  fi

  # if there is no systemctl command, it is not systemd
  systemctl=$(command -v systemctl 2> /dev/null)
  if [ -z "${systemctl}" ] || [ ! -x "${systemctl}" ]; then
    return 1
  fi

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

install_build_dependencies() {
  bash="$(command -v bash 2> /dev/null)"

  if [ -z "${bash}" ] || [ ! -x "${bash}" ]; then
    error "Unable to find a usable version of \`bash\` (required for local build)."
    return 1
  fi

  info "Fetching dependency handling script..."
  download "${PACKAGES_SCRIPT}" "${TMPDIR}/install-required-packages.sh" || true

  if [ ! -s "${TMPDIR}/install-required-packages.sh" ]; then
    error "Downloaded dependency installation script is empty."
  else
    info "Running dependency handling script..."

    opts="--dont-wait --non-interactive"

    # shellcheck disable=SC2086
    if ! "${bash}" "${TMPDIR}/install-required-packages.sh" ${opts} netdata >&3 2>&3; then
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
      error "Unrecognized updater type ${updater_type} requested. Supported types are 'systemd', 'interval', and 'crontab'."
      exit 1
      ;;
  esac

  case "${updater_type}" in
    "systemd")
      if issystemd; then
        systemctl enable netdata-updater.timer

        info "Auto-updating has been ENABLED using a systemd timer unit.\n"
        info "If the update process fails, the failure will be logged to the systemd journal just like a regular service failure."
        info "Successful updates should produce empty logs."
      else
        error "Systemd-based auto-update scheduling requested, but this does not appear to be a systemd system."
        error "Auto-updates have NOT been enabled."
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
        error "Interval-based auto-update scheduling requested, but I could not find an interval scheduling directory."
        error "Auto-updates have NOT been enabled."
        return 1
      fi
      ;;
    "crontab")
      if [ -d "/etc/cron.d" ]; then
        cat > "/etc/cron.d/netdata-updater" <<-EOF
	2 57 * * * root ${NETDATA_PREFIX}/netdata-updater.sh
	EOF

        info "Auto-updating has been ENABLED through cron, using a crontab at /etc/cron.d/netdata-updater\n"
        info "If the update process fails and you have email notifications set up correctly for cron on this system, you should receive an email notification of the failure."
        info "Successful updates will not send an email."
      else
        error "Crontab-based auto-update scheduling requested, but there is no '/etc/cron.d'."
        error "Auto-updates have NOT been enabled."
        return 1
      fi
      ;;
    *)
      error "Unable to determine what type of auto-update scheduling to use."
      error "Auto-updates have NOT been enabled."
      return 1
  esac

  return 0
}

disable_netdata_updater() {
  if issystemd && ( systemctl list-units --full -all | grep -Fq "netdata-updater.timer" ) ; then
    systemctl disable netdata-updater.timer
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
  fi

  info "Auto-updates have been DISABLED."

  return 0
}

str_in_list() {
  printf "%s\n" "${2}" | tr ' ' "\n" | grep -qE "^${1}\$"
  return $?
}

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

create_tmp_directory() {
  if [ -n "${NETDATA_TMPDIR_PATH}" ]; then
    echo "${NETDATA_TMPDIR_PATH}"
  else
    if [ -z "${NETDATA_TMPDIR}" ] || _cannot_use_tmpdir "${NETDATA_TMPDIR}" ; then
      if [ -z "${TMPDIR}" ] || _cannot_use_tmpdir "${TMPDIR}" ; then
        if _cannot_use_tmpdir /tmp ; then
          if _cannot_use_tmpdir "${PWD}" ; then
            fatal "Unable to find a usable temporary directory. Please set \$TMPDIR to a path that is both writable and allows execution of files and try again."
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

  echo "${tag}" >"${dest}"
}

newer_commit_date() {
  info "Checking if a newer version of the updater script is available."

  commit_check_url="https://api.github.com/repos/netdata/netdata/commits?path=packaging%2Finstaller%2Fnetdata-updater.sh&page=1&per_page=1"
  python_version_check="from __future__ import print_function;import sys,json;data = json.load(sys.stdin);print(data[0]['commit']['committer']['date'] if isinstance(data, list) else '')"

  if command -v jq > /dev/null 2>&1; then
    commit_date="$(_safe_download "${commit_check_url}" /dev/stdout | jq '.[0].commit.committer.date' 2>/dev/null | tr -d '"')"
  elif command -v python > /dev/null 2>&1;then
    commit_date="$(_safe_download "${commit_check_url}" /dev/stdout | python -c "${python_version_check}")"
  elif command -v python3 > /dev/null 2>&1;then
    commit_date="$(_safe_download "${commit_check_url}" /dev/stdout | python3 -c "${python_version_check}")"
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

    ndtmpdir=$(create_tmp_directory)
    cd "$ndtmpdir" || exit 1

    if _safe_download "https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/netdata-updater.sh" ./netdata-updater.sh; then
      chmod +x ./netdata-updater.sh || exit 1
      export ENVIRONMENT_FILE="${ENVIRONMENT_FILE}"
      exec ./netdata-updater.sh --not-running-from-cron --no-updater-self-update --tmpdir-path "$(pwd)"
    else
      error "Failed to download newest version of updater script, continuing with current version."
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

validate_environment_file() {
  if [ -n "${RELEASE_CHANNEL}" ] && [ -n "${NETDATA_PREFIX}" ] && [ -n "${REINSTALL_OPTIONS}" ] && [ -n "${IS_NETDATA_STATIC_BINARY}" ]; then
    return 0
  else
    error "Environment file located at ${ENVIRONMENT_FILE} is not valid, unable to update."
  fi
}

update_available() {
  basepath="$(dirname "$(dirname "$(dirname "${NETDATA_LIB_DIR}")")")"
  searchpath="${basepath}/bin:${basepath}/sbin:${basepath}/usr/bin:${basepath}/usr/sbin:${PATH}"
  searchpath="${basepath}/netdata/bin:${basepath}/netdata/sbin:${basepath}/netdata/usr/bin:${basepath}/netdata/usr/sbin:${searchpath}"
  ndbinary="$(PATH="${searchpath}" command -v netdata 2>/dev/null)"

  if [ -z "${ndbinary}" ]; then
    current_version=0
  else
    current_version="$(parse_version "$(${ndbinary} -v | cut -f 2 -d ' ')")"
  fi

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
    return 1
  else
    info "Update available"
    return 0
  fi
}

set_tarball_urls() {
  filename="netdata-latest.tar.gz"

  if [ "$2" = "yes" ]; then
    if [ -e /opt/netdata/etc/netdata/.install-type ]; then
      # shellcheck disable=SC1091
      . /opt/netdata/etc/netdata/.install-type
      filename="netdata-${PREBUILT_ARCH}-latest.gz.run"
    else
      filename="netdata-x86_64-latest.gz.run"
    fi
  fi

  if [ "$1" = "stable" ]; then
    latest="$(get_netdata_latest_tag /dev/stdout)"
    export NETDATA_TARBALL_URL="https://github.com/netdata/netdata/releases/download/$latest/${filename}"
    export NETDATA_TARBALL_CHECKSUM_URL="https://github.com/netdata/netdata/releases/download/$latest/sha256sums.txt"
  else
    export NETDATA_TARBALL_URL="$NETDATA_NIGHTLIES_BASEURL/${filename}"
    export NETDATA_TARBALL_CHECKSUM_URL="$NETDATA_NIGHTLIES_BASEURL/sha256sums.txt"
  fi
}

update_build() {
  [ -z "${logfile}" ] && info "Running on a terminal - (this script also supports running headless from crontab)"

  install_build_dependencies

  RUN_INSTALLER=0
  ndtmpdir=$(create_tmp_directory)
  cd "$ndtmpdir" || exit 1

  if update_available; then
    download "${NETDATA_TARBALL_CHECKSUM_URL}" "${ndtmpdir}/sha256sum.txt" >&3 2>&3
    download "${NETDATA_TARBALL_URL}" "${ndtmpdir}/netdata-latest.tar.gz"
    if [ -n "${NETDATA_TARBALL_CHECKSUM}" ] && grep "${NETDATA_TARBALL_CHECKSUM}" sha256sum.txt >&3 2>&3; then
      info "Newest version is already installed"
    else
      if ! grep netdata-latest.tar.gz sha256sum.txt | safe_sha256sum -c - >&3 2>&3; then
        fatal "Tarball checksum validation failed. Stopping netdata upgrade and leaving tarball in ${ndtmpdir}\nUsually this is a result of an older copy of the tarball or checksum file being cached somewhere upstream and can be resolved by retrying in an hour."
      fi
      NEW_CHECKSUM="$(safe_sha256sum netdata-latest.tar.gz 2> /dev/null | cut -d' ' -f1)"
      tar -xf netdata-latest.tar.gz >&3 2>&3
      rm netdata-latest.tar.gz >&3 2>&3
      cd "$(find . -maxdepth 1 -name "netdata-${path_version}*" | head -n 1)" || exit 1
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

    if [ "${INSTALL_TYPE}" = "custom" ] && [ -f "${NETDATA_PREFIX}" ]; then
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

update_static() {
  ndtmpdir="$(create_tmp_directory)"
  PREVDIR="$(pwd)"

  info "Entering ${ndtmpdir}"
  cd "${ndtmpdir}" || exit 1

  if update_available; then
    sysarch="$(uname -m)"
    download "${NETDATA_TARBALL_CHECKSUM_URL}" "${ndtmpdir}/sha256sum.txt"
    download "${NETDATA_TARBALL_URL}" "${ndtmpdir}/netdata-${sysarch}-latest.gz.run"
    if ! grep "netdata-${sysarch}-latest.gz.run" "${ndtmpdir}/sha256sum.txt" | safe_sha256sum -c - > /dev/null 2>&1; then
      fatal "Static binary checksum validation failed. Stopping netdata installation and leaving binary in ${ndtmpdir}\nUsually this is a result of an older copy of the file being cached somewhere and can be resolved by simply retrying in an hour."
    fi

    if [ -e /opt/netdata/etc/netdata/.install-type ] ; then
      install_type="$(cat /opt/netdata/etc/netdata/.install-type)"
    else
      install_type="INSTALL_TYPE='legacy-static'"
    fi

    # Do not pass any options other than the accept, for now
    # shellcheck disable=SC2086
    if sh "${ndtmpdir}/netdata-${sysarch}-latest.gz.run" --accept -- ${REINSTALL_OPTIONS}; then
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
  exit 0
}

update_binpkg() {
  os_release_file=
  if [ -s "/etc/os-release" ] && [ -r "/etc/os-release" ]; then
    os_release_file="/etc/os-release"
  elif [ -s "/usr/lib/os-release" ] && [ -r "/usr/lib/os-release" ]; then
    os_release_file="/usr/lib/os-release"
  else
    fatal "Cannot find an os-release file ..." F0401
  fi

  # shellcheck disable=SC1090
  . "${os_release_file}"

  DISTRO="${ID}"

  supported_compat_names="debian ubuntu centos fedora opensuse"

  if str_in_list "${DISTRO}" "${supported_compat_names}"; then
    DISTRO_COMPAT_NAME="${DISTRO}"
  else
    case "${DISTRO}" in
      opensuse-leap)
        DISTRO_COMPAT_NAME="opensuse"
        ;;
      rhel)
        DISTRO_COMPAT_NAME="centos"
        ;;
      *)
        DISTRO_COMPAT_NAME="unknown"
        ;;
    esac
  fi

  if [ "${INTERACTIVE}" = "0" ]; then
    interactive_opts="-y"
    env="DEBIAN_FRONTEND=noninteractive"
  else
    interactive_opts=""
    env=""
  fi

  case "${DISTRO_COMPAT_NAME}" in
    debian)
      pm_cmd="apt-get"
      repo_subcmd="update"
      upgrade_cmd="upgrade"
      pkg_install_opts="${interactive_opts}"
      repo_update_opts="${interactive_opts}"
      pkg_installed_check="dpkg -l"
      INSTALL_TYPE="binpkg-deb"
      ;;
    ubuntu)
      pm_cmd="apt-get"
      repo_subcmd="update"
      upgrade_cmd="upgrade"
      pkg_install_opts="${interactive_opts}"
      repo_update_opts="${interactive_opts}"
      pkg_installed_check="dpkg -l"
      INSTALL_TYPE="binpkg-deb"
      ;;
    centos)
      if command -v dnf > /dev/null; then
        pm_cmd="dnf"
        repo_subcmd="makecache"
      else
        pm_cmd="yum"
      fi
      upgrade_cmd="upgrade"
      pkg_install_opts="${interactive_opts}"
      repo_update_opts="${interactive_opts}"
      pkg_installed_check="rpm -q"
      INSTALL_TYPE="binpkg-rpm"
      ;;
    fedora)
      if command -v dnf > /dev/null; then
        pm_cmd="dnf"
        repo_subcmd="makecache"
      else
        pm_cmd="yum"
      fi
      upgrade_cmd="upgrade"
      pkg_install_opts="${interactive_opts}"
      repo_update_opts="${interactive_opts}"
      pkg_installed_check="rpm -q"
      INSTALL_TYPE="binpkg-rpm"
      ;;
    opensuse)
      pm_cmd="zypper"
      repo_subcmd="--gpg-auto-import-keys refresh"
      upgrade_cmd="upgrade"
      pkg_install_opts="${interactive_opts} --allow-unsigned-rpm"
      repo_update_opts=""
      pkg_installed_check="rpm -q"
      INSTALL_TYPE="binpkg-rpm"
      ;;
    *)
      warning "We do not provide native packages for ${DISTRO}."
      return 2
      ;;
  esac

  if [ -n "${repo_subcmd}" ]; then
    # shellcheck disable=SC2086
    env ${env} ${pm_cmd} ${repo_subcmd} ${repo_update_opts} || fatal "Failed to update repository metadata."
  fi

  for repopkg in netdata-repo netdata-repo-edge; do
    if ${pkg_installed_check} ${repopkg} > /dev/null 2>&1; then
      # shellcheck disable=SC2086
      env ${env} ${pm_cmd} ${upgrade_cmd} ${pkg_install_opts} ${repopkg} || fatal "Failed to update Netdata repository config."
      # shellcheck disable=SC2086
      env ${env} ${pm_cmd} ${repo_subcmd} ${repo_update_opts} || fatal "Failed to update repository metadata."
    fi
  done

  # shellcheck disable=SC2086
  env ${env} ${pm_cmd} ${upgrade_cmd} ${pkg_install_opts} netdata || fatal "Failed to update Netdata package."
}

# Simple function to encapsulate original updater behavior.
update_legacy() {
  set_tarball_urls "${RELEASE_CHANNEL}" "${IS_NETDATA_STATIC_BINARY}"
  if [ "${IS_NETDATA_STATIC_BINARY}" = "yes" ]; then
    update_static && exit 0
  else
    update_build && exit 0
  fi
}

logfile=
ndtmpdir=

trap cleanup EXIT

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
      fatal "Cannot find environment file or install type file, unable to update."
    fi
  fi
fi

if [ -r "${ENVIRONMENT_FILE}" ] ; then
  # shellcheck source=/dev/null
  . "${ENVIRONMENT_FILE}" || exit 1
fi

if [ -r "$(dirname "${ENVIRONMENT_FILE}")/.install-type" ]; then
  # shellcheck source=/dev/null
  . "$(dirname "${ENVIRONMENT_FILE}")/.install-type" || exit 1
fi

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
  elif [ "${1}" = "--enable-auto-updates" ]; then
    enable_netdata_updater "${2}"
    exit $?
  elif [ "${1}" = "--disable-auto-updates" ]; then
    disable_netdata_updater
    exit $?
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

# We dont expect to find lib dir variable on older installations, so load this path if none found
export NETDATA_LIB_DIR="${NETDATA_LIB_DIR:-${NETDATA_PREFIX}/var/lib/netdata}"

# Source the tarball checksum, if not already available from environment (for existing installations with the old logic)
[ -z "${NETDATA_TARBALL_CHECKSUM}" ] && [ -f "${NETDATA_LIB_DIR}/netdata.tarball.checksum" ] && NETDATA_TARBALL_CHECKSUM="$(cat "${NETDATA_LIB_DIR}/netdata.tarball.checksum")"

# Grab the nightlies baseurl (defaulting to our Google Storage bucket)
export NETDATA_NIGHTLIES_BASEURL="${NETDATA_NIGHTLIES_BASEURL:-https://storage.googleapis.com/netdata-nightlies}"

if [ "${INSTALL_UID}" != "$(id -u)" ]; then
  fatal "You are running this script as user with uid $(id -u). We recommend to run this script as root (user with uid 0)"
fi

self_update

# shellcheck disable=SC2153
case "${INSTALL_TYPE}" in
    *-build)
      validate_environment_file || exit 1
      set_tarball_urls "${RELEASE_CHANNEL}" "${IS_NETDATA_STATIC_BINARY}"
      update_build && exit 0
      ;;
    *-static*)
      validate_environment_file || exit 1
      set_tarball_urls "${RELEASE_CHANNEL}" "${IS_NETDATA_STATIC_BINARY}"
      update_static && exit 0
      ;;
    *binpkg*)
      update_binpkg && exit 0
      ;;
    "") # Fallback case for no `.install-type` file. This just works like the old install type detection.
      validate_environment_file || exit 1
      update_legacy
      ;;
    custom)
      # At this point, we _should_ have a valid `.environment` file, but it's best to just check.
      # If we do, then behave like the legacy updater.
      if validate_environment_file; then
        update_legacy
      else
        fatal "This script does not support updating custom installations without valid environment files."
      fi
      ;;
    oci)
      fatal "This script does not support updating Netdata inside our official Docker containers, please instead update the container itself."
      ;;
    *)
      fatal "Unrecognized installation type (${INSTALL_TYPE}), unable to update."
      ;;
esac
