#!/bin/sh
#
# SPDX-License-Identifier: GPL-3.0-or-later

# ======================================================================
# Constants

KICKSTART_OPTIONS="${*}"
PACKAGES_SCRIPT="https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/install-required-packages.sh"
PATH="${PATH}:/usr/local/bin:/usr/local/sbin"
REPOCONFIG_URL_PREFIX="https://packagecloud.io/netdata/netdata-repoconfig/packages"
REPOCONFIG_VERSION="1-1"
TELEMETRY_URL="https://posthog.netdata.cloud/capture/"
START_TIME="$(date +%s)"

# ======================================================================
# Defaults for environment variables

SELECTED_INSTALL_METHOD="none"
INSTALL_TYPE="unknown"
INSTALL_PREFIX=""
NETDATA_AUTO_UPDATES="1"
NETDATA_CLAIM_ONLY=0
NETDATA_CLAIM_URL="https://app.netdata.cloud"
NETDATA_DISABLE_CLOUD=0
NETDATA_ONLY_BUILD=0
NETDATA_ONLY_NATIVE=0
NETDATA_ONLY_STATIC=0
NETDATA_REQUIRE_CLOUD=1
RELEASE_CHANNEL="nightly"

NETDATA_DISABLE_TELEMETRY="${DO_NOT_TRACK:-0}"
NETDATA_TARBALL_BASEURL="${NETDATA_TARBALL_BASEURL:-https://storage.googleapis.com/netdata-nightlies}"
NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS:-""}"
TELEMETRY_API_KEY="${NETDATA_POSTHOG_API_KEY:-mqkwGT0JNFqO-zX2t0mW6Tec9yooaVu7xCBlXtHnt5Y}"

if echo "${0}" | grep -q 'kickstart-static64'; then
  NETDATA_ONLY_STATIC=1
fi

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
  --build-only               Only install using a local build.
  --reinstall                Explicitly reinstall instead of updating any existing install.
  --reinstall-even-if-unsafe Even try to reinstall if we don't think we can do so safely (implies --reinstall).
  --disable-cloud            Disable support for Netdata Cloud (default: detect)
  --require-cloud            Only install if Netdata Cloud can be enabled. Overrides --disable-cloud.
  --install <path>           Specify an installation prefix for local builds (default: autodetect based on system type).
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
  NETDATA_INSTALLER_OPTIONS: Specifies extra options to pass to the static installer or local build script.

HEREDOC
}

# ======================================================================
# Telemetry functions

