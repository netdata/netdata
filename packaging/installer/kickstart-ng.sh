#!/bin/sh
#
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Other options:
#  --dont-wait                do not prompt for user input
#  --non-interactive          do not prompt for user input
#  --interactive              prompt for user input even if there is no controlling terminal
#  --stable-channel           install a stable version instead of a nightly build
#  --reinstall                explicitly reinstall instead of updating any existing install
#  --claim-token              use a specified token for claiming to Netdata Cloud
#  --claim-rooms              when claiming, add the node to the specified rooms
#  --claim-only               when running against an existing install, only try to claim it, not update it
#  --claim-*                  specify other options for the claiming script
#  --no-cleanup               don't do any cleanup steps, intended for debugging the installer
#
# Environment options:
#
#  TMPDIR                     specify where to save temporary files
#  ROOTCMD                    specify a command to use to run programs with root privileges

# ======================================================================
# Constants

REPOCONFIG_URL_PREFIX="https://packagecloud.io/netdata/netdata-repoconfig/packages"
REPOCONFIG_VERSION="1-1"
PATH="${PATH}:/usr/local/bin:/usr/local/sbin"

# ======================================================================
# Defaults for environment variables

RELEASE_CHANNEL="nightly"
NETDATA_CLAIM_URL="https://app.netdata.cloud"

if [ ! -t 1 ]; then
  INTERACTIVE=0
else
  INTERACTIVE=1
fi

# ======================================================================
# Usage info

usage() {
  cat << HEREDOC
USAGE: kickstart.sh [options]
       where options include:

  --dont-wait                Do not prompt for user input. (default: prompt if there is a controlling terminal)
  --non-interactive          Do not prompt for user input.
  --interactive              Prompt for user input even if there is no controlling terminal.
  --stable-channel           Install a stable version instead of a nightly build (default: install a nightly build)
  --nightly-channel          Install a nightly build instead of a stable version
  --reinstall                Explicitly reinstall instead of updating any existing install.
  --claim-token              Use a specified token for claiming to Netdata Cloud.
  --claim-rooms              When claiming, add the node to the specified rooms.
  --claim-only               If there is an existing install, only try to claim it, not update it.
  --claim-*                  Specify other options for the claiming script.
  --no-cleanup               Don't do any cleanup steps. This is intended to help with debuggint the installer.

Additionally, this script may use the following environment variables:

  TMPDIR:                    Used to specify where to put temporary files. On most systems, the default we select
                             automatically should be fine. The user running the script needs to both be able to
                             write files to the temporary directory, and run files from that location.
  ROOTCMD:                   Used to specify a command to use to run another command with root privileges if needed. By
                             default we try to use sudo, doas, or pkexec (in that order of preference), but if
                             you need special options for one of those to work, or have a different tool to do
                             the same thing on your system, you can specify it here.

HEREDOC
}

# ======================================================================
# Utility functions

setup_terminal() {
  TPUT_RESET=""
  TPUT_WHITE=""
  TPUT_BGRED=""
  TPUT_BGGREEN=""
  TPUT_BOLD=""
  TPUT_DIM=""

  # Is stderr on the terminal? If not, then fail
  test -t 2 || return 1

  if command -v tput > /dev/null 2>&1; then
    if [ $(($(tput colors 2> /dev/null))) -ge 8 ]; then
      # Enable colors
      TPUT_RESET="$(tput sgr 0)"
      TPUT_WHITE="$(tput setaf 7)"
      TPUT_BGRED="$(tput setab 1)"
      TPUT_BGGREEN="$(tput setab 2)"
      TPUT_BOLD="$(tput bold)"
      TPUT_DIM="$(tput dim)"
    fi
  fi

  echo "${TPUT_RESET}"

  return 0
}

fatal() {
  printf >&2 "%s\n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ABORTED ${TPUT_RESET} ${*}"
  exit 1
}

run_ok() {
  printf >&2 "%s\n\n" "${TPUT_BGGREEN}${TPUT_WHITE}${TPUT_BOLD} OK ${TPUT_RESET}"
}

run_failed() {
  printf >&2 "%s\n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} FAILED ${TPUT_RESET}"
}

