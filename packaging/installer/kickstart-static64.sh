#!/usr/bin/env sh

# SPDX-License-Identifier: GPL-3.0-or-later
# shellcheck disable=SC1117,SC2039,SC2059,SC2086
#
#  Options to run
#  --dont-wait                do not wait for input
#  --non-interactive          do not wait for input
#  --dont-start-it            do not start netdata after install
#  --stable-channel           Use the stable release channel, rather than the nightly to fetch sources
#  --disable-telemetry        Opt-out of anonymous telemetry program (DO_NOT_TRACK=1)
#  --local-files              Use a manually provided tarball for the installation
#  --allow-duplicate-install  do not bail if we detect a duplicate install
#  --reinstall                if an existing install would be updated, reinstall instead
#  --claim-token              specify a token to use for claiming the newly installed instance
#  --claim-url                specify a URL to use for claiming the newly installed isntance
#  --claim-rooms              specify a list of rooms to claim the newly installed instance to
#  --claim-proxy              specify a proxy to use while claiming the newly installed instance
#
# Environment options:
#
#  NETDATA_TARBALL_BASEURL  set the base url for downloading the dist tarball
#
# ----------------------------------------------------------------------------
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

# ----------------------------------------------------------------------------
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
  printf >&2 "${TPUT_RESET}\n"

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
    export NETDATA_TARBALL_URL="https://github.com/netdata/netdata/releases/download/$latest/netdata-latest.gz.run"
    export NETDATA_TARBALL_CHECKSUM_URL="https://github.com/netdata/netdata/releases/download/$latest/sha256sums.txt"
  else
    export NETDATA_TARBALL_URL="$NETDATA_TARBALL_BASEURL/netdata-latest.gz.run"
    export NETDATA_TARBALL_CHECKSUM_URL="$NETDATA_TARBALL_BASEURL/sha256sums.txt"
  fi
}

safe_sha256sum() {
  # Within the context of the installer, we only use -c option that is common between the two commands
  # We will have to reconsider if we start using non-common options
  if command -v sha256sum > /dev/null 2>&1; then
    sha256sum "$@"
  elif command -v shasum > /dev/null 2>&1; then
    shasum -a 256 "$@"
  else
    fatal "I could not find a suitable checksum binary to use"
  fi
}

mark_install_type() {
  install_type_file="/opt/netdata/etc/netdata/.install-type"
  if [ -f "${install_type_file}" ]; then
    # shellcheck disable=SC1090
    . "${install_type_file}"
    cat > "${TMPDIR}/install-type" <<- EOF
	INSTALL_TYPE='kickstart-static'
	PREBUILT_ARCH='${PREBUILT_ARCH}'
	EOF
    ${sudo} chown netdata:netdata "${TMPDIR}/install-type"
    ${sudo} cp "${TMPDIR}/install-type" "${install_type_file}"
  fi
}

claim() {
  progress "Attempting to claim agent to ${NETDATA_CLAIM_URL}"
  NETDATA_CLAIM_PATH=/opt/netdata/bin/netdata-claim.sh

  if ${sudo} "${NETDATA_CLAIM_PATH}" -token=${NETDATA_CLAIM_TOKEN} -rooms=${NETDATA_CLAIM_ROOMS} -url=${NETDATA_CLAIM_URL} ${NETDATA_CLAIM_EXTRA}; then
    progress "Successfully claimed node"
    return 0
  else
    run_failed "Unable to claim node, you must do so manually."
    return 1
  fi
}

# ----------------------------------------------------------------------------
umask 022

sudo=""
[ -z "${UID}" ] && UID="$(id -u)"
[ "${UID}" -ne "0" ] && sudo="sudo"

# ----------------------------------------------------------------------------
if [ "$(uname -m)" != "x86_64" ]; then
  fatal "Static binary versions of netdata are available only for 64bit Intel/AMD CPUs (x86_64), but yours is: $(uname -m)."
fi