telemetry_event() {
  if [ "${NETDATA_DISABLE_TELEMETRY}" -eq 1 ]; then
    return 0
  fi

  now="$(date +%s)"
  total_duration="$((now - START_TIME))"

  if [ -e "/etc/os-release" ]; then
    eval "$(grep -E "^(NAME|ID|ID_LIKE|VERSION|VERSION_ID)=" < /etc/os-release | sed 's/^/HOST_/')"
  fi

  if [ -z "${HOST_NAME}" ] || [ -z "${HOST_VERSION}" ] || [ -z "${HOST_ID}" ]; then
    if [ -f "/etc/lsb-release" ]; then
      DISTRIB_ID="unknown"
      DISTRIB_RELEASE="unknown"
      DISTRIB_CODENAME="unknown"
      eval "$(grep -E "^(DISTRIB_ID|DISTRIB_RELEASE|DISTRIB_CODENAME)=" < /etc/lsb-release)"
      if [ -z "${HOST_NAME}" ]; then HOST_NAME="${DISTRIB_ID}"; fi
      if [ -z "${HOST_VERSION}" ]; then HOST_VERSION="${DISTRIB_RELEASE}"; fi
      if [ -z "${HOST_ID}" ]; then HOST_ID="${DISTRIB_CODENAME}"; fi
    fi
  fi

  KERNEL_NAME="$(uname -s)"

  if [ "${KERNEL_NAME}" = FreeBSD ]; then
    TOTAL_RAM="$(sysctl -n hw.physmem)"
  elif [ "${KERNEL_NAME}" = Darwin ]; then
    TOTAL_RAM="$(sysctl -n hw.memsize)"
  elif [ -r /proc/meminfo ]; then
    TOTAL_RAM="$(grep -F MemTotal /proc/meminfo | cut -f 2 -d ':' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' | cut -f 1 -d ' ')"
    TOTAL_RAM="$((TOTAL_RAM * 1024))"
  fi

  if [ -f /etc/machine-id ]; then
    DISTINCT_ID="$(cat /etc/machine-id)"
  elif command -v uuidgen > /dev/null 2>&1; then
    DISTINCT_ID="$(uuidgen)"
  else
    DISTINCT_ID="null"
  fi

  REQ_BODY="$(cat << EOF
{
  "api_key": "${TELEMETRY_API_KEY}",
  "event": "${1}",
  "properties": {
    "distinct_id": "${DISTINCT_ID}",
    "event_source": "agent installer",
    "\$current_url": "agent installer",
    "\$pathname": "netdata-installer",
    "\$host": "installer.netdata.io",
    "\$ip": "127.0.0.1",
    "script_variant": "kickstart-ng",
    "error_code": "${3}",
    "error_message": "${2}",
    "install_options": "${KICKSTART_OPTIONS}",
    "install_interactivity": "${INTERACTIVE}",
    "total_runtime": "${total_duration}",
    "selected_install_method": "${SELECTED_INSTALL_METHOD}",
    "netdata_release_channel": "${RELEASE_CHANNEL:-null}",
    "netdata_install_type": "${INSTALL_TYPE}",
    "host_os_name": "${HOST_NAME:-unknown}",
    "host_os_id": "${HOST_ID:-unknown}",
    "host_os_id_like": "${HOST_ID_LIKE:-unknown}",
    "host_os_version": "${HOST_VERSION:-unknown}",
    "host_os_version_id": "${HOST_VERSION_ID:-unknown}",
    "system_kernel_name": "${KERNEL_NAME}",
    "system_kernel_version": "$(uname -r)",
    "system_architecture": "$(uname -m)",
    "system_total_ram": "${TOTAL_RAM:-unknown}"
  }
}
EOF
)"

  if command -v curl > /dev/null 2>&1; then
    curl --silent -o /dev/null -X POST --max-time 2 --header "Content-Type: application/json" -d "${REQ_BODY}" "${TELEMETRY_URL}" > /dev/null
  elif command -v wget > /dev/null 2>&1; then
    wget -q -O - --no-check-certificate \
    --method POST \
    --timeout=1 \
    --header 'Content-Type: application/json' \
    --body-data "${REQ_BODY}" \
     "${TELEMETRY_URL}" > /dev/null
  fi
}

trap_handler() {
  code="${1}"
  lineno="${2}"

  printf >&2 "%s\n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ERROR ${TPUT_RESET} Installer exited unexpectedly (${code}-${lineno})"

  telemetry_event INSTALL_CRASH "Installer exited unexpectedly (${code}-${lineno})" "E${code}-${lineno}"

  trap - EXIT

  cleanup

  exit 1
}

trap 'trap_handler 0 ${LINENO}' EXIT
trap 'trap_handler 1 0' HUP
trap 'trap_handler 2 0' INT
trap 'trap_handler 3 0' QUIT
trap 'trap_handler 13 0' PIPE
trap 'trap_handler 15 0' TERM

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

cleanup() {
  if [ -z "${NO_CLEANUP}" ]; then
    ${ROOTCMD} rm -rf "${tmpdir}"
  fi
}

fatal() {
  printf >&2 "%s\n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ABORTED ${TPUT_RESET} ${1}"
  telemetry_event "INSTALL_FAILED" "${1}" "${2}"
  cleanup
  trap - EXIT
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
        fatal "Unable to find a usable temporary directory. Please set \$TMPDIR to a path that is both writable and allows execution of files and try again." F0400
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
    run curl --fail -q -sSL --connect-timeout 10 --retry 3 --output "${dest}" "${url}" || return 1
  elif command -v wget > /dev/null 2>&1; then
    run wget -T 15 -O "${dest}" "${url}" || return 1
  else
    fatal "I need curl or wget to proceed, but neither of them are available on this system." F0003
  fi
}