ESCAPED_PRINT_METHOD=
# shellcheck disable=SC3050
if printf "%q " test > /dev/null 2>&1; then
  ESCAPED_PRINT_METHOD="printfq"
fi

escaped_print() {
  if [ "${ESCAPED_PRINT_METHOD}" = "printfq" ]; then
    # shellcheck disable=SC3050
    printf "%q " "${@}"
  else
    printf "%s" "${*}"
  fi
  return 0
}

progress() {
  echo >&2 " --- ${TPUT_BOLD}${*}${TPUT_RESET} --- "
}

run_logfile="/dev/null"
run() {
  user="${USER--}"
  dir="${PWD}"

  if [ "$(id -u)" = "0" ]; then
    info="[root ${dir}]# "
    info_console="[${TPUT_DIM}${dir}${TPUT_RESET}]# "
  else
    info="[${user} ${dir}]$ "
    info_console="[${TPUT_DIM}${dir}${TPUT_RESET}]$ "
  fi

  {
    printf "%s" "${info}"
    escaped_print "${@}"
    printf " ... "
  } >> "${run_logfile}"

  printf >&2 "%s" "${info_console}${TPUT_BOLD}"
  escaped_print >&2 "${@}"
  printf >&2 "%s\n" "${TPUT_RESET}"

  "${@}"

  ret=$?
  if [ ${ret} -ne 0 ]; then
    run_failed
    printf "%s\n" "FAILED with exit code ${ret}" >> "${run_logfile}"
  else
    run_ok
    printf "OK\n" >> "${run_logfile}"
  fi

  return ${ret}
}

fatal() {
  printf >&2 "%s\n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ABORTED ${TPUT_RESET} ${*}"
  exit 1
}

warning() {
  printf >&2 "%s\n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} WARNING ${TPUT_RESET} ${*}"
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

  mktemp -d -t netdata-kickstart-XXXXXXXXXX
}

download() {
  url="${1}"
  dest="${2}"
  if command -v curl > /dev/null 2>&1; then
    run curl -q -sSL --connect-timeout 10 --retry 3 --output "${dest}" "${url}" || return 1
  elif command -v wget > /dev/null 2>&1; then
    run wget -T 15 -O "${dest}" "${url}" || return 1
  elif command -v fetch > /dev/null 2>&1; then # Native FreeBSD tool
    run fetch -T 15 -a -o "${dest}" "${url}" || return 1
  else
    fatal "I need curl, wget, or fetch to proceed, but none of them are available on this system."
  fi
}

get_system_info() {
  case "$(uname -s)" in
    Linux)
      SYSTYPE="Linux"

      os_release_file=
      if [ -s "/etc/os-release" ] && [ -r "/etc/os-release" ]; then
        os_release_file="/etc/os-release"
      elif [ -s "/usr/lib/os-release" ] && [ -r "/usr/lib/os-release" ]; then
        os_release_file="/usr/lib/os-release"
      else
        echo >&2 "Cannot find an os-release file ..."
        return 1
      fi

      # shellcheck disable=SC1090
      . "${os_release_file}"

      DISTRO="${ID}"
      SYSVERSION="${VERSION_ID}"
      SYSCODENAME="${VERSION_CODENAME}"

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
      ;;
    Darwin)
      SYSTYPE="Darwin"
      SYSVERSION="$(sw_vers -buildVersion)"
      ;;
    FreebSD)
      SYSTYPE="FreeBSD"
      SYSVERSION="$(uname -K)"
      ;;
    *)
      fatal "Unsupported system type detected. Netdata cannot be installed on this system using this script."
      ;;
  esac
}

str_in_list() {
  printf "%s\n" "${2}" | tr ' ' "\n" | grep -qE "^${1}\$"
  return $?
}

confirm_root_support() {
  if [ "$(id -u)" -ne "0" ]; then
    if [ -z "${ROOTCMD}" ] && command -v sudo > /dev/null; then
      ROOTCMD="sudo"
    fi

    if [ -z "${ROOTCMD}" ] && command -v doas > /dev/null; then
      ROOTCMD="doas"
    fi

    if [ -z "${ROOTCMD}" ] && command -v pkexec > /dev/null; then
      ROOTCMD="pkexec"
    fi

    if [ -z "${ROOTCMD}" ]; then
      fatal "We need root privileges to continue, but cannot find a way to gain them. Either re-run this script as root, or set \$ROOTCMD to a command that can be used to gain root privileges"
    fi
  fi
}

