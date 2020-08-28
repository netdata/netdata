#!/usr/bin/env sh
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Run me with:
#
# bash <(curl -Ss https://my-netdata.io/kickstart.sh)
#
# or (to install all netdata dependencies):
#
# bash <(curl -Ss https://my-netdata.io/kickstart.sh) all
#
# Other options:
#  --dont-wait                do not prompt for user input
#  --non-interactive          do not prompt for user input
#  --no-updates               do not install script for daily updates
#  --local-files              set the full path of the desired tarball to run install with
#  --allow-duplicate-install  do not bail if we detect a duplicate install
#  --reinstall                if an existing install would be updated, reinstall instead
#
# Environment options:
#
#  TMPDIR                     specify where to save temporary files
#  NETDATA_TARBALL_BASEURL    set the base url for downloading the dist tarball
#
# This script will:
#
# 1. install all netdata compilation dependencies
#    using the package manager of the system
#
# 2. download netdata nightly package to temporary directory
#
# 3. install netdata

# shellcheck disable=SC2039,SC2059,SC2086

# External files
PACKAGES_SCRIPT="https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/install-required-packages.sh"

# Netdata Tarball Base URL (defaults to our Google Storage Bucket)
[ -z "$NETDATA_TARBALL_BASEURL" ] && NETDATA_TARBALL_BASEURL=https://storage.googleapis.com/netdata-nightlies

# ---------------------------------------------------------------------------------------------------------------------
# library functions copied from packaging/installer/functions.sh

setup_terminal() {
  TPUT_RESET=""
  TPUT_YELLOW=""
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
      TPUT_YELLOW="$(tput setaf 3)"
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
  printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ABORTED ${TPUT_RESET} ${*} \n\n"
  exit 1
}

run_ok() {
  printf >&2 "${TPUT_BGGREEN}${TPUT_WHITE}${TPUT_BOLD} OK ${TPUT_RESET} \n\n"
}

run_failed() {
  printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} FAILED ${TPUT_RESET} \n\n"
}

ESCAPED_PRINT_METHOD=
if printf "%q " test > /dev/null 2>&1; then
  ESCAPED_PRINT_METHOD="printfq"
fi
escaped_print() {
  if [ "${ESCAPED_PRINT_METHOD}" = "printfq" ]; then
    printf "%q " "${@}"
  else
    printf "%s" "${*}"
  fi
  return 0
}

progress() {
  echo >&2 " --- ${TPUT_DIM}${TPUT_BOLD}${*}${TPUT_RESET} --- "
}

run_logfile="/dev/null"
run() {
  local user="${USER--}" dir="${PWD}" info info_console

  if [ "${UID}" = "0" ]; then
    info="[root ${dir}]# "
    info_console="[${TPUT_DIM}${dir}${TPUT_RESET}]# "
  else
    info="[${user} ${dir}]$ "
    info_console="[${TPUT_DIM}${dir}${TPUT_RESET}]$ "
  fi

  {
    printf "${info}"
    escaped_print "${@}"
    printf " ... "
  } >> "${run_logfile}"

  printf >&2 "${info_console}${TPUT_BOLD}${TPUT_YELLOW}"
  escaped_print >&2 "${@}"
  printf >&2 "${TPUT_RESET}"

  "${@}"

  local ret=$?
  if [ ${ret} -ne 0 ]; then
    run_failed
    printf >> "${run_logfile}" "FAILED with exit code ${ret}\n"
  else
    run_ok
    printf >> "${run_logfile}" "OK\n"
  fi

  return ${ret}
}

fatal() {
  printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ABORTED ${TPUT_RESET} ${*} \n\n"
  exit 1
}

warning() {
  printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} WARNING ${TPUT_RESET} ${*} \n\n"
  if [ "${INTERACTIVE}" = "0" ]; then
    fatal "Stopping due to non-interactive mode. Fix the issue or retry installation in an interactive mode."
  else
    read -r -p "Press ENTER to attempt netdata installation > "
    progress "OK, let's give it a try..."
  fi
}

_cannot_use_tmpdir() {
  local testfile ret
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
        echo >&2 "Unable to find a usable temprorary directory. Please set \$TMPDIR to a path that is both writable and allows execution of files and try again."
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
    run curl -q -sSL --connect-timeout 10 --retry 3 --output "${dest}" "${url}"
  elif command -v wget > /dev/null 2>&1; then
    run wget -T 15 -O "${dest}" "${url}" || fatal "Cannot download ${url}"
  else
    fatal "I need curl or wget to proceed, but neither is available on this system."
  fi
}