get_redirect() {
  url="${1}"

  if command -v curl > /dev/null 2>&1; then
    run sh -c "curl ${url} -s -L -I -o /dev/null -w '%{url_effective}' | grep -o '[^/]*$'" || return 1
  elif command -v wget > /dev/null 2>&1; then
    run sh -c "wget --max-redirect=0 ${url} 2>&1 | grep Location | cut -d ' ' -f2  | grep -o '[^/]*$'" || return 1
  else
    fatal "I need curl or wget to proceed, but neither of them are available on this system." F0003
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
    fatal "I could not find a suitable checksum binary to use" F0004
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
        warning "Cannot find an os-release file ..."
      fi

      if [ -n "${os_release_file}" ]; then
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
            rocky|rhel)
                DISTRO_COMPAT_NAME="centos"
                SYSVERSION=$(echo "$SYSVERSION" | cut -d'.' -f1)
                ;;
            *)
                DISTRO_COMPAT_NAME="unknown"
                ;;
            esac
        fi
      else
        DISTRO="unknown"
        DISTRO_COMPAT_NAME="unknown"
        SYSVERSION="unknown"
        SYSCODENAME="unknown"
        SYSARCH="$(uname -m)"
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
      fatal "Unsupported system type detected. Netdata cannot be installed on this system using this script." F0200
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
      if [ "${INTERACTIVE}" -eq 0 ]; then
        ROOTCMD="sudo -n"
      else
        ROOTCMD="sudo"
      fi
    fi

    if [ -z "${ROOTCMD}" ] && command -v doas > /dev/null; then
      if [ "${INTERACTIVE}" -eq 0 ]; then
        ROOTCMD="doas -n"
      else
        ROOTCMD="doas"
      fi
    fi

    if [ -z "${ROOTCMD}" ] && command -v pkexec > /dev/null; then
      ROOTCMD="pkexec"
    fi

    if [ -z "${ROOTCMD}" ]; then
      fatal "We need root privileges to continue, but cannot find a way to gain them. Either re-run this script as root, or set \$ROOTCMD to a command that can be used to gain root privileges" F0201
    fi
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

# ======================================================================
# Existing install handling code

update() {
  updater="${ndprefix}/usr/libexec/netdata/netdata-updater.sh"

  if [ -x "${updater}" ]; then
    if run ${ROOTCMD} "${updater}" --not-running-from-cron; then
      progress "Updated existing install at ${ndprefix}"
      return 0
    else
      fatal "Failed to update existing Netdata install at ${ndprefix}" F0100
    fi
  else
    return 1
  fi
}

