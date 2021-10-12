#!/bin/sh
#
# SPDX-License-Identifier: GPL-3.0-or-later

# ======================================================================
# Constants

REPOCONFIG_URL_PREFIX="https://packagecloud.io/netdata/netdata-repoconfig/packages"
REPOCONFIG_VERSION="1-1"
PATH="${PATH}:/usr/local/bin:/usr/local/sbin"

# ======================================================================
# Defaults for environment variables

RELEASE_CHANNEL="nightly"
NETDATA_CLAIM_URL="https://app.netdata.cloud"
NETDATA_CLAIM_ONLY=0
NETDATA_USER_CONFIG_DIR="/etc/netdata"
NETDATA_AUTO_UPDATES="1"
NETDATA_ONLY_NATIVE=0
NETDATA_ONLY_STATIC=0

NETDATA_DISABLE_TELEMETRY="${DO_NOT_TRACK:-0}"
NETDATA_TARBALL_BASEURL="${NETDATA_TARBALL_BASEURL:-https://storage.googleapis.com/netdata-nightlies}"

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

  --non-interactive          Do not prompt for user input. (default: prompt if there is a controlling terminal)
  --interactive              Prompt for user input even if there is no controlling terminal.
  --dont-start-it            Do not start the agent by default (only for static installs or local builds)
  --stable-channel           Install a stable version instead of a nightly build (default: install a nightly build)
  --nightly-channel          Install a nightly build instead of a stable version
  --no-updates               Do not enable automatic updates (default: enable automatic updates)
  --auto-update              Enable automatic updates.
  --disable-telemetry        Opt-out of anonymous statistics.
  --native-only              Only install if native binary packages are available.
  --static-only              Only install if a static build is available.
  --reinstall                Explicitly reinstall instead of updating any existing install.
  --claim-token              Use a specified token for claiming to Netdata Cloud.
  --claim-rooms              When claiming, add the node to the specified rooms.
  --claim-only               If there is an existing install, only try to claim it, not update it.
  --claim-*                  Specify other options for the claiming script.
  --no-cleanup               Don't do any cleanup steps. This is intended to help with debugging the installer.