# ======================================================================
# Existing install handling code

update() {
  updater="${ndprefix}/usr/libexec/netdata/netdata-updater.sh"

  if [ -x "${updater}" ]; then
    if run ${ROOTCMD} "${updater}" --not-running-from-cron; then
      progress "Updated existing install at ${ndprefix}"
      return 0
    else
      fatal "Failed to update existing Netdata install at ${ndprefix}"
    fi
  else
    return 1
  fi
}

check_for_existing_install() {
  if pkg_installed netdata; then
    ndprefix="/"
  else
    ndpath="$(command -v netdata 2>/dev/null)"
    if [ -z "$ndpath" ] && [ -x /opt/netdata/bin/netdata ] ; then
      ndpath="/opt/netdata/bin/netdata"
    fi

    if [ -n "${ndprefix}" ]; then
      ndprefix="$(dirname "$(dirname "${ndpath}")")"
    fi

    if [ "${ndprefix}" = /usr ] ; then
      ndprefix="/"
    fi
  fi

  if [ -n "${ndprefix}" ]; then
    typefile="${ndprefix}/etc/netdata/.install-type"
    if [ -r "${typefile}" ]; then
      # shellcheck disable=SC1090
      . "${typefile}"
    else
      INSTALL_TYPE="unknown"
    fi
  else
    return 1
  fi
}

handle_existing_install() {
  if ! check_for_existing_install; then
    progress "No existing install found, assuming this is a new installation."
    return 0
  fi

  if [ -n "${ndprefix}" ]; then
    case "${INSTALL_TYPE}" in
      kickstart-*|legacy-*|manual-static)
        if [ -z "${NETDATA_REINSTALL}" ]; then
          if [ -n "${NETDATA_NO_UPDATE}" ]; then
            if ! update; then
              warning "Unable to find usable updater script, not updating existing install at ${ndprefix}."
            fi
          fi

          if [ -n "${NETDATA_CLAIM_TOKEN}" ]; then
            progress "Attempting to claim existing install at ${ndprefix}."
            claim
          fi
        else
          progress "Found an existing netdata install at ${ndprefix}, but user requested reinstall, continuing."
        fi
        ;;
      binpkg-*)
        if [ -n "${NETDATA_CLAIM_TOKEN}" ]; then
          progress "Attempting to claim existing install at ${ndprefix}."
          claim
        fi
        ;;
      oci)
        fatal "This is an OCI container, use the regular image lifecycle management commands in your container instead of this script for managing it."
        ;;
      unknown)
        fatal "Found an existing netdata install at ${ndprefix}, but could not determine the install type, refusing to proceed."
        ;;
      *)
        fatal "Found an existing netdata install at ${ndprefix}, but it is not a supported install type, refusing to proceed."
        ;;
    esac
  else
    progress "No existing installations of netdata found, assuming this is a fresh install."
  fi
}

# ======================================================================
# Claiming support code

check_claim_opts() {
# shellcheck disable=SC2235,SC2030
  if [ -z "${NETDATA_CLAIM_TOKEN}" ] && [ -n "${NETDATA_CLAIM_ROOMS}" ]; then
    fatal "Invalid claiming options, claim rooms may only be specified when a token and URL are specified."
  fi
}

claim() {
  progress "Attempting to claim agent to ${NETDATA_CLAIM_URL}"
  if [ -z "${NETDATA_PREFIX}" ] ; then
    NETDATA_CLAIM_PATH=/usr/sbin/netdata-claim.sh
  else
    NETDATA_CLAIM_PATH="${NETDATA_PREFIX}/netdata/usr/sbin/netdata-claim.sh"
  fi

  # shellcheck disable=SC2086
  if "${NETDATA_CLAIM_PATH}" -token="${NETDATA_CLAIM_TOKEN}" -rooms="${NETDATA_CLAIM_ROOMS}" -url="${NETDATA_CLAIM_URL}" ${NETDATA_CLAIM_EXTRA}; then
    progress "Successfully claimed node"
  else
    warning "Unable to claim node, you must do so manually."
    if [ -z "${NETDATA_NEW_INSTALL}" ]; then
      exit 1
    fi
  fi
}