handle_existing_install() {
  if pkg_installed netdata; then
    ndprefix="/"
  else
    if [ -n "${INSTALL_PREFIX}" ]; then
      searchpath="${INSTALL_PREFIX}/bin:${INSTALL_PREFIX}/sbin:${INSTALL_PREFIX}/usr/bin:${INSTALL_PREFIX}/usr/sbin:${PATH}"
      searchpath="${INSTALL_PREFIX}/netdata/bin:${INSTALL_PREFIX}/netdata/sbin:${INSTALL_PREFIX}/netdata/usr/bin:${INSTALL_PREFIX}/netdata/usr/sbin:${searchpath}"
    else
      searchpath="${PATH}"
    fi

    ndpath="$(PATH="${searchpath}" command -v netdata 2>/dev/null)"

    if [ -z "$ndpath" ] && [ -x /opt/netdata/bin/netdata ]; then
      ndpath="/opt/netdata/bin/netdata"
    fi

    if [ -n "${ndpath}" ]; then
      ndprefix="$(dirname "$(dirname "${ndpath}")")"
    fi

    if echo "${ndprefix}" | grep -Eq '/usr$'; then
      ndprefix="$(dirname "${ndprefix}")"
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
    kickstart-*|legacy-*|binpkg-*|manual-static|unknown)
      if [ "${INSTALL_TYPE}" = "unknown" ]; then
        warning "Found an existing netdata install at ${ndprefix}, but could not determine the install type."
      else
        progress "Found an existing netdata install at ${ndprefix}, with installation type '${INSTALL_TYPE}'."
      fi

      if [ -n "${NETDATA_REINSTALL}" ] || [ -n "${NETDATA_UNSAFE_REINSTALL}" ]; then
        progress "Found an existing netdata install at ${ndprefix}, but user requested reinstall, continuing."

        case "${INSTALL_TYPE}" in
          binpkg-*) NETDATA_ONLY_NATIVE=1 ;;
          *-build) NETDATA_ONLY_BUILD=1 ;;
          *-static) NETDATA_ONLY_STATIC=1 ;;
          *)
            if [ -n "${NETDATA_UNSAFE_REINSTALL}" ]; then
              warning "Reinstalling over top of a ${INSTALL_TYPE} installation may be unsafe, but the user has requested we proceed."
            elif [ "${INTERACTIVE}" -eq 0 ]; then
              fatal "User requested reinstall, but we cannot safely reinstall over top of a ${INSTALL_TYPE} installation, exiting." F0104
            else
              if confirm "Reinstalling over top of a ${INSTALL_TYPE} installation may be unsafe, do you want to continue?"; then
                progress "OK, continuing."
              else
                fatal "Cancelling reinstallation at user request." F0105
              fi
            fi
            ;;
        esac

        return 0
      fi

      ret=0

      if [ "${NETDATA_CLAIM_ONLY}" -eq 0 ] && echo "${INSTALL_TYPE}" | grep -vq "binpkg-*"; then
        if ! update; then
          warning "Unable to find usable updater script, not updating existing install at ${ndprefix}."
        fi
      else
        warning "Not updating existing install at ${ndprefix}."
      fi

      if [ -n "${NETDATA_CLAIM_TOKEN}" ]; then
        progress "Attempting to claim existing install at ${ndprefix}."
        INSTALL_PREFIX="${ndprefix}"
        claim
        ret=$?
      elif [ "${NETDATA_CLAIM_ONLY}" -eq 1 ]; then
        fatal "User asked to claim, but did not proide a claiming token." F0202
      else
        progress "Not attempting to claim existing install at ${ndprefix} (no claiming token provided)."
      fi

      cleanup
      trap - EXIT
      exit $ret
      ;;
    oci)
      fatal "This is an OCI container, use the regular image lifecycle management commands in your container instead of this script for managing it." F0203
      ;;
    *)
      if [ -n "${NETDATA_REINSTALL}" ] || [ -n "${NETDATA_UNSAFE_REINSTALL}" ]; then
        if [ -n "${NETDATA_UNSAFE_REINSTALL}" ]; then
          warning "Reinstalling over top of a ${INSTALL_TYPE} installation may be unsafe, but the user has requested we proceed."
        elif [ "${INTERACTIVE}" -eq 0 ]; then
          fatal "User requested reinstall, but we cannot safely reinstall over top of a ${INSTALL_TYPE} installation, exiting." F0104
        else
          if confirm "Reinstalling over top of a ${INSTALL_TYPE} installation may be unsafe, do you want to continue?"; then
            progress "OK, continuing."
          else
            fatal "Cancelling reinstallation at user request." F0105
          fi
        fi
      else
        fatal "Found an existing netdata install at ${ndprefix}, but the install type is '${INSTALL_TYPE}', which is not supported, refusing to proceed." F0103
      fi
      ;;
  esac
}

soft_disable_cloud() {
  cloud_prefix="${INSTALL_PREFIX}/var/lib/netdata/cloud.d"

  run ${ROOTCMD} mkdir -p "${cloud_prefix}"

  cat > "${tmpdir}/cloud.conf" << EOF
[global]
  enabled = no
EOF

  run ${ROOTCMD} cp "${tmpdir}/cloud.conf" "${cloud_prefix}/cloud.conf"

  if [ -z "${NETDATA_NO_START}" ]; then
    case "${SYSTYPE}" in
      Darwin) run ${ROOTCMD} launchctl kickstart -k com.github.netdata ;;
      FreeBSD) run ${ROOTCMD} service netdata restart ;;
      Linux)
        initpath="$(${ROOTCMD} readlink /proc/1/exe)"

        if command -v service > /dev/null 2>&1; then
          run ${ROOTCMD} service netdata restart
        elif command -v rc-service > /dev/null 2>&1; then
          run ${ROOTCMD} rc-service netdata restart
        elif [ "$(basename "${initpath}" 2> /dev/null)" = "systemd" ]; then
          run ${ROOTCMD} systemctl restart netdata
        elif [ -f /etc/init.d/netdata ]; then
          run ${ROOTCMD} /etc/init.d/netdata restart
        fi
        ;;
    esac
  fi
}

confirm_install_prefix() {
  if [ -n "${INSTALL_PREFIX}" ] && [ "${NETDATA_ONLY_BUILD}" -ne 1 ]; then
    fatal "The \`--install\` option is only supported together with the \`--only-build\` option." F0204
  fi

  if [ -n "${INSTALL_PREFIX}" ]; then
    NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} --install ${INSTALL_PREFIX}"
  else
    case "${SYSTYPE}" in
      Darwin)
        INSTALL_PREFIX="/usr/local/netdata"
        NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} --install-no-prefix ${INSTALL_PREFIX}"
        ;;
      FreeBSD)
        INSTALL_PREFIX="/usr/local"
        NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} --install-no-prefix ${INSTALL_PREFIX}"
        ;;
    esac
  fi
}