if [ "$(uname -s)" != "Linux" ]; then
  fatal "Static binary versions of netdata are available only for Linux, but this system is $(uname -s)"
fi

# ----------------------------------------------------------------------------
opts=
NETDATA_INSTALLER_OPTIONS=""
NETDATA_UPDATES="--auto-update"
RELEASE_CHANNEL="nightly"
while [ -n "${1}" ]; do
  case "${1}" in
    "--dont-wait") opts="${opts} --accept" ;;
    "--non-interactive") opts="${opts} --accept" ;;
    "--accept") opts="${opts} --accept" ;;
    "--dont-start-it")
      NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS:+${NETDATA_INSTALLER_OPTIONS} }${1}"
      NETDATA_CLAIM_EXTRA="${NETDATA_CLAIM_EXTRA} -daemon-not-running"
      ;;
    "--no-updates") NETDATA_UPDATES="" ;;
    "--stable-channel")
      RELEASE_CHANNEL="stable"
      NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS:+${NETDATA_INSTALLER_OPTIONS} }${1}"
      ;;
    "--disable-telemetry") NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS:+${NETDATA_INSTALLER_OPTIONS} }${1}";;
    "--local-files")
      NETDATA_UPDATES="" # Disable autoupdates if using pre-downloaded files.
      if [ -z "${2}" ]; then
        fatal "Option --local-files requires extra information. The desired tarball full filename is needed"
      fi

      NETDATA_LOCAL_TARBALL_OVERRIDE="${2}"

      if [ -z "${3}" ]; then
        fatal "Option --local-files requires a pair of the tarball source and the checksum file"
      fi

      NETDATA_LOCAL_TARBALL_OVERRIDE_CHECKSUM="${3}"
      shift 2
      ;;
    "--allow-duplicate-install") NETDATA_ALLOW_DUPLICATE_INSTALL=1 ;;
    "--reinstall") NETDATA_REINSTALL=1 ;;
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
    "--claim-proxy")
      NETDATA_CLAIM_EXTRA="${NETDATA_CLAIM_EXTRA} -proxy ${2}"
      shift 1
      ;;
    *)
      echo >&2 "Unknown option '${1}' or invalid number of arguments. Please check the README for the available arguments of ${0} and try again"
      exit 1
  esac
  shift 1
done

if [ ! "${DO_NOT_TRACK:-0}" -eq 0 ] || [ -n "$DO_NOT_TRACK" ]; then
  NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS:+${NETDATA_INSTALLER_OPTIONS} }--disable-telemetry"
fi

if [ -n "${NETDATA_DISABLE_CLOUD}" ]; then
  if [ -n "${NETDATA_CLAIM_TOKEN}" ] || [ -n "${NETDATA_CLAIM_ROOMS}" ] || [ -n "${NETDATA_CLAIM_URL}" ]; then
    run_failed "Cloud explicitly disabled but automatic claiming requested."
    run_failed "Either enable Netdata Cloud, or remove the --claim-* options."
    exit 1
  fi
fi

# shellcheck disable=SC2235,SC2030
if ( [ -z "${NETDATA_CLAIM_TOKEN}" ] && [ -n "${NETDATA_CLAIM_URL}" ] ) || ( [ -n "${NETDATA_CLAIM_TOKEN}" ] && [ -z "${NETDATA_CLAIM_URL}" ] ); then
  run_failed "Invalid claiming options, both a claiming token and URL must be specified."
  exit 1
elif [ -z "${NETDATA_CLAIM_TOKEN}" ] && [ -n "${NETDATA_CLAIM_ROOMS}" ]; then
  run_failed "Invalid claiming options, claim rooms may only be specified when a token and URL are specified."
  exit 1
fi

