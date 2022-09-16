#!/bin/sh
#
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Next unused error code: F050A

# ======================================================================
# Constants

AGENT_BUG_REPORT_URL="https://github.com/netdata/netdata/issues/new/choose"
CLOUD_BUG_REPORT_URL="https://github.com/netdata/netdata-cloud/issues/new/choose"
DEFAULT_RELEASE_CHANNEL="nightly"
DISCORD_INVITE="https://discord.gg/5ygS846fR6"
DISCUSSIONS_URL="https://github.com/netdata/netdata/discussions"
DISCUSSIONS_URL="https://github.com/netdata/netdata/discussions"
DOCS_URL="https://learn.netdata.cloud/docs/"
FORUM_URL="https://community.netdata.cloud/"
KICKSTART_OPTIONS="${*}"
KICKSTART_SOURCE="$(
    self=${0}
    while [ -L "${self}" ]
    do
        cd "${self%/*}" || exit 1
        self=$(readlink "${self}")
    done
    cd "${self%/*}" || exit 1
    echo "$(pwd -P)/${self##*/}"
)"
PACKAGES_SCRIPT="https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/install-required-packages.sh"
PATH="${PATH}:/usr/local/bin:/usr/local/sbin"
PUBLIC_CLOUD_URL="https://app.netdata.cloud"
REPOCONFIG_URL_PREFIX="https://packagecloud.io/netdata/netdata-repoconfig/packages"
REPOCONFIG_VERSION="1-2"
START_TIME="$(date +%s)"
STATIC_INSTALL_ARCHES="x86_64 armv7l aarch64 ppc64le"
TELEMETRY_URL="https://posthog.netdata.cloud/capture/"

# ======================================================================
# Defaults for environment variables

DRY_RUN=0
SELECTED_INSTALL_METHOD="none"
INSTALL_TYPE="unknown"
INSTALL_PREFIX=""
NETDATA_AUTO_UPDATES="default"
NETDATA_CLAIM_ONLY=0
NETDATA_CLAIM_URL="${PUBLIC_CLOUD_URL}"
NETDATA_DISABLE_CLOUD=0
NETDATA_ONLY_BUILD=0
NETDATA_ONLY_NATIVE=0
NETDATA_ONLY_STATIC=0
NETDATA_OFFLINE_INSTALL_SOURCE=""
NETDATA_REQUIRE_CLOUD=1
NETDATA_WARNINGS=""
RELEASE_CHANNEL="default"

if [ -n "$DISABLE_TELEMETRY" ]; then
  NETDATA_DISABLE_TELEMETRY="${DISABLE_TELEMETRY}"
elif [ -n "$DO_NOT_TRACK" ]; then
  NETDATA_DISABLE_TELEMETRY="${DO_NOT_TRACK}"
else
  NETDATA_DISABLE_TELEMETRY=0
fi

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
# Core program logic

main() {
  case "${ACTION}" in
    uninstall)
      uninstall
      printf >&2 "Finished uninstalling the Netdata Agent."
      deferred_warnings
      cleanup
      trap - EXIT
      exit 0
      ;;
    reinstall-clean)
      NEW_INSTALL_PREFIX="${INSTALL_PREFIX}"
      uninstall
      cleanup

      ACTION=
      INSTALL_PREFIX="${NEW_INSTALL_PREFIX}"
      # shellcheck disable=SC2086
      main

      trap - EXIT
      exit 0
      ;;
    prepare-offline)
      prepare_offline_install_source "${OFFLINE_TARGET}"
      deferred_warnings
      trap - EXIT
      exit 0
      ;;
  esac

  set_tmpdir

  if [ -n "${INSTALL_VERSION}" ]; then
    if echo "${INSTALL_VERSION}" | grep -E -o "^[[:digit:]]+\.[[:digit:]]+\.[[:digit:]]+$" > /dev/null 2>&1; then
      NEW_SELECTED_RELEASE_CHANNEL="stable"
    else
      NEW_SELECTED_RELEASE_CHANNEL="nightly"
    fi

    if ! [ "${NEW_SELECTED_RELEASE_CHANNEL}" = "${SELECTED_RELEASE_CHANNEL}" ]; then
        warning "Selected release channel does not match this version and it will be changed automatically."
        SELECTED_RELEASE_CHANNEL="${NEW_SELECTED_RELEASE_CHANNEL}"
    fi
  fi

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

  set_auto_updates

  printf >&2 "%s\n\n" "Successfully installed the Netdata Agent."
  deferred_warnings
  success_banner
  telemetry_event INSTALL_SUCCESS "" ""
  cleanup
  trap - EXIT
}

# ======================================================================
# Usage info

