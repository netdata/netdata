#!/bin/sh
#
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Other options:
#  --dont-wait                do not prompt for user input
#  --non-interactive          do not prompt for user input
#  --stable-channel           install a stable version instead of a nightly build
#
# Environment options:
#
#  TMPDIR                     specify where to save temporary files

REPOCONFIG_URL_PREFIX="https://packagecloud.io/netdata/netdata-repoconfig/packages"
REPOCONFIG_VERSION="1-1"
INTERACTIVE=1
RELEASE_CHANNEL="nightly"

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

  return 0
}
setup_terminal || echo > /dev/null

# -----------------------------------------------------------------------------
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

# ------------------------------------------
# Native package install code.

get_distro_compat_name() {
  supported_compat_names="debian ubuntu centos fedora opensuse"

  if str_in_list "${DISTRO}" "${supported_compat_names}"; then
    distro_compat_name="${DISTRO}"
  else
    case "${DISTRO}" in
      rhel)
        distro_compat_name="centos"
        ;;
      *)
        distro_compat_name="unknown"
    esac
  fi
}

try_package_install() {
  if [ -z "${DISTRO}" ]; then
    echo "Unable to determine Linux distribution for native packages."
    return 1
  fi

  progress "Attempting to install using native packages..."

  get_distro_compat_name

  if [ "${RELEASE_CHANNEL}" = "nightly" ]; then
    release="-edge"
  else
    release=""
  fi

  if [ "${INTERACTIVE}" = "0" ]; then
    opts="-y"
  else
    opts=""
  fi

  case "${distro_compat_name}" in
    debian)
      pm_cmd="apt-get"
      repo_subcmd="update"
      repo_prefix="debian/${SYSCODENAME}"
      pkg_type="deb"
      pkg_suffix="_all"
      pkg_vsep="_"
      ;;
    ubuntu)
      pm_cmd="apt-get"
      repo_subcmd="update"
      repo_prefix="ubuntu/${SYSCODENAME}"
      pkg_type="deb"
      pkg_suffix="_all"
      pkg_vsep="_"
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
      ;;
    opensuse)
      pm_cmd="zypper"
      repo_subcmd="refresh"
      repo_prefix="opensuse/${SYSVERSION}"
      pkg_type="rpm"
      pkg_suffix=".noarch"
      pkg_vsep="-"
      ;;
    *)
      warning "We do not provide native binary packages for ${DISTRO}."
      return 2
      ;;
  esac

  repoconfig_name="netdata-repo${release}"
  repoconfig_file="${repoconfig_name}${pkg_vsep}${REPOCONFIG_VERSION}${pkg_suffix}.${pkg_type}"
  repoconfig_url="${REPOCONFIG_URL_PREFIX}/${repo_prefix}/${repoconfig_file}/download.${pkg_type}"

  progress "Downloading repository configuration package."
  if download "${repoconfig_url}" "${repoconfig_file}"; then
    progress "Installing repository configuration package."
    # shellcheck disable=SC2086
    if run ${pm_cmd} install ${opts} "./${repoconfig_file}"; then
      progress "Updating repository metadata."
      if run ${pm_cmd} ${repo_subcmd} ${opts}; then
        progress "Installing Netdata package."
        # shellcheck disable=SC2086
        if run ${pm_cmd} install ${opts} netdata; then
          return 0
        else
          warning "Failed to install Netdata package."
          progress "Attempting to uninstall repository configuration package."
          # shellcheck disable=SC2086
          run ${pm_cmd} uninstall ${opts} "${repoconfig_name}"
          return 2
        fi
      else
        fatal "Failed to update repository metadata."
      fi
    else
      warning "Failed to install repository configuration package."
      return 2
    fi
  else
    warning "Failed to download repository configuration package."
    return 2
  fi
}

# ------------------------------------------

while [ -n "${1}" ]; do
  case "${1}" in
    "--dont-wait") INTERACTIVE=0 ;;
    "--non-interactive") INTERACTIVE=0 ;;
    "--stable-channel") RELEASE_CHANNEL="stable" ;;
  esac
  shift 1
done

get_system_info

tmpdir="$(create_tmp_directory)"
progress "Using ${tmpdir} as a temporary directory."
cd "${tmpdir}" || exit 1

case "${SYSTYPE}" in
  Linux)
    try_package_install

    case "$?" in
      0)
        rm -rf "${tmpdir}"
        exit 0
        ;;
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