# ======================================================================
# Claiming support code

check_claim_opts() {
# shellcheck disable=SC2235,SC2030
  if [ -z "${NETDATA_CLAIM_TOKEN}" ] && [ -n "${NETDATA_CLAIM_ROOMS}" ]; then
    fatal "Invalid claiming options, claim rooms may only be specified when a token and URL are specified." F0204
  elif [ -z "${NETDATA_CLAIM_TOKEN}" ] && [ -n "${NETDATA_CLAIM_EXTRA}" ]; then
    fatal "Invalid claiming options, a claiming token must be specified." F0204
  elif [ "${NETDATA_DISABLE_CLOUD}" -eq 1 ] && [ -n "${NETDATA_CLAIM_TOKEN}" ]; then
    fatal "Cloud explicitly disabled, but automatic claiming requested. Either enable Netdata Cloud, or remove the --claim-* options." F0204
  fi
}

is_netdata_running() {
  if command -v pgrep > /dev/null 2>&1; then
    if pgrep netdata; then
      return 0
    else
      return 1
    fi
  else
    if [ -z "${INSTALL_PREFIX}" ]; then
      NETDATACLI_PATH=/usr/sbin/netdatacli
    elif [ "${INSTALL_PREFIX}" = "/opt/netdata" ]; then
      NETDATACLI_PATH="/opt/netdata/bin/netdatacli"
    else
      NETDATACLI_PATH="${INSTALL_PREFIX}/netdata/usr/sbin/netdatacli"
    fi

    if "${NETDATACLI_PATH}" ping > /dev/null 2>&1; then
      return 0
    else
      return 1
    fi
  fi
}