set_tarball_urls() {
  if [ -n "${NETDATA_LOCAL_TARBALL_OVERRIDE}" ]; then
    progress "Not fetching remote tarballs, local override was given"
    return
  fi

  if [ "$1" = "stable" ]; then
    local latest
    # Simple version
    latest="$(download "https://api.github.com/repos/netdata/netdata/releases/latest" /dev/stdout | grep tag_name | cut -d'"' -f4)"
    export NETDATA_TARBALL_URL="https://github.com/netdata/netdata/releases/download/$latest/netdata-$latest.tar.gz"
    export NETDATA_TARBALL_CHECKSUM_URL="https://github.com/netdata/netdata/releases/download/$latest/sha256sums.txt"
  else
    export NETDATA_TARBALL_URL="$NETDATA_TARBALL_BASEURL/netdata-latest.tar.gz"
    export NETDATA_TARBALL_CHECKSUM_URL="$NETDATA_TARBALL_BASEURL/sha256sums.txt"
  fi
}

detect_bash4() {
  bash="${1}"
  if [ -z "${BASH_VERSION}" ]; then
    # we don't run under bash
    if [ -n "${bash}" ] && [ -x "${bash}" ]; then
      # shellcheck disable=SC2016
      BASH_MAJOR_VERSION=$(${bash} -c 'echo "${BASH_VERSINFO[0]}"')
    fi
  else
    # we run under bash
    BASH_MAJOR_VERSION="${BASH_VERSINFO[0]}"
  fi

  if [ -z "${BASH_MAJOR_VERSION}" ]; then
    echo >&2 "No BASH is available on this system"
    return 1
  elif [ $((BASH_MAJOR_VERSION)) -lt 4 ]; then
    echo >&2 "No BASH v4+ is available on this system (installed bash is v${BASH_MAJOR_VERSION}"
    return 1
  fi
  return 0
}

dependencies() {
  SYSTEM="$(uname -s 2> /dev/null || uname -v)"
  OS="$(uname -o 2> /dev/null || uname -rs)"
  MACHINE="$(uname -m 2> /dev/null)"

  echo "System            : ${SYSTEM}"
  echo "Operating System  : ${OS}"
  echo "Machine           : ${MACHINE}"
  echo "BASH major version: ${BASH_MAJOR_VERSION}"

  if [ "${OS}" != "GNU/Linux" ] && [ "${SYSTEM}" != "Linux" ]; then
    warning "Cannot detect the packages to be installed on a ${SYSTEM} - ${OS} system."
  else
    bash="$(command -v bash 2> /dev/null)"
    if ! detect_bash4 "${bash}"; then
      warning "Cannot detect packages to be installed in this system, without BASH v4+."
    else
      progress "Fetching script to detect required packages..."
      if [ -n "${NETDATA_LOCAL_TARBALL_OVERRIDE_DEPS_SCRIPT}" ]; then
        if [ -f "${NETDATA_LOCAL_TARBALL_OVERRIDE_DEPS_SCRIPT}" ]; then
          run cp "${NETDATA_LOCAL_TARBALL_OVERRIDE_DEPS_SCRIPT}" "${ndtmpdir}/install-required-packages.sh"
        else
          fatal "Invalid given dependency file, please check your --local-files parameter options and try again"
        fi
      else
        download "${PACKAGES_SCRIPT}" "${ndtmpdir}/install-required-packages.sh"
      fi

      if [ ! -s "${ndtmpdir}/install-required-packages.sh" ]; then
        warning "Downloaded dependency installation script is empty."
      else
        progress "Running downloaded script to detect required packages..."
        run ${sudo} "${bash}" "${ndtmpdir}/install-required-packages.sh" ${PACKAGES_INSTALLER_OPTIONS}
        # shellcheck disable=SC2181
        if [ $? -ne 0 ]; then
          warning "It failed to install all the required packages, but installation might still be possible."
        fi
      fi

    fi
  fi
}

safe_sha256sum() {
  # Within the contexct of the installer, we only use -c option that is common between the two commands
  # We will have to reconsider if we start non-common options
  if command -v sha256sum > /dev/null 2>&1; then
    sha256sum "$@"
  elif command -v shasum > /dev/null 2>&1; then
    shasum -a 256 "$@"
  else
    fatal "I could not find a suitable checksum binary to use"
  fi
}