usage() {
  cat << HEREDOC
USAGE: kickstart.sh [options]
       where options include:

  --non-interactive                Do not prompt for user input. (default: prompt if there is a controlling terminal)
  --interactive                    Prompt for user input even if there is no controlling terminal.
  --dont-start-it                  Do not start the agent by default (only for static installs or local builds)
  --dry-run                        Report what we would do with the given options on this system, but don’t actually do anything.
  --release-channel                Specify the release channel to use for the install (default: ${DEFAULT_RELEASE_CHANNEL})
  --stable-channel                 Equivalent to "--release-channel stable"
  --nightly-channel                Equivalent to "--release-channel nightly"
  --no-updates                     Do not enable automatic updates (default: enable automatic updates using the best supported scheduling method)
  --auto-update                    Enable automatic updates.
  --auto-update-type               Specify a particular scheduling type for auto-updates (valid types: systemd, interval, crontab)
  --disable-telemetry              Opt-out of anonymous statistics.
  --repositories-only              Only install appropriate repository configuration packages (only for native install).
  --native-only                    Only install if native binary packages are available.
  --static-only                    Only install if a static build is available.
  --build-only                     Only install using a local build.
  --reinstall                      Explicitly reinstall instead of updating any existing install.
  --reinstall-even-if-unsafe       Even try to reinstall if we don't think we can do so safely (implies --reinstall).
  --disable-cloud                  Disable support for Netdata Cloud (default: detect)
  --require-cloud                  Only install if Netdata Cloud can be enabled. Overrides --disable-cloud.
  --install <path>                 This option is deprecated and will be removed in a future version, use --install-prefix instead.
  --install-prefix <path>          Specify an installation prefix for local builds (default: autodetect based on system type).
  --old-install-prefix <path>      Specify an old local builds installation prefix for uninstall/reinstall (if it's not default).
  --install-version <version>      Specify the version of Netdata to install.
  --claim-token                    Use a specified token for claiming to Netdata Cloud.
  --claim-rooms                    When claiming, add the node to the specified rooms.
  --claim-only                     If there is an existing install, only try to claim it, not update it.
  --claim-*                        Specify other options for the claiming script.
  --no-cleanup                     Don't do any cleanup steps. This is intended to help with debugging the installer.
  --uninstall                      Uninstall an existing installation of Netdata.
  --reinstall-clean                Clean reinstall Netdata.
  --local-build-options            Specify additional options to pass to the installer code when building locally. Only valid if --build-only is also specified.
  --static-install-options         Specify additional options to pass to the static installer code. Only valid if --static-only is also specified.
  --prepare-offline-install-source Instead of installing the agent, prepare a directory that can be used to install on another system without needing to download anything.

Additionally, this script may use the following environment variables:

  TMPDIR:                    Used to specify where to put temporary files. On most systems, the default we select
                             automatically should be fine. The user running the script needs to both be able to
                             write files to the temporary directory, and run files from that location.
  ROOTCMD:                   Used to specify a command to use to run another command with root privileges if needed. By
                             default we try to use sudo, doas, or pkexec (in that order of preference), but if
                             you need special options for one of those to work, or have a different tool to do
                             the same thing on your system, you can specify it here.
  DISABLE_TELEMETRY          If set to a value other than 0, behave as if \`--disable-telemetry\` was specified.

HEREDOC
}

# ======================================================================
# Telemetry functions

telemetry_event() {
  if [ "${NETDATA_DISABLE_TELEMETRY}" -eq 1 ] || [ "${DRY_RUN}" -eq 1 ]; then
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
    DISTINCT_ID="$(uuidgen | tr '[:upper:]' '[:lower:]')"
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
    "install_auto_updates": "${NETDATA_AUTO_UPDATES}",
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

  deferred_warnings

  printf >&2 "%s\n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ERROR ${TPUT_RESET} Installer exited unexpectedly (${code}-${lineno})"

  case "${code}" in
    0)
      printf >&2 "%s\n" "This is almost certainly the result of a bug. If you have time, please report it at ${AGENT_BUG_REPORT_URL}."
      ;;
    *)
      printf >&2 "%s\n" "This is probably a result of a transient issue on your system. Things should work correctly if you try again."
      printf >&2 "%s\n" "If you continue to experience this issue, you can reacn out to us for support on:"
      support_list
      ;;
  esac

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
    if num_colors=$(tput colors 2> /dev/null) && [ "${num_colors:-0}" -ge 8 ]; then
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

support_list() {
  printf >&2 "%s\n" "  - GitHub: ${DISCUSSIONS_URL}"
  printf >&2 "%s\n" "  - Discord: ${DISCORD_INVITE}"
  printf >&2 "%s\n" "  - Our community forums: ${FORUM_URL}"
}

success_banner() {
  printf >&2 "%s\n\n" "Official documentation can be found online at ${DOCS_URL}."

  if [ -z "${CLAIM_TOKEN}" ]; then
    printf >&2 "%s\n\n" "Looking to monitor all of your infrastructure with Netdata? Check out Netdata Cloud at ${PUBLIC_CLOUD_URL}."
  fi

  printf >&2 "%s\n" "Join our community and connect with us on:"
  support_list
}

cleanup() {
  if [ -z "${NO_CLEANUP}" ] && [ -n "${tmpdir}" ]; then
    cd || true
    ${ROOTCMD} rm -rf "${tmpdir}"
  fi
}

deferred_warnings() {
  if [ -n "${NETDATA_WARNINGS}" ]; then
    printf >&2 "%s\n" "The following non-fatal warnings or errors were encountered:"
    # shellcheck disable=SC2059
    printf >&2 "${NETDATA_WARNINGS}"
    printf >&2 "\n\n"
  fi
}

fatal() {
  deferred_warnings
  printf >&2 "%s\n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ABORTED ${TPUT_RESET} ${1}"
  printf >&2 "%s\n" "For community support, you can connect with us on:"
  support_list
  telemetry_event "INSTALL_FAILED" "${1}" "${2}"
  cleanup
  trap - EXIT
  exit 1
}

ESCAPED_PRINT_METHOD=
# shellcheck disable=SC3050
if printf "%s " test > /dev/null 2>&1; then
  ESCAPED_PRINT_METHOD="printfq"
fi

escaped_print() {
  if [ "${ESCAPED_PRINT_METHOD}" = "printfq" ]; then
    # shellcheck disable=SC3050
    printf "%s " "${@}"
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

  if [ "${DRY_RUN}" -eq 1 ]; then
    printf >&2 "%s" "Would run command:\n"
  fi

  {
    printf "%s" "${info}"
    escaped_print "${@}"
    printf " ... "
  } >> "${run_logfile}"

  printf >&2 "%s" "${info_console}${TPUT_BOLD}"
  escaped_print >&2 "${@}"
  printf >&2 "%s\n" "${TPUT_RESET}"

  if [ "${DRY_RUN}" -ne 1 ]; then
    "${@}"
    ret=$?
  else
    ret=0
  fi

  if [ ${ret} -ne 0 ]; then
    printf >&2 "%s\n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} FAILED ${TPUT_RESET}"
    printf "%s\n" "FAILED with exit code ${ret}" >> "${run_logfile}"
    # shellcheck disable=SC2089
    NETDATA_WARNINGS="${NETDATA_WARNINGS}\n  - Command \"${*}\" failed with exit code ${ret}."
  else
    printf >&2 "%s\n\n" "${TPUT_BGGREEN}${TPUT_WHITE}${TPUT_BOLD} OK ${TPUT_RESET}"
    printf "OK\n" >> "${run_logfile}"
  fi

  return ${ret}
}

warning() {
  printf >&2 "%s\n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} WARNING ${TPUT_RESET} ${*}"
  NETDATA_WARNINGS="${NETDATA_WARNINGS}\n  - ${*}"
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

set_tmpdir() {
  if [ -z "${tmpdir}" ]; then
    tmpdir="$(create_tmp_directory)"
    progress "Using ${tmpdir} as a temporary directory."
    cd "${tmpdir}" || fatal "Failed to change current working directory to ${tmpdir}." F000A
  fi
}

check_for_remote_file() {
  url="${1}"

  if echo "${url}" | grep -Eq "^file:///"; then
    [ -e "${url#file://}" ] || return 1
  elif command -v curl > /dev/null 2>&1; then
    curl --output /dev/null --silent --head --fail "${url}" || return 1
  elif command -v wget > /dev/null 2>&1; then
    wget -S --spider "${url}" 2>&1 | grep -q 'HTTP/1.1 200 OK' || return 1
  else
    fatal "I need curl or wget to proceed, but neither of them are available on this system." F0003
  fi
}

download() {
  url="${1}"
  dest="${2}"

  if echo "${url}" | grep -Eq "^file:///"; then
    run cp "${url#file://}" "${dest}" || return 1
  elif command -v curl > /dev/null 2>&1; then
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
  if command -v shasum > /dev/null 2>&1; then
    shasum -a 256 "$@"
  elif command -v sha256sum > /dev/null 2>&1; then
    sha256sum "$@"
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

        supported_compat_names="debian ubuntu centos fedora opensuse ol"

        if str_in_list "${DISTRO}" "${supported_compat_names}"; then
            DISTRO_COMPAT_NAME="${DISTRO}"
        else
            case "${DISTRO}" in
            opensuse-leap)
                DISTRO_COMPAT_NAME="opensuse"
                ;;
            almalinux|rocky|rhel)
                DISTRO_COMPAT_NAME="centos"
                ;;
            *)
                DISTRO_COMPAT_NAME="unknown"
                ;;
            esac
        fi

        case "${DISTRO_COMPAT_NAME}" in
          centos|ol)
            SYSVERSION=$(echo "$SYSVERSION" | cut -d'.' -f1)
            ;;
        esac
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
    if [ "${DRY_RUN}" -eq 1 ]; then
      progress "Would attempt to update existing installation by running the updater script located at: ${updater}"
      return 0
    fi

    if [ "${INTERACTIVE}" -eq 0 ]; then
        opts="--non-interactive"
    else
        opts="--interactive"
    fi

    export NETDATA_SAVE_WARNINGS=1
    export NETDATA_PROPAGATE_WARNINGS=1
    # shellcheck disable=SC2090
    export NETDATA_WARNINGS="${NETDATA_WARNINGS}"
    if run ${ROOTCMD} "${updater}" ${opts} --not-running-from-cron; then
      progress "Updated existing install at ${ndprefix}"
      return 0
    else
      if [ -n "${EXIT_REASON}" ]; then
        fatal "Failed to update existing Netdata install at ${ndprefix}: ${EXIT_REASON}" "${EXIT_CODE}"
      else
        fatal "Failed to update existing Netdata install at ${ndprefix}." U0000
      fi
    fi
  else
    warning "Could not find a usable copy of the updater script."
    return 1
  fi
}

uninstall() {
  set_tmpdir
  get_system_info
  detect_existing_install

  if [ -n "${OLD_INSTALL_PREFIX}" ]; then
    INSTALL_PREFIX="$(echo "${OLD_INSTALL_PREFIX}/" | sed 's/$/netdata/g')"
  else
    INSTALL_PREFIX="${ndprefix}"
  fi

  uninstaller="${INSTALL_PREFIX}/usr/libexec/netdata/netdata-uninstaller.sh"
  uninstaller_url="https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/netdata-uninstaller.sh"

  if [ $INTERACTIVE = 0 ]; then
    FLAGS="--yes --force"
  else
    FLAGS="--yes"
  fi

  if [ -x "${uninstaller}" ]; then
    if [ "${DRY_RUN}" -eq 1 ]; then
      progress "Would attempt to uninstall existing install with uninstaller script found at: ${uninstaller}"
      return 0
    else
      progress "Found existing netdata-uninstaller. Running it.."
      export NETDATA_SAVE_WARNINGS=1
      export NETDATA_PROPAGATE_WARNINGS=1
      # shellcheck disable=SC2090
      export NETDATA_WARNINGS="${NETDATA_WARNINGS}"
      if ! run ${ROOTCMD} "${uninstaller}" $FLAGS; then
        warning "Uninstaller failed. Some parts of Netdata may still be present on the system."
      fi
    fi
  else
    if [ "${DRY_RUN}" -eq 1 ]; then
      progress "Would download installer script from: ${uninstaller_url}"
      progress "Would attempt to uninstall existing install with downloaded uninstaller script."
      return 0
    else
      progress "Downloading netdata-uninstaller ..."
      download "${uninstaller_url}" "${tmpdir}/netdata-uninstaller.sh"
      chmod +x "${tmpdir}/netdata-uninstaller.sh"
      export NETDATA_SAVE_WARNINGS=1
      export NETDATA_PROPAGATE_WARNINGS=1
      # shellcheck disable=SC2090
      export NETDATA_WARNINGS="${NETDATA_WARNINGS}"
      if ! run ${ROOTCMD} "${tmpdir}/netdata-uninstaller.sh" $FLAGS; then
        warning "Uninstaller failed. Some parts of Netdata may still be present on the system."
      fi
    fi
  fi
}

detect_existing_install() {
  set_tmpdir

  progress "Checking for existing installations of Netdata..."

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

    if echo "${ndprefix}" | grep -Eq '^/usr$'; then
      ndprefix="$(dirname "${ndprefix}")"
    fi
  fi

  if [ -n "${ndprefix}" ]; then
    typefile="${ndprefix}/etc/netdata/.install-type"
    if [ -r "${typefile}" ]; then
      ${ROOTCMD} sh -c "cat \"${typefile}\" > \"${tmpdir}/install-type\""
      # shellcheck disable=SC1090,SC1091
      . "${tmpdir}/install-type"
    else
      INSTALL_TYPE="unknown"
    fi

    envfile="${ndprefix}/etc/netdata/.environment"
    if [ "${INSTALL_TYPE}" = "unknown" ] || [ "${INSTALL_TYPE}" = "custom" ]; then
      if [ -r "${envfile}" ]; then
        ${ROOTCMD} sh -c "cat \"${envfile}\" > \"${tmpdir}/environment\""
        # shellcheck disable=SC1091
        . "${tmpdir}/environment"
        if [ -n "${NETDATA_IS_STATIC_INSTALL}" ]; then
          if [ "${NETDATA_IS_STATIC_INSTALL}" = "yes" ]; then
            INSTALL_TYPE="legacy-static"
          else
            INSTALL_TYPE="legacy-build"
          fi
        fi
      fi
    fi
  fi
}

handle_existing_install() {
  detect_existing_install

  if [ -z "${ndprefix}" ] || [ -z "${INSTALL_TYPE}" ]; then
    progress "No existing installations of netdata found, assuming this is a fresh install."
    return 0
  fi

  case "${INSTALL_TYPE}" in
    kickstart-*|legacy-*|binpkg-*|manual-static|unknown)
      if [ "${INSTALL_TYPE}" = "unknown" ]; then
        warning "Found an existing netdata install at ${ndprefix}, but could not determine the install type."
        warning "Usually this means you installed Netdata through your distribution’s regular package repositories or some other unsupported method."
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
      elif [ "${INSTALL_TYPE}" = "unknown" ]; then
        fatal "We do not support trying to update or claim installations when we cannot determine the install type. You will need to uninstall the existing install using the same method you used to install it to proceed." F0106
      fi

      ret=0

      if [ "${NETDATA_CLAIM_ONLY}" -eq 0 ]; then
        if ! update; then
          warning "Failed to update existing Netdata install at ${ndprefix}."
        else
          progress "Successfully updated existing netdata install at ${ndprefix}."
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

      deferred_warnings
      success_banner
      cleanup
      trap - EXIT
      exit $ret
      ;;
    oci)
      fatal "This is an OCI container, use the regular container lifecycle management commands for your container tools instead of this script for managing it." F0203
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
        if [ -n "${NETDATA_CLAIM_TOKEN}" ]; then
          progress "Attempting to claim existing install at ${ndprefix}."
          INSTALL_PREFIX="${ndprefix}"
          claim
          ret=$?

          cleanup
          trap - EXIT
          exit $ret
        elif [ "${NETDATA_CLAIM_ONLY}" -eq 1 ]; then
          fatal "User asked to claim, but did not proide a claiming token." F0202
        else
          fatal "Found an existing netdata install at ${ndprefix}, but the install type is '${INSTALL_TYPE}', which is not supported, refusing to proceed." F0103
        fi
      fi
      ;;
  esac
}

soft_disable_cloud() {
  set_tmpdir

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
    fatal "The \`--install-prefix\` and \`--install\` options are only supported together with the \`--build-only\` option." F0204
  fi

  if [ -n "${INSTALL_PREFIX}" ]; then
    NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} --install-prefix ${INSTALL_PREFIX}"
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
    fatal "Invalid claiming options, claim rooms may only be specified when a token is specified." F0204
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
  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would attempt to claim agent to ${NETDATA_CLAIM_URL}"
  else
    progress "Attempting to claim agent to ${NETDATA_CLAIM_URL}"
  fi

  progress "Attempting to claim agent to ${NETDATA_CLAIM_URL}"
  if command -v netdata-claim.sh > /dev/null 2>&1; then
    NETDATA_CLAIM_PATH="$(command -v netdata-claim.sh)"
  elif [ -z "${INSTALL_PREFIX}" ] || [ "${INSTALL_PREFIX}" = "/" ]; then
    NETDATA_CLAIM_PATH=/usr/sbin/netdata-claim.sh
  elif [ "${INSTALL_PREFIX}" = "/opt/netdata" ]; then
    NETDATA_CLAIM_PATH="/opt/netdata/bin/netdata-claim.sh"
  elif [ ! -d "${INSTALL_PREFIX}/netdata" ]; then
    if [ -d "${INSTALL_PREFIX}/usr" ]; then
        NETDATA_CLAIM_PATH="${INSTALL_PREFIX}/usr/sbin/netdata-claim.sh"
    else
        NETDATA_CLAIM_PATH="${INSTALL_PREFIX}/sbin/netdata-claim.sh"
    fi
  else
    NETDATA_CLAIM_PATH="${INSTALL_PREFIX}/netdata/usr/sbin/netdata-claim.sh"
  fi

  if [ ! -x "${NETDATA_CLAIM_PATH}" ]; then
    fatal "Unable to find usable claiming script." F0106
  fi

  if ! is_netdata_running; then
    NETDATA_CLAIM_EXTRA="${NETDATA_CLAIM_EXTRA} -daemon-not-running"
  fi

  # shellcheck disable=SC2086
  run ${ROOTCMD} "${NETDATA_CLAIM_PATH}" -token="${NETDATA_CLAIM_TOKEN}" -rooms="${NETDATA_CLAIM_ROOMS}" -url="${NETDATA_CLAIM_URL}" ${NETDATA_CLAIM_EXTRA}
  case $? in
    0)
      progress "Successfully claimed node"
      return 0
      ;;
    1)
      warning "Unable to claim node due to invalid claiming options. If you are seeing this message, you’ve probably found a bug and should open a bug report at ${AGENT_BUG_REPORT_URL}"
      ;;
    2)
      warning "Unable to claim node due to issues creating the claiming directory or preparing the local claiming key. Make sure you have a working openssl command and that ${INSTALL_PREFIX}/var/lib/netdata/cloud.d exists, then try again."
      ;;
    3)
      warning "Unable to claim node due to missing dependencies. Usually this means that the Netdata Agent was built without support for Netdata Cloud. If you built the agent from source, please install all needed dependencies for Cloud support. If you used the regular installation script and see this error, please file a bug."
      ;;
    4)
      warning "Failed to claim node due to inability to connect to ${NETDATA_CLAIM_URL}. Usually this either means that the specified claiming URL is wrong, or that you are having networking problems."
      ;;
    5)
      progress "Successfully claimed node, but was not able to notify the Netdata Agent. You will need to restart the Netdata service on this node before it will show up in the Cloud."
      return 0
      ;;
    8)
      warning "Failed to claim node due to an invalid agent ID. You can usually resolve this by removing ${INSTALL_PREFIX}/var/lib/netdata/registry/netdata.public.unique.id and restarting the agent. Then try to claim it again using the same options."
      ;;
    9)
      warning "Failed to claim node due to an invalid node name. This probably means you tried to specify a custom name for this node (for example, using the --claim-hostname option), but the hostname itself was either empty or consisted solely of whitespace. You can resolve this by specifying a valid host name and trying again."
      ;;
    10)
      warning "Failed to claim node due to an invalid room ID. This issue is most likely caused by a typo.  Please check if the room(s) you are trying to add appear on the list of rooms provided to the --claim-rooms option ('${NETDATA_CLAIM_ROOMS}'). Then verify if the rooms are visible in Netdata Cloud and try again."
      ;;
    11)
      warning "Failed to claim node due to an issue with the generated RSA key pair. You can usually resolve this by removing all files in ${INSTALL_PREFIX}/var/lib/netdata/cloud.d and then trying again."
      ;;
    12)
      warning "Failed to claim node due to an invalid or expired claiming token. Please check that the token specified with the --claim-token option ('${NETDATA_CLAIM_TOKEN}') matches what you see in the Cloud and try again."
      ;;
    13)
      warning "Failed to claim node because the Cloud thinks it is already claimed. If this node was created by cloning a VM or as a container from a template, please remove the file ${INSTALL_PREFIX}/var/lib/netdata/registry/netdata.public.unique.id and restart the agent. Then try to claim it again with the same options. Otherwise, if you are certain this node has never been claimed before, you can use the --claim-id option to specify a new node ID to use for claiming, for example by using the uuidgen command like so: --claim-id \"\$(uuidgen)\""
      ;;
    14)
      warning "Failed to claim node because the node is already in the process of being claimed. You should not need to do anything to resolve this, the node should show up properly in the Cloud soon. If it does not, please report a bug at ${AGENT_BUG_REPORT_URL}."
      ;;
    15|16|17)
      warning "Failed to claim node due to an internal server error in the Cloud. Please retry claiming this node later, and if you still see this message file a bug report at ${CLOUD_BUG_REPORT_URL}."
      ;;
    18)
      warning "Unable to claim node because this Netdata installation does not have a unique ID yet. Make sure the agent is running and started up correctly, and then try again."
      ;;
    *)
      warning "Failed to claim node for an unknown reason. This usually means either networking problems or a bug. Please retry claiming later, and if you still see this message file a bug report at ${AGENT_BUG_REPORT_URL}"
      ;;
  esac

  if [ -z "${NETDATA_NEW_INSTALL}" ]; then
    deferred_warnings
    printf >&2 "%s\n" "For community support, you can connect with us on:"
    support_list
    cleanup
    trap - EXIT
    exit 1
  fi
}

# ======================================================================
# Auto-update handling code.
set_auto_updates() {
  if [ -x "${INSTALL_PREFIX}/usr/libexec/netdata/netdata-updater.sh" ]; then
    updater="${INSTALL_PREFIX}/usr/libexec/netdata/netdata-updater.sh"
  elif [ -x "${INSTALL_PREFIX}/netdata/usr/libexec/netdata/netdata-updater.sh" ]; then
    updater="${INSTALL_PREFIX}/netdata/usr/libexec/netdata/netdata-updater.sh"
  else
    warning "Could not find netdata-updater.sh. This means that auto-updates cannot (currently) be enabled on this system. See https://learn.netdata.cloud/docs/agent/packaging/installer/update for more information about updating Netdata."
    return 0
  fi

  if [ "${AUTO_UPDATE}" -eq 1 ]; then
    if [ "${DRY_RUN}" -eq 1 ]; then
      progress "Would have attempted to enable automatic updates."
    # This first case is for catching using a new kickstart script with an old build. It can be safely removed after v1.34.0 is released.
    elif ! grep -q '\-\-enable-auto-updates' ${updater}; then
      echo
    elif ! ${ROOTCMD} ${updater} --enable-auto-updates "${NETDATA_AUTO_UPDATE_TYPE}"; then
      warning "Failed to enable auto updates. Netdata will still work, but you will need to update manually."
    fi
  else
    if [ "${DRY_RUN}" -eq 1 ]; then
      progress "Would have attempted to disable automatic updates."
    else
      ${ROOTCMD} ${updater} --disable-auto-updates
    fi
  fi
}

# ======================================================================
# Native package install code.

# Check for an already installed package with a given name.
pkg_installed() {
  case "${DISTRO_COMPAT_NAME}" in
    debian|ubuntu)
      # shellcheck disable=SC2016
      dpkg-query --show --showformat '${Status}' "${1}" 2>&1 | cut -f 1 -d ' ' | grep -q '^install$'
      return $?
      ;;
    centos|fedora|opensuse|ol)
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
    centos|fedora|ol)
      # shellcheck disable=SC2086
      ${pm_cmd} search --nogpgcheck -v netdata | grep -qE 'Repo *: netdata(-edge)?$'
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
    if ${pm_cmd} search --nogpgcheck -v libuv | grep -q "No matches found"; then
      progress "libuv not found, checking for EPEL availability."
      if ${pm_cmd} search --nogpgcheck -v epel-release | grep -q "No matches found"; then
        warning "Unable to find a suitable source for libuv, cannot install using native packages on this system."
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
  if [ -z "${DISTRO}" ] || [ "${DISTRO}" = "unknown" ]; then
    warning "Unable to determine Linux distribution for native packages."
    return 2
  fi

  set_tmpdir

  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would attempt to install using native packages..."
  else
    progress "Attempting to install using native packages..."
  fi

  if [ "${SELECTED_RELEASE_CHANNEL}" = "nightly" ]; then
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
      NATIVE_VERSION="${INSTALL_VERSION:+"=${INSTALL_VERSION}"}"
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
      NATIVE_VERSION="${INSTALL_VERSION:+"=${INSTALL_VERSION}"}"
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
      NATIVE_VERSION="${INSTALL_VERSION:+"-${INSTALL_VERSION}.${SYSARCH}"}"
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
      NATIVE_VERSION="${INSTALL_VERSION:+"-${INSTALL_VERSION}.${SYSARCH}"}"
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
      NATIVE_VERSION="${INSTALL_VERSION:+"-${INSTALL_VERSION}.${SYSARCH}"}"
      ;;
    ol)
      if command -v dnf > /dev/null; then
        pm_cmd="dnf"
        repo_subcmd="makecache"
      else
        pm_cmd="yum"
      fi
      repo_prefix="ol/${SYSVERSION}"
      pkg_type="rpm"
      pkg_suffix=".noarch"
      pkg_vsep="-"
      pkg_install_opts="${interactive_opts}"
      repo_update_opts="${interactive_opts}"
      uninstall_subcmd="remove"
      INSTALL_TYPE="binpkg-rpm"
      NATIVE_VERSION="${INSTALL_VERSION:+"-${INSTALL_VERSION}.${SYSARCH}"}"
      ;;
    *)
      warning "We do not provide native packages for ${DISTRO}."
      return 2
      ;;
  esac

  if [ -n "${INSTALL_VERSION}" ]; then
    if echo "${INSTALL_VERSION}" | grep -q "nightly"; then
      new_release="-edge"
    else
      new_release=
    fi

    if { [ -n "${new_release}" ] && [ -z "${release}" ]; } || { [ -z "${new_release}" ] && [ -n "${release}" ]; }; then
      warning "Selected release channel does not match this version and it will be changed automatically."
    fi

    release="${new_release}"
  fi

  repoconfig_name="netdata-repo${release}"
  repoconfig_file="${repoconfig_name}${pkg_vsep}${REPOCONFIG_VERSION}${pkg_suffix}.${pkg_type}"
  repoconfig_url="${REPOCONFIG_URL_PREFIX}/${repo_prefix}/${repoconfig_file}/download.${pkg_type}"

  if ! pkg_installed "${repoconfig_name}"; then
    progress "Checking for availability of repository configuration package."
    if ! check_for_remote_file "${repoconfig_url}"; then
      warning "No repository configuration package available for ${DISTRO} ${SYSVERSION}."
      return 2
    fi

    if ! download "${repoconfig_url}" "${tmpdir}/${repoconfig_file}"; then
      fatal "Failed to download repository configuration package." F0209
    fi

    if [ -n "${needs_early_refresh}" ]; then
      # shellcheck disable=SC2086
      if ! run ${ROOTCMD} env ${env} ${pm_cmd} ${repo_subcmd} ${repo_update_opts}; then
        warning "Failed to refresh repository metadata."
        return 2
      fi
    fi

    # shellcheck disable=SC2086
    if ! run ${ROOTCMD} env ${env} ${pm_cmd} install ${pkg_install_opts} "${tmpdir}/${repoconfig_file}"; then
      warning "Failed to install repository configuration package."
      return 2
    fi

    if [ -n "${repo_subcmd}" ]; then
      # shellcheck disable=SC2086
      if ! run ${ROOTCMD} env ${env} ${pm_cmd} ${repo_subcmd} ${repo_update_opts}; then
        fatal "Failed to refresh repository metadata." F0205
      fi
    fi
  else
    progress "Repository configuration is already present, attempting to install netdata."
  fi

  if [ "${REPO_ACTION}" = "repositories-only" ]; then
    progress "Successfully installed repository configuraion package."
    deferred_warnings
    cleanup
    trap - EXIT
    exit 1
  fi

  if ! check_special_native_deps; then
    warning "Could not find secondary dependencies for ${DISTRO} on ${SYSARCH}."
    if [ -z "${NO_CLEANUP}" ]; then
      progress "Attempting to uninstall repository configuration package."
      # shellcheck disable=SC2086
      run ${ROOTCMD} env ${env} ${pm_cmd} ${uninstall_subcmd} ${pkg_install_opts} "${repoconfig_name}"
    fi
    return 2
  fi

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

  # shellcheck disable=SC2086
  if ! run ${ROOTCMD} env ${env} ${pm_cmd} install ${pkg_install_opts} "netdata${NATIVE_VERSION}"; then
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
# shellcheck disable=SC2034,SC2086,SC2126
set_static_archive_urls() {
  if [ -z "${2}" ]; then
    arch="${SYSARCH}"
  else
    arch="${2}"
  fi

  if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ]; then
    path="$(cd "${NETDATA_OFFLINE_INSTALL_SOURCE}" || exit 1; pwd)"
    export NETDATA_STATIC_ARCHIVE_URL="file://${path}/netdata-${arch}-latest.gz.run"
    export NETDATA_STATIC_ARCHIVE_CHECKSUM_URL="file://${path}/sha256sums.txt"
  elif [ "${1}" = "stable" ]; then
    if [ -n "${INSTALL_VERSION}" ]; then
      export NETDATA_STATIC_ARCHIVE_OLD_URL="https://github.com/netdata/netdata/releases/download/v${INSTALL_VERSION}/netdata-v${INSTALL_VERSION}.gz.run"
      export NETDATA_STATIC_ARCHIVE_URL="https://github.com/netdata/netdata/releases/download/v${INSTALL_VERSION}/netdata-${arch}-v${INSTALL_VERSION}.gz.run"
      export NETDATA_STATIC_ARCHIVE_CHECKSUM_URL="https://github.com/netdata/netdata/releases/download/v${INSTALL_VERSION}/sha256sums.txt"
    else
      latest="$(get_redirect "https://github.com/netdata/netdata/releases/latest")"
      export NETDATA_STATIC_ARCHIVE_URL="https://github.com/netdata/netdata/releases/download/${latest}/netdata-${arch}-latest.gz.run"
      export NETDATA_STATIC_ARCHIVE_CHECKSUM_URL="https://github.com/netdata/netdata/releases/download/${latest}/sha256sums.txt"
    fi
  else
    if [ -n "${INSTALL_VERSION}" ]; then
      export NETDATA_STATIC_ARCHIVE_URL="${NETDATA_TARBALL_BASEURL}/netdata-${arch}-v${INSTALL_VERSION}.gz.run"
      export NETDATA_STATIC_ARCHIVE_OLD_URL="${NETDATA_TARBALL_BASEURL}/netdata-v${INSTALL_VERSION}.gz.run"
      export NETDATA_STATIC_ARCHIVE_CHECKSUM_URL="${NETDATA_TARBALL_BASEURL}/sha256sums.txt"
    else
      export NETDATA_STATIC_ARCHIVE_URL="${NETDATA_TARBALL_BASEURL}/netdata-${arch}-latest.gz.run"
      export NETDATA_STATIC_ARCHIVE_CHECKSUM_URL="${NETDATA_TARBALL_BASEURL}/sha256sums.txt"
    fi
  fi
}

try_static_install() {
  set_static_archive_urls "${SELECTED_RELEASE_CHANNEL}"
  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would attempt to install using static build..."
  else
    progress "Attempting to install using static build..."
  fi

  # Check status code first, so that we can provide nicer fallback for dry runs.
  if check_for_remote_file "${NETDATA_STATIC_ARCHIVE_URL}"; then
    if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ]; then
      netdata_agent="$(basename "${NETDATA_STATIC_ARCHIVE_URL#"file://"}")"
    elif [ -n "${INSTALL_VERSION}" ]; then
      if [ "${SELECTED_RELEASE_CHANNEL}" = "stable" ]; then
        netdata_agent="${NETDATA_STATIC_ARCHIVE_URL#"https://github.com/netdata/netdata/releases/download/v${INSTALL_VERSION}/"}"
      else
        netdata_agent="${NETDATA_STATIC_ARCHIVE_URL#"${NETDATA_TARBALL_BASEURL}/"}"
      fi
    else
      if [ "${SELECTED_RELEASE_CHANNEL}" = "stable" ]; then
        netdata_agent="${NETDATA_STATIC_ARCHIVE_URL#"https://github.com/netdata/netdata/releases/download/${latest}/"}"
      else
        netdata_agent="${NETDATA_STATIC_ARCHIVE_URL#"${NETDATA_TARBALL_BASEURL}/"}"
      fi
    fi
  elif [ "${SYSARCH}" = "x86_64" ] && check_for_remote_file "${NETDATA_STATIC_ARCHIVE_OLD_URL}"; then
    netdata_agent="${NETDATA_STATIC_ARCHIVE_OLD_URL#"https://github.com/netdata/netdata/releases/download/v${INSTALL_VERSION}/"}"
    export NETDATA_STATIC_ARCHIVE_URL="${NETDATA_STATIC_ARCHIVE_OLD_URL}"
  else
    warning "No static build available for ${SYSARCH} CPUs."
    return 2
  fi

  if ! download "${NETDATA_STATIC_ARCHIVE_URL}" "${tmpdir}/${netdata_agent}"; then
    fatal "Unable to download static build archive for ${SYSARCH}." F0208
  fi

  if ! download "${NETDATA_STATIC_ARCHIVE_CHECKSUM_URL}" "${tmpdir}/sha256sum.txt"; then
    fatal "Unable to fetch checksums to verify static build archive." F0206
  fi

  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would validate SHA256 checksum of downloaded static build archive."
  else
    if [ -z "${INSTALL_VERSION}" ]; then
      if ! grep "${netdata_agent}" "${tmpdir}/sha256sum.txt" | safe_sha256sum -c - > /dev/null 2>&1; then
        fatal "Static binary checksum validation failed. Usually this is a result of an older copy of the file being cached somewhere upstream and can be resolved by retrying in an hour." F0207
      fi
    fi
  fi

  if [ "${INTERACTIVE}" -eq 0 ]; then
    opts="${opts} --accept"
  fi

  progress "Installing netdata"
  # shellcheck disable=SC2086
  if ! run ${ROOTCMD} sh "${tmpdir}/${netdata_agent}" ${opts} -- ${NETDATA_INSTALLER_OPTIONS}; then
    warning "Failed to install static build of Netdata on ${SYSARCH}."
    run rm -rf /opt/netdata
    return 2
  fi

  if [ "${DRY_RUN}" -ne 1 ]; then
  install_type_file="/opt/netdata/etc/netdata/.install-type"
    if [ -f "${install_type_file}" ]; then
      ${ROOTCMD} sh -c "cat \"${install_type_file}\" > \"${tmpdir}/install-type\""
      ${ROOTCMD} chown "$(id -u)":"$(id -g)" "${tmpdir}/install-type"
      # shellcheck disable=SC1090,SC1091
      . "${tmpdir}/install-type"
      cat > "${tmpdir}/install-type" <<- EOF
	INSTALL_TYPE='kickstart-static'
	PREBUILT_ARCH='${PREBUILT_ARCH}'
	EOF
      ${ROOTCMD} chown netdata:netdata "${tmpdir}/install-type"
      ${ROOTCMD} cp "${tmpdir}/install-type" "${install_type_file}"
    fi
  fi
}

# ======================================================================
# Local build install code

set_source_archive_urls() {
  if [ "$1" = "stable" ]; then
    if [ -n "${INSTALL_VERSION}" ]; then
      export NETDATA_SOURCE_ARCHIVE_URL="https://github.com/netdata/netdata/releases/download/v${INSTALL_VERSION}/netdata-v${INSTALL_VERSION}.tar.gz"
      export NETDATA_SOURCE_ARCHIVE_CHECKSUM_URL="https://github.com/netdata/netdata/releases/download/v${INSTALL_VERSION}/sha256sums.txt"
    else
      latest="$(get_redirect "https://github.com/netdata/netdata/releases/latest")"
      export NETDATA_SOURCE_ARCHIVE_URL="https://github.com/netdata/netdata/releases/download/${latest}/netdata-${latest}.tar.gz"
      export NETDATA_SOURCE_ARCHIVE_CHECKSUM_URL="https://github.com/netdata/netdata/releases/download/${latest}/sha256sums.txt"
    fi
  else
    if [ -n "${INSTALL_VERSION}" ]; then
      export NETDATA_SOURCE_ARCHIVE_URL="${NETDATA_TARBALL_BASEURL}/netdata-v${INSTALL_VERSION}.tar.gz"
      export NETDATA_SOURCE_ARCHIVE_CHECKSUM_URL="${NETDATA_TARBALL_BASEURL}/sha256sums.txt"
    else
      export NETDATA_SOURCE_ARCHIVE_URL="${NETDATA_TARBALL_BASEURL}/netdata-latest.tar.gz"
      export NETDATA_SOURCE_ARCHIVE_CHECKSUM_URL="${NETDATA_TARBALL_BASEURL}/sha256sums.txt"
    fi
  fi
}

install_local_build_dependencies() {
  set_tmpdir
  bash="$(command -v bash 2> /dev/null)"

  if [ -z "${bash}" ] || [ ! -x "${bash}" ]; then
    warning "Unable to find a usable version of \`bash\` (required for local build)."
    return 1
  fi

  if ! download "${PACKAGES_SCRIPT}" "${tmpdir}/install-required-packages.sh"; then
    fatal "Failed to download dependency handling script for local build." F000D
  fi

  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would run downloaded script to install required build dependencies..."
  else
    progress "Running downloaded script to install required build dependencies..."
  fi

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
    warning "Failed to install all required packages, but installation might still be possible."
  fi
}

build_and_install() {
  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would attempt to build netdata..."
  else
    progress "Building netdata..."
  fi

  echo "INSTALL_TYPE='kickstart-build'" > system/.install-type

  opts="${NETDATA_INSTALLER_OPTIONS}"

  if [ "${INTERACTIVE}" -eq 0 ]; then
    opts="${opts} --dont-wait"
  fi

  if [ "${SELECTED_RELEASE_CHANNEL}" = "stable" ]; then
    opts="${opts} --stable-channel"
  fi

  if [ "${NETDATA_REQUIRE_CLOUD}" -eq 1 ]; then
    opts="${opts} --require-cloud"
  elif [ "${NETDATA_DISABLE_CLOUD}" -eq 1 ]; then
    opts="${opts} --disable-cloud"
  fi

  export NETDATA_SAVE_WARNINGS=1
  export NETDATA_PROPAGATE_WARNINGS=1
  # shellcheck disable=SC2090
  export NETDATA_WARNINGS="${NETDATA_WARNINGS}"
  # shellcheck disable=SC2086
  run ${ROOTCMD} ./netdata-installer.sh ${opts}

  case $? in
    1)
      if [ -n "${EXIT_REASON}" ]; then
        fatal "netdata-installer.sh failed to run: ${EXIT_REASON}" "${EXIT_CODE}"
      else
        fatal "netdata-installer.sh failed to run correctly." I0000
      fi
      ;;
    2)
      fatal "Insufficient RAM to install netdata." F0008
      ;;
  esac
}

try_build_install() {
  set_tmpdir

  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would attempt to install by building locally..."
  else
    progress "Attempting to install by building locally..."
  fi

  if ! install_local_build_dependencies; then
    return 1
  fi

  set_source_archive_urls "${SELECTED_RELEASE_CHANNEL}"

  if [ -n "${INSTALL_VERSION}" ]; then
    if ! download "${NETDATA_SOURCE_ARCHIVE_URL}" "${tmpdir}/netdata-v${INSTALL_VERSION}.tar.gz"; then
      fatal "Failed to download source tarball for local build." F000B
    fi
  elif ! download "${NETDATA_SOURCE_ARCHIVE_URL}" "${tmpdir}/netdata-latest.tar.gz"; then
    fatal "Failed to download source tarball for local build." F000B
  fi

  if ! download "${NETDATA_SOURCE_ARCHIVE_CHECKSUM_URL}" "${tmpdir}/sha256sum.txt"; then
    fatal "Failed to download checksums for source tarball verification." F000C
  fi

  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would validate SHA256 checksum of downloaded source archive."
  else
    if [ -z "${INSTALL_VERSION}" ]; then
      # shellcheck disable=SC2086
      if ! grep netdata-latest.tar.gz "${tmpdir}/sha256sum.txt" | safe_sha256sum -c - > /dev/null 2>&1; then
        fatal "Tarball checksum validation failed. Usually this is a result of an older copy of the file being cached somewhere upstream and can be resolved by retrying in an hour." F0005
      fi
    fi
  fi

  if [ -n "${INSTALL_VERSION}" ]; then
    run tar -xf "${tmpdir}/netdata-v${INSTALL_VERSION}.tar.gz" -C "${tmpdir}"
    rm -rf "${tmpdir}/netdata-v${INSTALL_VERSION}.tar.gz" > /dev/null 2>&1
  else
    run tar -xf "${tmpdir}/netdata-latest.tar.gz" -C "${tmpdir}"
    rm -rf "${tmpdir}/netdata-latest.tar.gz" > /dev/null 2>&1
  fi

  if [ "${DRY_RUN}" -ne 1 ]; then
    cd "$(find "${tmpdir}" -mindepth 1 -maxdepth 1 -type d -name netdata-)" || fatal "Cannot change directory to netdata source tree" F0006
  fi

  if [ -x netdata-installer.sh ] || [ "${DRY_RUN}" -eq 1 ]; then
    build_and_install || return 1
  else
    # This case is needed because some platforms produce an extra directory on the source tarball extraction.
    if [ "$(find . -mindepth 1 -maxdepth 1 -type d | wc -l)" -eq 1 ] && [ -x "$(find . -mindepth 1 -maxdepth 1 -type d)/netdata-installer.sh" ]; then
      cd "$(find . -mindepth 1 -maxdepth 1 -type d)" &&  build_and_install || return 1
    else
      fatal "Cannot install netdata from source (the source directory does not include netdata-installer.sh)." F0009
    fi
  fi
}

# ======================================================================
# Offline install support code

prepare_offline_install_source() {
  if [ -e "${1}" ]; then
    if [ ! -d "${1}" ]; then
      fatal "${1} is not a directory, unable to prepare offline install source." F0503
    fi
  else
    run mkdir -p "${1}" || fatal "Unable to create target directory for offline install preparation." F0504
  fi

  run cd "${1}" || fatal "Failed to swtich to target directory for offline install preparation." F0505

  if [ "${NETDATA_ONLY_NATIVE}" -ne 1 ] && [ "${NETDATA_ONLY_BUILD}" -ne 1 ]; then
    set_static_archive_urls "${SELECTED_RELEASE_CHANNEL}" "x86_64"

    if check_for_remote_file "${NETDATA_STATIC_ARCHIVE_URL}"; then
      for arch in ${STATIC_INSTALL_ARCHES}; do
        set_static_archive_urls "${SELECTED_RELEASE_CHANNEL}" "${arch}"

        progress "Fetching ${NETDATA_STATIC_ARCHIVE_URL}"
        if ! download "${NETDATA_STATIC_ARCHIVE_URL}" "netdata-${arch}-latest.gz.run"; then
          warning "Failed to download static installer archive for ${arch}."
        fi
      done
      legacy=0
    else
      warning "Selected version of Netdata only provides static builds for x86_64. You will only be able to install on x86_64 systems with this offline install source."
      progress "Fetching ${NETDATA_STATIC_ARCHIVE_OLD_URL}"
      legacy=1

      if ! download "${NETDATA_STATIC_ARCHIVE_OLD_URL}" "netdata-x86_64-latest.gz.run"; then
        warning "Failed to download static installer archive for x86_64."
      fi
    fi

    progress "Fetching ${NETDATA_STATIC_ARCHIVE_CHECKSUM_URL}"
    if ! download "${NETDATA_STATIC_ARCHIVE_CHECKSUM_URL}" "sha256sums.txt"; then
      fatal "Failed to download checksum file." F0506
    fi
  fi

  if [ "${legacy:-0}" -eq 1 ]; then
    sed -e 's/netdata-latest.gz.run/netdata-x86_64-latest.gz.run' sha256sums.txt > sha256sums.tmp
    mv sha256sums.tmp sha256sums.txt
  fi

  if [ "${DRY_RUN}" -ne 1 ]; then
    progress "Verifying checksums."
    if ! grep -e "$(find . -name '*.gz.run')" sha256sums.txt | safe_sha256sum -c -; then
      fatal "Checksums for offline install files are incorrect. Usually this is a result of an older copy of the file being cached somewhere upstream and can be resolved by retrying in an hour." F0507
    fi
  else
    progress "Would verify SHA256 checksums of downloaded installation files."
  fi

  if [ "${DRY_RUN}" -ne 1 ]; then
    progress "Preparing install script."
    cat > "install.sh" <<-EOF
	#!/bin/sh
	dir=\$(CDPATH= cd -- "\$(dirname -- "\$0")" && pwd)
	"\${dir}/kickstart.sh" --offline-install-source "\${dir}" \${@}
	EOF
    chmod +x "install.sh"
  else
    progress "Would create install script"
  fi

  if [ "${DRY_RUN}" -ne 1 ]; then
    progress "Copying kickstart script."
    cp "${KICKSTART_SOURCE}" "kickstart.sh"
    chmod +x "kickstart.sh"
  else
    progress "Would copy kickstart.sh to offline install source directory"
  fi

  if [ "${DRY_RUN}" -ne 1 ]; then
    progress "Saving release channel information."
    echo "${SELECTED_RELEASE_CHANNEL}" > "channel"
  else
    progress "Would save release channel information to offline install source directory"
  fi

  progress "Finished preparing offline install source directory at ${1}. You can now copy this directory to a target system and then run the script ‘install.sh’ from it to install on that system."
}

# ======================================================================
# Per system-type install logic

install_on_linux() {
  if [ "${NETDATA_ONLY_STATIC}" -ne 1 ] && [ "${NETDATA_ONLY_BUILD}" -ne 1 ] && [ -z "${NETDATA_OFFLINE_INSTALL_SOURCE}" ]; then
    SELECTED_INSTALL_METHOD="native"
    try_package_install

    case "$?" in
      0)
        NETDATA_INSTALL_SUCCESSFUL=1
        INSTALL_PREFIX="/"
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
# Argument parsing code

validate_args() {
  check_claim_opts

  if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ]; then
    if [ "${NETDATA_ONLY_NATIVE}" -eq 1 ] || [ "${NETDATA_ONLY_BUILD}" -eq 1 ]; then
      fatal "Offline installs are only supported for static builds currently." F0502
    fi
  fi

  if [ -n "${LOCAL_BUILD_OPTIONS}" ]; then
    if [ "${NETDATA_ONLY_BUILD}" -eq 1 ]; then
      NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} ${LOCAL_BUILD_OPTIONS}"
    else
      fatal "Specifying local build options is only supported when the --build-only option is also specified." F0401
    fi
  fi

  if [ -n "${STATIC_INSTALL_OPTIONS}" ]; then
    if [ "${NETDATA_ONLY_STATIC}" -eq 1 ]; then
      NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} ${STATIC_INSTALL_OPTIONS}"
    else
      fatal "Specifying installer options options is only supported when the --static-only option is also specified." F0402
    fi
  fi

  if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ] && [ -n "${INSTALL_VERSION}" ]; then
      fatal "Specifying an install version alongside an offline install source is not supported." F050A
  fi

  if [ "${NETDATA_AUTO_UPDATES}" = "default" ]; then
    if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ] || [ -n "${INSTALL_VERSION}" ]; then
      AUTO_UPDATE=0
    else
      AUTO_UPDATE=1
    fi
  elif [ "${NETDATA_AUTO_UPDATES}" = 1 ]; then
    AUTO_UPDATE=1
  else
    AUTO_UPDATE=0
  fi

  if [ "${RELEASE_CHANNEL}" = "default" ]; then
    if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ]; then
      SELECTED_RELEASE_CHANNEL="$(cat "${NETDATA_OFFLINE_INSTALL_SOURCE}/channel")"

      if [ -z "${SELECTED_RELEASE_CHANNEL}" ]; then
        fatal "Could not find a release channel indicator in ${NETDATA_OFFLINE_INSTALL_SOURCE}." F0508
      fi
    else
      SELECTED_RELEASE_CHANNEL="${DEFAULT_RELEASE_CHANNEL}"
    fi
  else
    if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ] && [ "${RELEASE_CHANNEL}" != "$(cat "${NETDATA_OFFLINE_INSTALL_SOURCE}/channel")" ]; then
      fatal "Release channal '${RELEASE_CHANNEL}' requested, but indicated offline installation source release channel is '$(cat "${NETDATA_OFFLINE_INSTALL_SOURCE}/channel")'." F0509
    fi

    SELECTED_RELEASE_CHANNEL="${RELEASE_CHANNEL}"
  fi
}

parse_args() {
  if [ -n "${NETDATA_INSTALLER_OPTIONS}" ]; then
      warning "Explicitly specifying additional installer options with NETDATA_INSTALLER_OPTIONS is deprecated. Please instead pass the options to the script using either --local-build-options or --static-install-options as appropriate."
  fi

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
      "--dry-run") DRY_RUN=1 ;;
      "--stable-channel") RELEASE_CHANNEL="stable" ;;
      "--no-updates") NETDATA_AUTO_UPDATES=0 ;;
      "--auto-update") NETDATA_AUTO_UPDATES="1" ;;
      "--auto-update-method")
        NETDATA_AUTO_UPDATE_TYPE="$(echo "${2}" | tr '[:upper:]' '[:lower:]')"
        case "${NETDATA_AUTO_UPDATE_TYPE}" in
          systemd|interval|crontab)
            shift 1
            ;;
          *)
            echo "Unrecognized value for --auto-update-type. Valid values are: systemd, interval, crontab"
            exit 1
            ;;
        esac
        ;;
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
        warning "--install flag is deprecated and will be removed in a future version. Please use --install-prefix instead."
        INSTALL_PREFIX="${2}"
        shift 1
        ;;
      "--install-prefix")
        INSTALL_PREFIX="${2}"
        shift 1
        ;;
      "--old-install-prefix")
        OLD_INSTALL_PREFIX="${2}"
        shift 1
        ;;
      "--install-version")
        INSTALL_VERSION="${2}"
        AUTO_UPDATE=0
        shift 1
        ;;
      "--uninstall")
        ACTION="uninstall"
        ;;
      "--reinstall-clean")
        ACTION="reinstall-clean"
        ;;
      "--repositories-only")
        REPO_ACTION="repositories-only"
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
      "--claim-"*)
        optname="$(echo "${1}" | cut -d '-' -f 4-)"
        case "${optname}" in
          id|proxy|user|hostname)
            NETDATA_CLAIM_EXTRA="${NETDATA_CLAIM_EXTRA} -${optname}=${2}"
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
      "--local-build-options")
        LOCAL_BUILD_OPTIONS="${2}"
        shift 1
        ;;
      "--static-install-options")
        STATIC_INSTALL_OPTIONS="${2}"
        shift 1
        ;;
      "--prepare-offline-install-source")
        if [ -n "${2}" ]; then
          ACTION="prepare-offline"
          OFFLINE_TARGET="${2}"
          shift 1
        else
          fatal "A target directory must be specified with the --prepare-offline-install-source option." F0500
        fi
        ;;
      "--offline-install-source")
        if [ -d "${2}" ]; then
          NETDATA_OFFLINE_INSTALL_SOURCE="${2}"
          shift 1
        else
          fatal "A source directory must be specified with the --offline-install-source option." F0501
        fi
        ;;
      *)
        warning "Passing unrecognized option '${1}' to installer script. This behavior is deprecated and will be removed in the near future. If you intended to pass this option to the installer code, please use either --local-build-options or --static-install-options to specify it instead."
        NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} ${1}"
        ;;
    esac
    shift 1
  done

  validate_args
}

# ======================================================================
# Main program

setup_terminal || echo > /dev/null

# shellcheck disable=SC2068
parse_args $@

confirm_root_support
get_system_info
confirm_install_prefix

if [ -z "${ACTION}" ]; then
  handle_existing_install
fi

main