claim() {
  progress "Attempting to claim agent to ${NETDATA_CLAIM_URL}"
  if [ -z "${INSTALL_PREFIX}" ] || [ "${INSTALL_PREFIX}" = "/" ]; then
    NETDATA_CLAIM_PATH=/usr/sbin/netdata-claim.sh
  elif [ "${INSTALL_PREFIX}" = "/opt/netdata" ]; then
    NETDATA_CLAIM_PATH="/opt/netdata/bin/netdata-claim.sh"
  else
    NETDATA_CLAIM_PATH="${INSTALL_PREFIX}/netdata/usr/sbin/netdata-claim.sh"
  fi

  if ! is_netdata_running; then
    NETDATA_CLAIM_EXTRA="${NETDATA_CLAIM_EXTRA} -daemon-not-running"
  fi

  # shellcheck disable=SC2086
  if ${ROOTCMD} "${NETDATA_CLAIM_PATH}" -token="${NETDATA_CLAIM_TOKEN}" -rooms="${NETDATA_CLAIM_ROOMS}" -url="${NETDATA_CLAIM_URL}" ${NETDATA_CLAIM_EXTRA}; then
    progress "Successfully claimed node"
  else
    warning "Unable to claim node, you must do so manually."
    if [ -z "${NETDATA_NEW_INSTALL}" ]; then
      cleanup
      trap - EXIT
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
      dpkg-query --show --showformat '${Status}' "${1}" 2>&1 | cut -f 1 -d ' ' | grep -q '^install$'
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
      uninstall_subcmd="purge"
      INSTALL_TYPE="binpkg-deb"
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
      uninstall_subcmd="purge"
      INSTALL_TYPE="binpkg-deb"
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
      INSTALL_TYPE="binpkg-rpm"
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
      INSTALL_TYPE="binpkg-rpm"
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
      INSTALL_TYPE="binpkg-rpm"
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
        fatal "Failed to update repository metadata." F0205
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

  if [ "${NETDATA_DISABLE_TELEMETRY}" -eq 1 ]; then
    run ${ROOTCMD} mkdir -p "/etc/netdata"
    run ${ROOTCMD} touch "/etc/netdata/.opt-out-from-anonymous-statistics"
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
    latest="$(get_redirect "https://github.com/netdata/netdata/releases/latest")"
    export NETDATA_STATIC_ARCHIVE_URL="https://github.com/netdata/netdata/releases/download/${latest}/netdata-${SYSARCH}-latest.gz.run"
    export NETDATA_STATIC_ARCHIVE_CHECKSUM_URL="https://github.com/netdata/netdata/releases/download/${latest}/sha256sums.txt"
  else
    export NETDATA_STATIC_ARCHIVE_URL="${NETDATA_TARBALL_BASEURL}/netdata-${SYSARCH}-latest.gz.run"
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
    fatal "Unable to fetch checksums to verify static build archive." F0206
  fi

  if ! grep "netdata-${SYSARCH}-latest.gz.run" "${tmpdir}/sha256sum.txt" | safe_sha256sum -c - > /dev/null 2>&1; then
    fatal "Static binary checksum validation failed. Usually this is a result of an older copy of the file being cached somewhere upstream and can be resolved by retrying in an hour." F0207
  fi

  if [ "${INTERACTIVE}" -eq 0 ]; then
    opts="${opts} --accept"
  fi

  progress "Installing netdata"
  # shellcheck disable=SC2086
  if ! run ${ROOTCMD} sh "${tmpdir}/netdata-${SYSARCH}-latest.gz.run" ${opts} -- ${NETDATA_AUTO_UPDATES:+--auto-update} ${NETDATA_INSTALLER_OPTIONS}; then
    warning "Failed to install static build of Netdata on ${SYSARCH}."
    run rm -rf /opt/netdata
    return 2
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
# Local build install code

set_source_archive_urls() {
  if [ "$1" = "stable" ]; then
    latest="$(get_redirect "https://github.com/netdata/netdata/releases/latest")"
    export NETDATA_SOURCE_ARCHIVE_URL="https://github.com/netdata/netdata/releases/download/${latest}/netdata-${latest}.tar.gz"
    export NETDATA_SOURCE_ARCHIVE_CHECKSUM_URL="https://github.com/netdata/netdata/releases/download/${latest}/sha256sums.txt"
  else
    export NETDATA_SOURCE_ARCHIVE_URL="${NETDATA_TARBALL_BASEURL}/netdata-latest.tar.gz"
    export NETDATA_SOURCE_ARCHIVE_CHECKSUM_URL="${NETDATA_TARBALL_BASEURL}/sha256sums.txt"
  fi
}

install_local_build_dependencies() {
  bash="$(command -v bash 2> /dev/null)"

  if [ -z "${bash}" ] || [ ! -x "${bash}" ]; then
    warning "Unable to find a usable version of \`bash\` (required for local build)."
    return 1
  fi

  progress "Fetching script to detect required packages..."
  download "${PACKAGES_SCRIPT}" "${tmpdir}/install-required-packages.sh"

  if [ ! -s "${tmpdir}/install-required-packages.sh" ]; then
    warning "Downloaded dependency installation script is empty."
  else
    progress "Running downloaded script to detect required packages..."

    if [ "${INTERACTIVE}" -eq 0 ]; then
      opts="--dont-wait --non-interactive"
    fi

    if [ "${SYSTYPE}" = "Darwin" ]; then
      sudo=""
    else
      sudo="${ROOTCMD}"
    fi

    # shellcheck disable=SC2086
    if ! run ${sudo} "${bash}" "${tmpdir}/install-required-packages.sh" ${opts} netdata; then
      warning "It failed to install all the required packages, but installation might still be possible."
    fi
  fi
}

build_and_install() {
  progress "Building netdata"

  echo "INSTALL_TYPE='kickstart-build'" > system/.install-type

  opts="${NETDATA_INSTALLER_OPTIONS}"

  if [ "${INTERACTIVE}" -eq 0 ]; then
    opts="${opts} --dont-wait"
  fi

  if [ "${NETDATA_AUTO_UPDATES}" -eq 1 ]; then
    opts="${opts} --auto-update"
  fi

  if [ "${RELEASE_CHANNEL}" = "stable" ]; then
    opts="${opts} --stable-channel"
  fi

  if [ "${NETDATA_REQUIRE_CLOUD}" -eq 1 ]; then
    opts="${opts} --require-cloud"
  elif [ "${NETDATA_DISABLE_CLOUD}" -eq 1 ]; then
    opts="${opts} --disable-cloud"
  fi

  # shellcheck disable=SC2086
  run ${ROOTCMD} ./netdata-installer.sh ${opts}

  case $? in
    1)
      fatal "netdata-installer.sh exited with error" F0007
      ;;
    2)
      fatal "Insufficient RAM to install netdata" F0008
      ;;
  esac
}