# ======================================================================
# Native package install code.

# Check for an already installed package with a given name.
pkg_installed() {
  case "${DISTRO_COMPAT_NAME}" in
    debian|ubuntu)
      dpkg -l "${1}" > /dev/null 2>&1
      return $?
      ;;
    centos|fedora|opensuse)
      rpm -q "${1}" > /dev/null 2>&1
      return $?
      ;;
  esac
}

# Check for the existence of a usable package in the repo.
pkg_avail_check() {
  case "${DISTRO_COMPAT_NAME}" in
    debian|ubuntu)
      ${ROOTCMD} apt-cache policy netdata | grep -q packagecloud.io/netdata/netdata;
      return $?
      ;;
    centos|fedora)
      # shellcheck disable=SC2086
      ${ROOTCMD} ${pm_cmd} search -v netdata | grep -qE 'Repo *: netdata(-edge)?$'
      return $?
      ;;
    opensuse)
      ${ROOTCMD} zypper packages -r "$(zypper repos | grep -E 'netdata |netdata-edge ' | cut -f 1 -d '|' | tr -d ' ')" | grep -E 'netdata '
      return $?
      ;;
    *)
      return 1
      ;;
  esac
}

try_package_install() {
  if [ -z "${DISTRO}" ]; then
    echo "Unable to determine Linux distribution for native packages."
    return 1
  fi

  progress "Attempting to install using native packages..."

  if [ "${RELEASE_CHANNEL}" = "nightly" ]; then
    release="-edge"
  else
    release=""
  fi

  if [ "${INTERACTIVE}" = "0" ]; then
    interactive_opts="-y"
  else
    interactive_opts=""
  fi

  case "${DISTRO_COMPAT_NAME}" in
    debian)
      pm_cmd="apt-get"
      repo_subcmd="update"
      repo_prefix="debian/${SYSCODENAME}"
      pkg_type="deb"
      pkg_suffix="_all"
      pkg_vsep="_"
      pkg_install_opts="${interactive_opts}"
      repo_update_opts="${interactive_opts}"
      uninstall_subcmd="uninstall"
      ;;
    ubuntu)
      pm_cmd="apt-get"
      repo_subcmd="update"
      repo_prefix="ubuntu/${SYSCODENAME}"
      pkg_type="deb"
      pkg_suffix="_all"
      pkg_vsep="_"
      pkg_install_opts="${interactive_opts}"
      repo_update_opts="${interactive_opts}"
      uninstall_subcmd="uninstall"
      ;;
    centos)
      if command -v dnf > /dev/null; then
        pm_cmd="dnf"
        repo_subcmd="makecache"
      else
        pm_cmd="yum"
        repo_subcmd="refresh"
      fi
      repo_prefix="el/${SYSVERSION}"
      pkg_type="rpm"
      pkg_suffix=".noarch"
      pkg_vsep="-"
      pkg_install_opts="${interactive_opts}"
      repo_update_opts="${interactive_opts}"
      uninstall_subcmd="remove"
      ;;
    fedora)
      if command -v dnf > /dev/null; then
        pm_cmd="dnf"
        repo_subcmd="makecache"
      else
        pm_cmd="yum"
        repo_subcmd="refresh"
      fi
      repo_prefix="fedora/${SYSVERSION}"
      pkg_type="rpm"
      pkg_suffix=".noarch"
      pkg_vsep="-"
      pkg_install_opts="${interactive_opts}"
      repo_update_opts="${interactive_opts}"
      uninstall_subcmd="remove"
      ;;
    opensuse)
      pm_cmd="zypper"
      repo_subcmd="--gpg-auto-import-keys refresh"
      repo_prefix="opensuse/${SYSVERSION}"
      pkg_type="rpm"
      pkg_suffix=".noarch"
      pkg_vsep="-"
      pkg_install_opts="${interactive_opts} --allow-unsigned-rpm"
      repo_update_opts=""
      uninstall_subcmd="remove"
      ;;
    *)
      warning "We do not provide native packages for ${DISTRO}."
      return 2
      ;;
  esac

  repoconfig_name="netdata-repo${release}"
  repoconfig_file="${repoconfig_name}${pkg_vsep}${REPOCONFIG_VERSION}${pkg_suffix}.${pkg_type}"
  repoconfig_url="${REPOCONFIG_URL_PREFIX}/${repo_prefix}/${repoconfig_file}/download.${pkg_type}"

  if ! pkg_installed "${repoconfig_name}"; then
    progress "Downloading repository configuration package."
    if ! download "${repoconfig_url}" "${TMPDIR}/${repoconfig_file}"; then
      warning "Failed to download repository configuration package."
      return 2
    fi

    progress "Installing repository configuration package."
    # shellcheck disable=SC2086
    if ! run ${ROOTCMD} ${pm_cmd} install ${pkg_install_opts} "${TMPDIR}/${repoconfig_file}"; then
      warning "Failed to install repository configuration package."
      return 2
    fi

    progress "Updating repository metadata."
    # shellcheck disable=SC2086
    if ! run ${ROOTCMD} ${pm_cmd} ${repo_subcmd} ${repo_update_opts}; then
      fatal "Failed to update repository metadata."
    fi
  else
    progress "Repository configuration is already present, attempting to install netdata."
  fi

  progress "Checking for usable Netdata package."
  if ! pkg_avail_check "${DISTRO_COMPAT_NAME}"; then
    warning "Could not find a usable native package for ${DISTRO} on ${SYSARCH}."
    if [ -z "${NO_CLEANUP}" ]; then
      progress "Attempting to uninstall repository configuration package."
      # shellcheck disable=SC2086
      run ${ROOTCMD} ${pm_cmd} ${uninstall_subcmd} ${pkg_install_opts} "${repoconfig_name}"
    fi
    return 2
  fi

  progress "Installing Netdata package."
  # shellcheck disable=SC2086
  if ! run ${ROOTCMD} ${pm_cmd} install ${pkg_install_opts} netdata; then
    warning "Failed to install Netdata package."
    if [ -z "${NO_CLEANUP}" ]; then
      progress "Attempting to uninstall repository configuration package."
      # shellcheck disable=SC2086
      run ${ROOTCMD} ${pm_cmd} ${uninstall_subcmd} ${pkg_install_opts} "${repoconfig_name}"
    fi
    return 2
  fi
}