# Netdata Tarball Base URL (defaults to our Google Storage Bucket)
[ -z "$NETDATA_TARBALL_BASEURL" ] && NETDATA_TARBALL_BASEURL=https://storage.googleapis.com/netdata-nightlies

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
      if [ -n "${NETDATA_CLAIM_TOKEN}" ] ; then
        claim
        exit $?
      elif [ -x "${ndprefix}/usr/libexec/netdata/netdata-updater.sh" ] ; then
        progress "Attempting to update existing install instead of creating a new one"
        if run ${sudo} "${ndprefix}/usr/libexec/netdata/netdata-updater.sh" --not-running-from-cron ; then
          progress "Updated existing install at ${ndpath}"
          exit 0
        else
          fatal "Failed to update existing Netdata install"
          exit 1
        fi
      else
        if [ -z "${NETDATA_ALLOW_DUPLICATE_INSTALL}" ] || [ "${ndstatic}" = "no" ] ; then
          fatal "Existing installation detected which cannot be safely updated by this script, refusing to continue."
          exit 1
        else
          progress "User explicitly requested duplicate install, proceeding."
        fi
      fi
    else
      if [ "${ndstatic}" = "yes" ] ; then
        progress "User requested reinstall instead of update, proceeding."
      else
        fatal "Existing install is not a static install, please use kickstart.sh instead."
        exit 1
      fi
    fi
  else
    progress "Existing install appears to be handled manually or through the system package manager."
    if [ -n "${NETDATA_CLAIM_TOKEN}" ] ; then
      claim
      exit $?
    elif [ -z "${NETDATA_ALLOW_DUPLICATE_INSTALL}" ] ; then
      fatal "Existing installation detected which cannot be safely updated by this script, refusing to continue."
      exit 1
    else
      progress "User explicitly requested duplicate install, proceeding."
    fi
  fi
fi

# ----------------------------------------------------------------------------
TMPDIR=$(create_tmp_directory)
cd "${TMPDIR}" || exit 1

if [ -z "${NETDATA_LOCAL_TARBALL_OVERRIDE}" ]; then
  set_tarball_urls "${RELEASE_CHANNEL}"
  progress "Downloading static netdata binary: ${NETDATA_TARBALL_URL}"

  download "${NETDATA_TARBALL_CHECKSUM_URL}" "${TMPDIR}/sha256sum.txt"
  download "${NETDATA_TARBALL_URL}" "${TMPDIR}/netdata-latest.gz.run"
else
  progress "Installation sources were given as input, running installation using \"${NETDATA_LOCAL_TARBALL_OVERRIDE}\""
  run cp "${NETDATA_LOCAL_TARBALL_OVERRIDE}" "${TMPDIR}/netdata-latest.gz.run"
  run cp "${NETDATA_LOCAL_TARBALL_OVERRIDE_CHECKSUM}" "${TMPDIR}/sha256sum.txt"
fi

if ! grep netdata-latest.gz.run "${TMPDIR}/sha256sum.txt" | safe_sha256sum -c - > /dev/null 2>&1; then
  fatal "Static binary checksum validation failed. Stopping Netdata Agent installation and leaving binary in ${TMPDIR}\nUsually this is a result of an older copy of the file being cached somewhere upstream and can be resolved by retrying in an hour."
fi

# ----------------------------------------------------------------------------
progress "Installing netdata"
run ${sudo} sh "${TMPDIR}/netdata-latest.gz.run" ${opts} -- ${NETDATA_UPDATES} ${NETDATA_INSTALLER_OPTIONS}

mark_install_type

#shellcheck disable=SC2181
if [ $? -eq 0 ]; then
  run ${sudo} rm "${TMPDIR}/netdata-latest.gz.run"
  if [ ! "${TMPDIR}" = "/" ] && [ -d "${TMPDIR}" ]; then
    run ${sudo} rm -rf "${TMPDIR}"
  fi
else
  echo >&2 "NOTE: did not remove: ${TMPDIR}/netdata-latest.gz.run"
  exit 1
fi

# --------------------------------------------------------------------------------------------------------------------

if [ -n "${NETDATA_CLAIM_TOKEN}" ]; then
  claim
fi