try_build_install() {
  if ! install_local_build_dependencies; then
    return 1
  fi

  set_source_archive_urls "${RELEASE_CHANNEL}"

  download "${NETDATA_SOURCE_ARCHIVE_CHECKSUM_URL}" "${tmpdir}/sha256sum.txt"
  download "${NETDATA_SOURCE_ARCHIVE_URL}" "${tmpdir}/netdata-latest.tar.gz"

  if ! grep netdata-latest.tar.gz "${tmpdir}/sha256sum.txt" | safe_sha256sum -c - > /dev/null 2>&1; then
    fatal "Tarball checksum validation failed. Usually this is a result of an older copy of the file being cached somewhere upstream and can be resolved by retrying in an hour." F0005
  fi

  run tar -xf "${tmpdir}/netdata-latest.tar.gz" -C "${tmpdir}"
  rm -rf "${tmpdir}/netdata-latest.tar.gz" > /dev/null 2>&1
  cd "$(find "${tmpdir}" -mindepth 1 -maxdepth 1 -type d -name netdata-)" || fatal "Cannot cd to netdata source tree" F0006

  if [ -x netdata-installer.sh ]; then
    build_and_install || return 1
  else
    # This case is needed because some platforms produce an extra directory on the source tarball extraction.
    if [ "$(find . -mindepth 1 -maxdepth 1 -type d | wc -l)" -eq 1 ] && [ -x "$(find . -mindepth 1 -maxdepth 1 -type d)/netdata-installer.sh" ]; then
      cd "$(find . -mindepth 1 -maxdepth 1 -type d)" &&  build_and_install || return 1
    else
      fatal "Cannot install netdata from source (the source directory does not include netdata-installer.sh). Leaving all files in ${tmpdir}" F0009
    fi
  fi
}

# ======================================================================
# Per system-type install logic

install_on_linux() {
  if [ "${NETDATA_ONLY_STATIC}" -ne 1 ] && [ "${NETDATA_ONLY_BUILD}" -ne 1 ]; then
    SELECTED_INSTALL_METHOD="native"
    try_package_install

    case "$?" in
      0)
        NETDATA_INSTALL_SUCCESSFUL=1
        ;;
      1)
        fatal "Unable to install on this system." F0300
        ;;
      2)
        if [ "${NETDATA_ONLY_NATIVE}" -eq 1 ]; then
          fatal "Could not install native binary packages." F0301
        else
          warning "Could not install native binary packages, falling back to alternative installation method."
        fi
        ;;
    esac
  fi

  if [ "${NETDATA_ONLY_NATIVE}" -ne 1 ] && [ "${NETDATA_ONLY_BUILD}" -ne 1 ] && [ -z "${NETDATA_INSTALL_SUCCESSFUL}" ]; then
    SELECTED_INSTALL_METHOD="static"
    INSTALL_TYPE="kickstart-static"
    try_static_install

    case "$?" in
      0)
        NETDATA_INSTALL_SUCCESSFUL=1
        INSTALL_PREFIX="/opt/netdata"
        ;;
      1)
        fatal "Unable to install on this system." F0302
        ;;
      2)
        if [ "${NETDATA_ONLY_STATIC}" -eq 1 ]; then
          fatal "Could not install static build." F0303
        else
          warning "Could not install static build, falling back to alternative installation method."
        fi
        ;;
    esac
  fi

  if [ "${NETDATA_ONLY_NATIVE}" -ne 1 ] && [ "${NETDATA_ONLY_STATIC}" -ne 1 ] && [ -z "${NETDATA_INSTALL_SUCCESSFUL}" ]; then
    SELECTED_INSTALL_METHOD="build"
    INSTALL_TYPE="kickstart-build"
    try_build_install

    case "$?" in
      0)
        NETDATA_INSTALL_SUCCESSFUL=1
        ;;
      *)
        fatal "Unable to install on this system." F0304
        ;;
    esac
  fi
}