# ---------------------------------------------------------------------------------------------------------------------
umask 022

sudo=""
[ -z "${UID}" ] && UID="$(id -u)"
[ "${UID}" -ne "0" ] && sudo="sudo"
export PATH="${PATH}:/usr/local/bin:/usr/local/sbin"


# ---------------------------------------------------------------------------------------------------------------------

INTERACTIVE=1
PACKAGES_INSTALLER_OPTIONS="netdata"
NETDATA_INSTALLER_OPTIONS=""
NETDATA_UPDATES="--auto-update"
RELEASE_CHANNEL="nightly"
while [ -n "${1}" ]; do
  if [ "${1}" = "all" ]; then
    PACKAGES_INSTALLER_OPTIONS="netdata-all"
    shift 1
  elif [ "${1}" = "--dont-wait" ] || [ "${1}" = "--non-interactive" ]; then
    INTERACTIVE=0
    shift 1
  elif [ "${1}" = "--no-updates" ]; then
    # echo >&2 "netdata will not auto-update"
    NETDATA_UPDATES=
    shift 1
  elif [ "${1}" = "--stable-channel" ]; then
    RELEASE_CHANNEL="stable"
    NETDATA_INSTALLER_OPTIONS="$NETDATA_INSTALLER_OPTIONS --stable-channel"
    shift 1
  elif [ "${1}" = "--allow-duplicate-install" ]; then
    NETDATA_ALLOW_DUPLICATE_INSTALL=1
    shift 1
  elif [ "${1}" = "--reinstall" ]; then
    NETDATA_REINSTALL=1
    shift 1
  elif [ "${1}" = "--local-files" ]; then
    shift 1
    if [ -z "${1}" ]; then
      fatal "Missing netdata: Option --local-files requires extra information. The desired tarball for netdata, the checksum, the go.d plugin tarball , the go.d plugin config tarball and the dependency management script, in this particular order"
    fi

    export NETDATA_LOCAL_TARBALL_OVERRIDE="${1}"
    shift 1

    if [ -z "${1}" ]; then
      fatal "Missing checksum file: Option --local-files requires extra information. The desired tarball for netdata, the checksum, the go.d plugin tarball , the go.d plugin config tarball and the dependency management script, in this particular order"
    fi

    export NETDATA_LOCAL_TARBALL_OVERRIDE_CHECKSUM="${1}"
    shift 1

    if [ -z "${1}" ]; then
      fatal "Missing go.d tarball: Option --local-files requires extra information. The desired tarball for netdata, the checksum, the go.d plugin tarball , the go.d plugin config tarball and the dependency management script, in this particular order"
    fi

    export NETDATA_LOCAL_TARBALL_OVERRIDE_GO_PLUGIN="${1}"
    shift 1

    if [ -z "${1}" ]; then
      fatal "Missing go.d config tarball: Option --local-files requires extra information. The desired tarball for netdata, the checksum, the go.d plugin tarball , the go.d plugin config tarball and the dependency management script, in this particular order"
    fi

    export NETDATA_LOCAL_TARBALL_OVERRIDE_GO_PLUGIN_CONFIG="${1}"
    shift 1

    if [ -z "${1}" ]; then
      fatal "Missing dependencies management scriptlet: Option --local-files requires extra information. The desired tarball for netdata, the checksum, the go.d plugin tarball , the go.d plugin config tarball and the dependency management script, in this particular order"
    fi

    export NETDATA_LOCAL_TARBALL_OVERRIDE_DEPS_SCRIPT="${1}"
    shift 1
  else
    break
  fi
done

if [ "${INTERACTIVE}" = "0" ]; then
  PACKAGES_INSTALLER_OPTIONS="--dont-wait --non-interactive ${PACKAGES_INSTALLER_OPTIONS}"
  NETDATA_INSTALLER_OPTIONS="$NETDATA_INSTALLER_OPTIONS --dont-wait"
fi

# ---------------------------------------------------------------------------------------------------------------------
# look for an existing install and try to update that instead if it exists

ndpath="$(command -v netdata 2>/dev/null)"
if [ -z "$ndpath" ] && [ -x /opt/netdata/bin/netdata ] ; then
    ndpath="/opt/netdata/bin/netdata"
fi