# ======================================================================
# Main program

setup_terminal || echo > /dev/null

while [ -n "${1}" ]; do
  case "${1}" in
    "--help")
      usage
      exit 0
      ;;
    "--no-cleanup") NO_CLEANUP=1 ;;
    "--dont-wait") INTERACTIVE=0 ;;
    "--non-interactive") INTERACTIVE=0 ;;
    "--interactive") INTERACTIVE=1 ;;
    "--stable-channel") RELEASE_CHANNEL="stable" ;;
    "--reinstall") NETDATA_REINSTALL=1 ;;
    "--claim-only") NETDATA_NO_UPDATE=1 ;;
    "--claim-token")
      NETDATA_CLAIM_TOKEN="${2}"
      shift 1
      ;;
    "--claim-rooms")
      NETDATA_CLAIM_ROOMS="${2}"
      shift 1
      ;;
    "--claim-url")
      NETDATA_CLAIM_URL="${2}"
      shift 1
      ;;
    "--claim-*")
      optname="$(echo "${1}" | cut -d '-' -f 4-)"
      NETDATA_CLAIM_EXTRA="${NETDATA_CLAIM_EXTRA} -${optname} ${2}"
      shift 1
      ;;
  esac
  shift 1
done

confirm_root_support
get_system_info

tmpdir="$(create_tmp_directory)"
progress "Using ${tmpdir} as a temporary directory."
cd "${tmpdir}" || exit 1

handle_existing_install

case "${SYSTYPE}" in
  Linux)
    try_package_install

    case "$?" in
      1)
        fatal "Unable to install on this system."
        ;;
      2)
        warning "Could not install native binary packages, falling back to alternative installation method."
        ;;
    esac
    ;;
  Darwin)
    fatal "This script currently does not support installation on macOS."
    ;;
  FreeBSD)
    fatal "This script currently does not support installation on FreeBSD."
    ;;
esac

if [ -n "${NETDATA_CLAIM_TOKEN}" ]; then
  claim
fi

if [ -z "${NO_CLEANUP}" ]; then
  rm -rf "${tmpdir}"
fi