install_on_macos() {
  if [ "${NETDATA_ONLY_NATIVE}" -eq 1 ]; then
    fatal "User requested native package, but native packages are not available for macOS. Try installing without \`--only-native\` option." F0305
  elif [ "${NETDATA_ONLY_STATIC}" -eq 1 ]; then
    fatal "User requested static build, but static builds are not available for macOS. Try installing without \`--only-static\` option." F0306
  else
    SELECTED_INSTALL_METHOD="build"
    INSTALL_TYPE="kickstart-build"
    try_build_install

    case "$?" in
      0)
        NETDATA_INSTALL_SUCCESSFUL=1
        ;;
      *)
        fatal "Unable to install on this system." F0307
        ;;
    esac
  fi
}

install_on_freebsd() {
  if [ "${NETDATA_ONLY_NATIVE}" -eq 1 ]; then
    fatal "User requested native package, but native packages are not available for FreeBSD. Try installing without \`--only-native\` option." F0308
  elif [ "${NETDATA_ONLY_STATIC}" -eq 1 ]; then
    fatal "User requested static build, but static builds are not available for FreeBSD. Try installing without \`--only-static\` option." F0309
  else
    SELECTED_INSTALL_METHOD="build"
    INSTALL_TYPE="kickstart-build"
    try_build_install

    case "$?" in
      0)
        NETDATA_INSTALL_SUCCESSFUL=1
        ;;
      *)
        fatal "Unable to install on this system." F030A
        ;;
    esac
  fi
}

# ======================================================================
# Main program

setup_terminal || echo > /dev/null

while [ -n "${1}" ]; do
  case "${1}" in
    "--help")
      usage
      cleanup
      trap - EXIT
      exit 0
      ;;
    "--no-cleanup") NO_CLEANUP=1 ;;
    "--dont-wait"|"--non-interactive") INTERACTIVE=0 ;;
    "--interactive") INTERACTIVE=1 ;;
    "--stable-channel") RELEASE_CHANNEL="stable" ;;
    "--no-updates") NETDATA_AUTO_UPDATES=0 ;;
    "--auto-update") NETDATA_AUTO_UPDATES="1" ;;
    "--reinstall") NETDATA_REINSTALL=1 ;;
    "--reinstall-even-if-unsafe") NETDATA_UNSAFE_REINSTALL=1 ;;
    "--claim-only") NETDATA_CLAIM_ONLY=1 ;;
    "--disable-cloud")
      NETDATA_DISABLE_CLOUD=1
      NETDATA_REQUIRE_CLOUD=0
      ;;
    "--require-cloud")
      NETDATA_DISABLE_CLOUD=0
      NETDATA_REQUIRE_CLOUD=1
      ;;
    "--dont-start-it")
      NETDATA_NO_START=1
      NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} --dont-start-it"
      ;;
    "--disable-telemetry")
      NETDATA_DISABLE_TELEMETRY="1"
      NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} --disable-telemetry"
      ;;
    "--install")
      INSTALL_PREFIX="${2}"
      shift 1
      ;;
    "--native-only")
      NETDATA_ONLY_NATIVE=1
      NETDATA_ONLY_STATIC=0
      NETDATA_ONLY_BUILD=0
      SELECTED_INSTALL_METHOD="native"
      ;;
    "--static-only")
      NETDATA_ONLY_STATIC=1
      NETDATA_ONLY_NATIVE=0
      NETDATA_ONLY_BUILD=0
      SELECTED_INSTALL_METHOD="static"
      ;;
    "--build-only")
      NETDATA_ONLY_BUILD=1
      NETDATA_ONLY_NATIVE=0
      NETDATA_ONLY_STATIC=0
      SELECTED_INSTALL_METHOD="build"
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
    *)
      warning "Passing unrecognized option '${1}' to installer script. If this is intended, please add it to \$NETDATA_INSTALLER_OPTIONS instead."
      NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} ${1}"
      ;;
  esac
  shift 1
done

check_claim_opts
confirm_root_support
get_system_info
confirm_install_prefix

tmpdir="$(create_tmp_directory)"
progress "Using ${tmpdir} as a temporary directory."
cd "${tmpdir}" || exit 1

handle_existing_install

case "${SYSTYPE}" in
  Linux) install_on_linux ;;
  Darwin) install_on_macos ;;
  FreeBSD) install_on_freebsd ;;
esac

if [ -n "${NETDATA_CLAIM_TOKEN}" ]; then
  claim
elif [ "${NETDATA_DISABLE_CLOUD}" -eq 1 ]; then
  soft_disable_cloud
fi

telemetry_event INSTALL_SUCCESS "" ""
cleanup
trap - EXIT