Additionally, this script may use the following environment variables:

  TMPDIR:                    Used to specify where to put temporary files. On most systems, the default we select
                             automatically should be fine. The user running the script needs to both be able to
                             write files to the temporary directory, and run files from that location.
  ROOTCMD:                   Used to specify a command to use to run another command with root privileges if needed. By
                             default we try to use sudo, doas, or pkexec (in that order of preference), but if
                             you need special options for one of those to work, or have a different tool to do
                             the same thing on your system, you can specify it here.
  DO_NOT_TRACK               If set to a value other than 0, behave as if \`--disable-telemetry\` was specified.

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

warning() {
  printf >&2 "%s\n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} WARNING ${TPUT_RESET} ${*}"
}

_cannot_use_tmpdir() {
  testfile="$(TMPDIR="${1}" mktemp -q -t netdata-test.XXXXXXXXXX)"
  ret=0

  if [ -z "${testfile}" ]; then
    return "${ret}"
  fi

  if printf '#!/bin/sh\necho SUCCESS\n' > "${testfile}"; then
    if chmod +x "${testfile}"; then
      if [ "$("${testfile}")" = "SUCCESS" ]; then
        ret=1
      fi
    fi
  fi

  rm -f "${testfile}"
  return "${ret}"
}

create_tmp_directory() {
  if [ -z "${TMPDIR}" ] || _cannot_use_tmpdir "${TMPDIR}"; then
    if _cannot_use_tmpdir /tmp; then
      if _cannot_use_tmpdir "${PWD}"; then
        fatal "Unable to find a usable temporary directory. Please set \$TMPDIR to a path that is both writable and allows execution of files and try again."
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
        fatal "Cannot find an os-release file ..."
      fi

      # shellcheck disable=SC1090
      . "${os_release_file}"

      DISTRO="${ID}"
      SYSVERSION="${VERSION_ID}"
      SYSCODENAME="${VERSION_CODENAME}"
      SYSARCH="$(uname -m)"

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
      SYSARCH="$(uname -m)"
      ;;
    FreeBSD)
      SYSTYPE="FreeBSD"
      SYSVERSION="$(uname -K)"
      SYSARCH="$(uname -m)"
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

handle_existing_install() {
  if pkg_installed netdata; then
    ndprefix="/"
  else
    ndpath="$(command -v netdata 2>/dev/null)"
    if [ -z "$ndpath" ] && [ -x /opt/netdata/bin/netdata ]; then
      ndpath="/opt/netdata/bin/netdata"
    fi

    if [ -n "${ndpath}" ]; then
      ndprefix="$(dirname "$(dirname "${ndpath}")")"
    fi

    if [ "${ndprefix}" = /usr ]; then
      ndprefix="/"
    fi
  fi

  if [ -n "${ndprefix}" ]; then
    typefile="${ndprefix}/etc/netdata/.install-type"
    if [ -r "${typefile}" ]; then
      ${ROOTCMD} sh -c "cat \"${typefile}\" > \"${tmpdir}/install-type\""
      # shellcheck disable=SC1091
      . "${tmpdir}/install-type"
    else
      INSTALL_TYPE="unknown"
    fi
  fi

  if [ -z "${ndprefix}" ]; then
    progress "No existing installations of netdata found, assuming this is a fresh install."
    return 0
  fi

  case "${INSTALL_TYPE}" in
    kickstart-*|legacy-*|manual-static)
      if [ -n "${NETDATA_REINSTALL}" ]; then
        progress "Found an existing netdata install at ${ndprefix}, but user requested reinstall, continuing."
        return 0
      fi

      ret=0

      if [ "${NETDATA_CLAIM_ONLY}" -eq 0 ]; then
        if ! update; then
          warning "Unable to find usable updater script, not updating existing install at ${ndprefix}."
        fi
      fi

      if [ -n "${NETDATA_CLAIM_TOKEN}" ]; then
        progress "Attempting to claim existing install at ${ndprefix}."
        claim
        ret=$?
      elif [ "${NETDATA_CLAIM_ONLY}" -eq 1 ]; then
        fatal "User asked to claim, but did not proide a claiming token."
      fi

      exit $ret
      ;;
    binpkg-*)
      ret=0

      if [ -n "${NETDATA_CLAIM_TOKEN}" ]; then
        progress "Attempting to claim existing install at ${ndprefix}."
        claim
        ret=$?
      fi

      exit $ret
      ;;
    oci)
      fatal "This is an OCI container, use the regular image lifecycle management commands in your container instead of this script for managing it."
      ;;
    unknown)
      warning "Found an existing netdata install at ${ndprefix}, but could not determine the install type."

      if [ -n "${NETDATA_REINSTALL}" ]; then
        progress "Found an existing netdata install at ${ndprefix}, but user requested reinstall, continuing."
        return 0
      fi

      ret=0

      if [ "${NETDATA_CLAIM_ONLY}" -eq 0 ]; then
        if ! update; then
          warning "Unable to find usable updater script, not updating existing install at ${ndprefix}."
        fi
      fi

      if [ -n "${NETDATA_CLAIM_TOKEN}" ]; then
        progress "Attempting to claim existing install at ${ndprefix}."
        claim
        ret=$?
      elif [ "${NETDATA_CLAIM_ONLY}" -eq 1 ]; then
        fatal "User asked to claim, but did not proide a claiming token."
      fi

      exit $ret
      ;;
    *)
      fatal "Found an existing netdata install at ${ndprefix}, but it is not a supported install type, refusing to proceed."
      ;;
  esac
}

# ======================================================================
# Claiming support code

check_claim_opts() {
# shellcheck disable=SC2235,SC2030
  if [ -z "${NETDATA_CLAIM_TOKEN}" ] && [ -n "${NETDATA_CLAIM_ROOMS}" ]; then
    fatal "Invalid claiming options, claim rooms may only be specified when a token and URL are specified."
  elif [ -z "${NETDATA_CLAIM_TOKEN}" ] && [ -n "${NETDATA_CLAIM_EXTRA}" ]; then
    fatal "Invalid claiming options, a claiming token must be specified."
  fi
}

claim() {
  progress "Attempting to claim agent to ${NETDATA_CLAIM_URL}"
  if [ -z "${NETDATA_PREFIX}" ]; then
    NETDATA_CLAIM_PATH=/usr/sbin/netdata-claim.sh
  elif [ "${NETDATA_PREFIX}" = "/opt/netdata" ]; then
    NETDATA_CLAIM_PATH="/opt/netdata/bin/netdata-claim.sh"
  else
    NETDATA_CLAIM_PATH="${NETDATA_PREFIX}/netdata/usr/sbin/netdata-claim.sh"
  fi

  if ! pgrep netdata > /dev/null; then
    NETDATA_CLAIM_EXTRA="${NETDATA_CLAIM_EXTRA} -daemon-not-running"
  fi

  # shellcheck disable=SC2086
  if ${ROOTCMD} "${NETDATA_CLAIM_PATH}" -token="${NETDATA_CLAIM_TOKEN}" -rooms="${NETDATA_CLAIM_ROOMS}" -url="${NETDATA_CLAIM_URL}" ${NETDATA_CLAIM_EXTRA}; then
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
    *)
      return 1
      ;;
  esac
}

# Check for the existence of a usable netdata package in the repo.
netdata_avail_check() {
  case "${DISTRO_COMPAT_NAME}" in
    debian|ubuntu)
      env DEBIAN_FRONTEND=noninteractive apt-cache policy netdata | grep -q packagecloud.io/netdata/netdata;
      return $?
      ;;
    centos|fedora)
      # shellcheck disable=SC2086
      ${pm_cmd} search -v netdata | grep -qE 'Repo *: netdata(-edge)?$'
      return $?
      ;;
    opensuse)
      zypper packages -r "$(zypper repos | grep -E 'netdata |netdata-edge ' | cut -f 1 -d '|' | tr -d ' ')" | grep -E 'netdata '
      return $?
      ;;
    *)
      return 1
      ;;
  esac
}

# Check for any distro-specific dependencies we know we need.
check_special_native_deps() {
  if [ "${DISTRO_COMPAT_NAME}" = "centos" ] && [ "${SYSVERSION}" = "7" ]; then
    progress "Checking for libuv availability."
    # shellcheck disable=SC2086
    if ${pm_cmd} search ${interactive_opts} -v libuv | grep -q "No matches found"; then
      progress "libv not found, checking for EPEL availability."
      # shellcheck disable=SC2086
      if ${pm_cmd} search ${interactive_opts} -v epel-release | grep -q "No matches found"; then
        warning "Unable to find a suitable source for libuv, cannot install on this system."
        return 1
      else
        progress "EPEL is available, attempting to install so that required dependencies are available."

        # shellcheck disable=SC2086
        if ! run ${ROOTCMD} env ${env} ${pm_cmd} install ${pkg_install_opts} epel-release; then
          warning "Failed to install EPEL."
          return 1
        fi
      fi
    else
      return 0
    fi
  fi
}

try_package_install() {
  if [ -z "${DISTRO}" ]; then
    warning "Unable to determine Linux distribution for native packages."
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
    env="DEBIAN_FRONTEND=noninteractive"
  else
    interactive_opts=""
    env=""
  fi

  case "${DISTRO_COMPAT_NAME}" in
    debian)
      needs_early_refresh=1
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
      needs_early_refresh=1
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
    if ! download "${repoconfig_url}" "${tmpdir}/${repoconfig_file}"; then
      warning "Failed to download repository configuration package."
      return 2
    fi

    if [ -n "${needs_early_refresh}" ]; then
      progress "Updating repository metadata."
      # shellcheck disable=SC2086
      if ! run ${ROOTCMD} env ${env} ${pm_cmd} ${repo_subcmd} ${repo_update_opts}; then
        warning "Failed to refresh repository metadata."
        return 2
      fi
    fi

    progress "Installing repository configuration package."
    # shellcheck disable=SC2086
    if ! run ${ROOTCMD} env ${env} ${pm_cmd} install ${pkg_install_opts} "${tmpdir}/${repoconfig_file}"; then
      warning "Failed to install repository configuration package."
      return 2
    fi

    if [ -n "${repo_subcmd}" ]; then
      progress "Updating repository metadata."
      # shellcheck disable=SC2086
      if ! run ${ROOTCMD} env ${env} ${pm_cmd} ${repo_subcmd} ${repo_update_opts}; then
        fatal "Failed to update repository metadata."
      fi
    fi
  else
    progress "Repository configuration is already present, attempting to install netdata."
  fi

  if ! check_special_native_deps; then
    warning "Could not find secondary dependencies ${DISTRO} on ${SYSARCH}."
    if [ -z "${NO_CLEANUP}" ]; then
      progress "Attempting to uninstall repository configuration package."
      # shellcheck disable=SC2086
      run ${ROOTCMD} env ${env} ${pm_cmd} ${uninstall_subcmd} ${pkg_install_opts} "${repoconfig_name}"
    fi
    return 2
  fi

  progress "Checking for usable Netdata package."
  if ! netdata_avail_check "${DISTRO_COMPAT_NAME}"; then
    warning "Could not find a usable native package for ${DISTRO} on ${SYSARCH}."
    if [ -z "${NO_CLEANUP}" ]; then
      progress "Attempting to uninstall repository configuration package."
      # shellcheck disable=SC2086
      run ${ROOTCMD} env ${env} ${pm_cmd} ${uninstall_subcmd} ${pkg_install_opts} "${repoconfig_name}"
    fi
    return 2
  fi

  progress "Installing Netdata package."
  # shellcheck disable=SC2086
  if ! run ${ROOTCMD} env ${env} ${pm_cmd} install ${pkg_install_opts} netdata; then
    warning "Failed to install Netdata package."
    if [ -z "${NO_CLEANUP}" ]; then
      progress "Attempting to uninstall repository configuration package."
      # shellcheck disable=SC2086
      run ${ROOTCMD} env ${env} ${pm_cmd} ${uninstall_subcmd} ${pkg_install_opts} "${repoconfig_name}"
    fi
    return 2
  fi
}

# ======================================================================
# Static build install code

set_static_archive_urls() {
  if [ "${RELEASE_CHANNEL}" = "stable" ]; then
    latest="$(download "https://api.github.com/repos/netdata/netdata/releases/latest" /dev/stdout | grep tag_name | cut -d'"' -f4)"
    export NETDATA_STATIC_ARCHIVE_URL="https://github.com/netdata/netdata/releases/download/${latest}/netdata-${SYSARCH}-${latest}.gz.run"
    export NETDATA_STATIC_ARCHIVE_CHECKSUM_URL="https://github.com/netdata/netdata/releases/download/${latest}/sha256sums.txt"
  else
    export NETDATA_STATIC_ARCHIVE_URL="${NETDATA_TARBALL_BASEURL}/netdata-latest.gz.run"
    export NETDATA_STATIC_ARCHIVE_CHECKSUM_URL="${NETDATA_TARBALL_BASEURL}/sha256sums.txt"
  fi
}

try_static_install() {
  set_static_archive_urls "${RELEASE_CHANNEL}"
  progress "Downloading static netdata binary: ${NETDATA_STATIC_ARCHIVE_URL}"

  if ! download "${NETDATA_STATIC_ARCHIVE_URL}" "${tmpdir}/netdata-${SYSARCH}-latest.gz.run"; then
    warning "Unable to download static build archive for ${SYSARCH}."
    return 2
  fi

  if ! download "${NETDATA_STATIC_ARCHIVE_CHECKSUM_URL}" "${tmpdir}/sha256sum.txt"; then
    fatal "Unable to fetch checksums to verify static build archive."
  fi

  if ! grep "netdata-${SYSARCH}-latest.gz.run" "${tmpdir}/sha256sum.txt" | safe_sha256sum -c - > /dev/null 2>&1; then
    fatal "Static binary checksum validation failed. Stopping Netdata Agent installation and leaving binary in ${tmpdir}. Usually this is a result of an older copy of the file being cached somewhere upstream and can be resolved by retrying in an hour."
  fi

  if [ "${INTERACTIVE}" -eq 0 ]; then
    opts="${opts} --accept"
  fi

  progress "Installing netdata"
  # shellcheck disable=SC2086
  if ! run ${ROOTCMD} sh "${tmpdir}/netdata-${SYSARCH}-latest.gz.run" ${opts} -- ${NETDATA_AUTO_UPDATES:+--auto-update} ${NETDATA_INSTALLER_OPTIONS}; then
    fatal "Failed to install static build of Netdata on ${SYSARCH}."
  fi

  install_type_file="/opt/netdata/etc/netdata/.install-type"
  if [ -f "${install_type_file}" ]; then
    ${ROOTCMD} sh -c "cat \"${install_type_file}\" > \"${tmpdir}/install-type\""
    ${ROOTCMD} chown "$(id -u)":"$(id -g)" "${tmpdir}/install-type"
    # shellcheck disable=SC1091
    . "${tmpdir}/install-type"
    cat > "${tmpdir}/install-type" <<- EOF
	INSTALL_TYPE='kickstart-static'
	PREBUILT_ARCH='${PREBUILT_ARCH}'
	EOF
    ${ROOTCMD} chown netdata:netdata "${tmpdir}/install-type"
    ${ROOTCMD} cp "${tmpdir}/install-type" "${install_type_file}"
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
    "--dont-wait"|"--non-interactive") INTERACTIVE=0 ;;
    "--interactive") INTERACTIVE=1 ;;
    "--stable-channel") RELEASE_CHANNEL="stable" ;;
    "--no-updates") NETDATA_AUTO_UPDATES="" ;;
    "--auto-update") NETDATA_AUTO_UPDATES="1" ;;
    "--disable-telemetry") NETDATA_DISABLE_TELEMETRY="1" ;;
    "--reinstall") NETDATA_REINSTALL=1 ;;
    "--claim-only") NETDATA_CLAIM_ONLY=1 ;;
    "--native-only")
      NETDATA_ONLY_NATIVE=1
      NETDATA_ONLY_STATIC=0
      ;;
    "--static-only")
      NETDATA_ONLY_STATIC=1
      NETDATA_ONLY_NATIVE=0
      ;;
    "--dont-start-it")
      NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} --dont-start-it"
      ;;
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
      case "${optname}" in
        id|proxy|user|hostname)
          NETDATA_CLAIM_EXTRA="${NETDATA_CLAIM_EXTRA} -${optname} ${2}"
          shift 1
          ;;
        verbose|insecure|noproxy|noreload|daemon-not-running)
          NETDATA_CLAIM_EXTRA="${NETDATA_CLAIM_EXTRA} -${optname}"
          ;;
        *)
          warning "Ignoring unrecognized claiming option ${optname}"
          ;;
      esac
      ;;
  esac
  shift 1
done

check_claim_opts
confirm_root_support
get_system_info

tmpdir="$(create_tmp_directory)"
progress "Using ${tmpdir} as a temporary directory."
cd "${tmpdir}" || exit 1

handle_existing_install

case "${SYSTYPE}" in
  Linux)
    if [ "${NETDATA_ONLY_STATIC}" -ne 1 ]; then
      try_package_install

      case "$?" in
        0)
          NETDATA_INSTALL_SUCCESSFUL=1
          ;;
        1)
          fatal "Unable to install on this system."
          ;;
        2)
          if [ "${NETDATA_ONLY_NATIVE}" -eq 1 ]; then
            fatal "Could not install native binary packages."
          else
            warning "Could not install native binary packages, falling back to alternative installation method."
          fi
          ;;
      esac
    fi

    if [ "${NETDATA_ONLY_NATIVE}" -ne 1 ] && [ -z "${NETDATA_INSTALL_SUCCESSFUL}" ]; then
      try_static_install

      case "$?" in
        0)
          NETDATA_INSTALL_SUCCESSFUL=1
          NETDATA_USER_CONFIG_DIR="/opt/netdata/etc/netdata"
          ;;
        1)
          fatal "Unable to install on this system."
          ;;
        2)
          if [ "${NETDATA_ONLY_STATIC}" -eq 1 ]; then
            fatal "Could not install static build."
          else
            warning "Could not install static build, falling back to alternative installation method."
          fi
          ;;
      esac
    fi
    ;;
  Darwin)
    if [ "${NETDATA_ONLY_NATIVE}" -eq 1 ]; then
      fatal "User requested native package, but native packages are not available for macOS. Try installing without \`--only-native\` option."
    elif [ "${NETDATA_ONLY_STATIC}" -eq 1 ]; then
      fatal "User requested static build, but static builds are not available for macOS. Try installing without \`--only-static\` option."
    else
      fatal "This script currently does not support installation on macOS."
    fi
    ;;
  FreeBSD)
    if [ "${NETDATA_ONLY_NATIVE}" -eq 1 ]; then
      fatal "User requested native package, but native packages are not available for FreeBSD. Try installing without \`--only-native\` option."
    elif [ "${NETDATA_ONLY_STATIC}" -eq 1 ]; then
      fatal "User requested static build, but static builds are not available for FreeBSD. Try installing without \`--only-static\` option."
    else
      fatal "This script currently does not support installation on FreeBSD."
    fi
    ;;
esac

if [ "${NETDATA_DISABLE_TELEMETRY}" -eq 1 ]; then
  run ${ROOTCMD} touch "${NETDATA_USER_CONFIG_DIR}/.opt-out-from-anonymous-statistics"
fi

if [ -n "${NETDATA_CLAIM_TOKEN}" ]; then
  claim
fi

if [ -z "${NO_CLEANUP}" ]; then
  ${ROOTCMD} rm -rf "${tmpdir}"
fi