if [ -n "$ndpath" ] ; then
  ndprefix="$(dirname "$(dirname "${ndpath}")")"

  if [ "${ndprefix}" = /usr ] ; then
    ndprefix="/"
  fi

  progress "Found existing install of Netdata under: ${ndprefix}"

  if [ -r "${ndprefix}/etc/netdata/.environment" ] ; then
    ndstatic="$(grep IS_NETDATA_STATIC_BINARY "${ndprefix}/etc/netdata/.environment" | cut -d "=" -f 2 | tr -d \")"
    if [ -z "${NETDATA_REINSTALL}" ] && [ -z "${NETDATA_LOCAL_TARBALL_OVERRIDE}" ] ; then
      if [ -x "${ndprefix}/usr/libexec/netdata/netdata-updater.sh" ] ; then
        progress "Attempting to update existing install instead of creating a new one"
        if run ${sudo} "${ndprefix}/usr/libexec/netdata/netdata-updater.sh" --not-running-from-cron ; then
          progress "Updated existing install at ${ndpath}"
          exit 0
        else
          fatal "Failed to update existing Netdata install"
          exit 1
        fi
      else
        if [ -z "${NETDATA_ALLOW_DUPLICATE_INSTALL}" ] || [ "${ndstatic}" = "yes" ] ; then
          fatal "Existing installation detected which cannot be safely updated by this script, refusing to continue."
          exit 1
        else
          progress "User explicitly requested duplicate install, proceeding."
        fi
      fi
    else
      if [ "${ndstatic}" = "no" ] ; then
        progress "User requested reinstall instead of update, proceeding."
      else
        fatal "Existing install is a static install, please use kickstart-static64.sh instead."
        exit 1
      fi
    fi
  else
    progress "Existing install appears to be handled manually or through the system package manager."
    if [ -z "${NETDATA_ALLOW_DUPLICATE_INSTALL}" ] ; then
      fatal "Existing installation detected which cannot be safely updated by this script, refusing to continue."
      exit 1
    else
      progress "User explicitly requested duplicate install, proceeding."
    fi
  fi
fi

# ---------------------------------------------------------------------------------------------------------------------
# install required system packages

ndtmpdir=$(create_tmp_directory)
cd "${ndtmpdir}" || exit 1

dependencies

# ---------------------------------------------------------------------------------------------------------------------
# download netdata package

if [ -z "${NETDATA_LOCAL_TARBALL_OVERRIDE}" ]; then
  set_tarball_urls "${RELEASE_CHANNEL}"

  download "${NETDATA_TARBALL_CHECKSUM_URL}" "${ndtmpdir}/sha256sum.txt"
  download "${NETDATA_TARBALL_URL}" "${ndtmpdir}/netdata-latest.tar.gz"
else
  progress "Installation sources were given as input, running installation using \"${NETDATA_LOCAL_TARBALL_OVERRIDE}\""
  run cp "${NETDATA_LOCAL_TARBALL_OVERRIDE_CHECKSUM}" "${ndtmpdir}/sha256sum.txt"
  run cp "${NETDATA_LOCAL_TARBALL_OVERRIDE}" "${ndtmpdir}/netdata-latest.tar.gz"
fi

if ! grep netdata-latest.tar.gz "${ndtmpdir}/sha256sum.txt" | safe_sha256sum -c - > /dev/null 2>&1; then
  fatal "Tarball checksum validation failed. Stopping netdata installation and leaving tarball in ${ndtmpdir}"
fi
run tar -xf netdata-latest.tar.gz
rm -rf netdata-latest.tar.gz > /dev/null 2>&1
cd netdata-* || fatal "Cannot cd to netdata source tree"

# ---------------------------------------------------------------------------------------------------------------------
# install netdata from source

if [ -x netdata-installer.sh ]; then
  progress "Installing netdata..."
  run ${sudo} ./netdata-installer.sh ${NETDATA_UPDATES} ${NETDATA_INSTALLER_OPTIONS} "${@}" || fatal "netdata-installer.sh exited with error"
  if [ -d "${ndtmpdir}" ] && [ ! "${ndtmpdir}" = "/" ]; then
    run ${sudo} rm -rf "${ndtmpdir}" > /dev/null 2>&1
  fi
else
  fatal "Cannot install netdata from source (the source directory does not include netdata-installer.sh). Leaving all files in ${ndtmpdir}"
fi
