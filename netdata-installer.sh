#!/bin/sh

# SPDX-License-Identifier: GPL-3.0-or-later

# Next unused error code: I0012

export PATH="${PATH}:/sbin:/usr/sbin:/usr/local/bin:/usr/local/sbin"
uniquepath() {
  path=""
  tmp="$(mktemp)"
  (echo  "${PATH}" | tr ":" "\n") > "$tmp"
  while read -r REPLY;
  do
    if echo "${path}" | grep -v "(^|:)${REPLY}(:|$)"; then
      [ -n "${path}" ] && path="${path}:"
      path="${path}${REPLY}"
    fi
  done < "$tmp"
rm "$tmp"
  [ -n "${path}" ]
export PATH="${path%:/sbin:/usr/sbin:/usr/local/bin:/usr/local/sbin}"
} > /dev/null
uniquepath

PROGRAM="$0"
NETDATA_SOURCE_DIR="$(pwd)"
INSTALLER_DIR="$(dirname "${PROGRAM}")"

if [ "${NETDATA_SOURCE_DIR}" != "${INSTALLER_DIR}" ] && [ "${INSTALLER_DIR}" != "." ]; then
  echo >&2 "Warning: you are currently in '${NETDATA_SOURCE_DIR}' but the installer is in '${INSTALLER_DIR}'."
fi

# -----------------------------------------------------------------------------
# reload the user profile

# shellcheck source=/dev/null
[ -f /etc/profile ] && . /etc/profile

# make sure /etc/profile does not change our current directory
cd "${NETDATA_SOURCE_DIR}" || exit 1

# -----------------------------------------------------------------------------
# load the required functions

if [ -f "${INSTALLER_DIR}/packaging/installer/functions.sh" ]; then
  # shellcheck source=packaging/installer/functions.sh
  . "${INSTALLER_DIR}/packaging/installer/functions.sh" || exit 1
else
  # shellcheck source=packaging/installer/functions.sh
  . "${NETDATA_SOURCE_DIR}/packaging/installer/functions.sh" || exit 1
fi

# Used to enable saved warnings support in functions.sh
# shellcheck disable=SC2034
NETDATA_SAVE_WARNINGS=1

# -----------------------------------------------------------------------------
# figure out an appropriate temporary directory
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

if [ -z "${TMPDIR}" ] || _cannot_use_tmpdir "${TMPDIR}"; then
  if _cannot_use_tmpdir /tmp; then
    if _cannot_use_tmpdir "${PWD}"; then
      fatal "Unable to find a usable temporary directory. Please set \$TMPDIR to a path that is both writable and allows execution of files and try again." I0000
    else
      TMPDIR="${PWD}"
    fi
  else
    TMPDIR="/tmp"
  fi
fi

# -----------------------------------------------------------------------------
# set up handling for deferred error messages
#
# This leverages the saved warnings functionality shared with some functions from functions.sh

print_deferred_errors() {
  if [ -n "${SAVED_WARNINGS}" ]; then
    printf >&2 "\n"
    printf >&2 "%b\n" "The following warnings and non-fatal errors were encountered during the installation process:"
    printf >&2 "%b\n" "${SAVED_WARNINGS}"
    printf >&2 "\n"
  fi
}

# make sure we save all commands we run
# Variable is used by code in the packaging/installer/functions.sh
# shellcheck disable=SC2034
run_logfile="netdata-installer.log"


# -----------------------------------------------------------------------------
# fix PKG_CHECK_MODULES error

if [ -d /usr/share/aclocal ]; then
  ACLOCAL_PATH=${ACLOCAL_PATH-/usr/share/aclocal}
  export ACLOCAL_PATH
fi

export LC_ALL=C
umask 002

# Be nice on production environments
renice 19 $$ > /dev/null 2> /dev/null

# you can set CFLAGS before running installer
# shellcheck disable=SC2269
LDFLAGS="${LDFLAGS}"
CFLAGS="${CFLAGS-"-O2 -pipe"}"
[ "z${CFLAGS}" = "z-O3" ] && CFLAGS="-O2"
# shellcheck disable=SC2269
ACLK="${ACLK}"

# keep a log of this command
{
  printf "\n# "
  date
  printf 'CFLAGS="%s" ' "${CFLAGS}"
  printf 'LDFLAGS="%s" ' "${LDFLAGS}"
  printf "%s" "${PROGRAM}" "${@}"
  printf "\n"
} >> netdata-installer.log

REINSTALL_OPTIONS="$(
  printf "%s" "${*}"
  printf "\n"
)"
# remove options that shown not be inherited by netdata-updater.sh
REINSTALL_OPTIONS="$(echo "${REINSTALL_OPTIONS}" | sed 's/--dont-wait//g' | sed 's/--dont-start-it//g')"

banner_nonroot_install() {
  cat << NONROOTNOPREFIX

  ${TPUT_RED}${TPUT_BOLD}Sorry! This will fail!${TPUT_RESET}

  You are attempting to install netdata as a non-root user, but you plan
  to install it in system paths.

  Please set an installation prefix, like this:

      $PROGRAM ${@} --install-prefix /tmp

  or, run the installer as root:

      sudo $PROGRAM ${@}

  We suggest to install it as root, or certain data collectors will
  not be able to work. Netdata drops root privileges when running.
  So, if you plan to keep it, install it as root to get the full
  functionality.

NONROOTNOPREFIX
}

banner_root_notify() {
  cat << NONROOT

  ${TPUT_RED}${TPUT_BOLD}IMPORTANT${TPUT_RESET}:
  You are about to install netdata as a non-root user.
  Netdata will work, but a few data collection modules that
  require root access will fail.

  If you are installing netdata permanently on your system, run
  the installer like this:

     ${TPUT_YELLOW}${TPUT_BOLD}sudo $PROGRAM ${@}${TPUT_RESET}

NONROOT
}

usage() {
  netdata_banner
  progress "installer command line options"
  cat << HEREDOC
USAGE: ${PROGRAM} [options]
       where options include:

  --install-prefix <path>    Install netdata in <path>. Ex. --install-prefix /opt will put netdata in /opt/netdata.
  --dont-start-it            Do not (re)start netdata after installation.
  --dont-wait                Run installation in non-interactive mode.
  --stable-channel           Use packages from GitHub release pages instead of nightly updates.
                             This results in less frequent updates.
  --nightly-channel          Use most recent nightly updates instead of GitHub releases.
                             This results in more frequent updates.
  --disable-ebpf             Disable eBPF Kernel plugin. Default: enabled.
  --force-legacy-cxx         Force usage of an older C++ standard to allow building on older systems. This will usually be autodetected.
  --enable-plugin-freeipmi   Enable the FreeIPMI plugin. Default: enable it when libipmimonitoring is available.
  --disable-plugin-freeipmi  Explicitly disable the FreeIPMI plugin.
  --disable-dbengine         Explicitly disable DB engine support.
  --enable-plugin-go         Enable the Go plugin. Default: Enabled when possible.
  --disable-plugin-go        Disable the Go plugin.
  --disable-go               Disable all Go components.
  --enable-plugin-nfacct     Enable nfacct plugin. Default: enable it when libmnl and libnetfilter_acct are available.
  --disable-plugin-nfacct    Explicitly disable the nfacct plugin.
  --enable-plugin-xenstat    Enable the xenstat plugin. Default: enable it when libxenstat and libyajl are available.
  --disable-plugin-xenstat   Explicitly disable the xenstat plugin.
  --enable-plugin-systemd-journal Enable the systemd journal plugin. Default: enable it when libsystemd is available.
  --disable-plugin-systemd-journal Explicitly disable the systemd journal plugin.
  --enable-exporting-kinesis Enable AWS Kinesis exporting connector. Default: enable it when libaws_cpp_sdk_kinesis
                             and its dependencies are available.
  --disable-exporting-kinesis Explicitly disable AWS Kinesis exporting connector.
  --enable-exporting-prometheus-remote-write Enable Prometheus remote write exporting connector. Default: enable it
                             when libprotobuf and libsnappy are available.
  --disable-exporting-prometheus-remote-write Explicitly disable Prometheus remote write exporting connector.
  --enable-exporting-mongodb Enable MongoDB exporting connector. Default: enable it when libmongoc is available.
  --disable-exporting-mongodb Explicitly disable MongoDB exporting connector.
  --enable-exporting-pubsub  Enable Google Cloud PubSub exporting connector. Default: enable it when
                             libgoogle_cloud_cpp_pubsub_protos and its dependencies are available.
  --disable-exporting-pubsub Explicitly disable Google Cloud PubSub exporting connector.
  --enable-lto               Enable link-time optimization. Default: disabled.
  --disable-lto              Explicitly disable link-time optimization.
  --enable-ml                Enable anomaly detection with machine learning. Default: autodetect.
  --disable-ml               Explicitly disable anomaly detection with machine learning.
  --disable-x86-sse          Disable SSE instructions & optimizations. Default: enabled.
  --use-system-protobuf      Use a system copy of libprotobuf instead of bundled copy. Default: bundled.
  --zlib-is-really-here
  --libs-are-really-here     If you see errors about missing zlib or libuuid but you know it is available, you might
                             have a broken pkg-config. Use this option to proceed without checking pkg-config.
  --disable-telemetry        Opt-out from our anonymous telemetry program. (DISABLE_TELEMETRY=1)
  --skip-available-ram-check Skip checking the amount of RAM the system has and pretend it has enough to build safely.
  --dev                      Do not remove the build directory - speeds up rebuilds
HEREDOC
}

if [ "$(uname -s)" = "Linux" ]; then
  case "$(uname -m)" in
    x86_64|i?86) ENABLE_EBPF=1 ;;
  esac
fi

DONOTSTART=0
DONOTWAIT=0
NETDATA_PREFIX=
LIBS_ARE_HERE=0
NETDATA_ENABLE_ML=""
ENABLE_DBENGINE=1
ENABLE_GO=1
ENABLE_PYTHON=1
ENABLE_CHARTS=1
FORCE_LEGACY_CXX=0
NETDATA_CMAKE_OPTIONS="${NETDATA_CMAKE_OPTIONS-}"
REMOVE_BUILD=1

RELEASE_CHANNEL="nightly" # valid values are 'nightly' and 'stable'
IS_NETDATA_STATIC_BINARY="${IS_NETDATA_STATIC_BINARY:-"no"}"
while [ -n "${1}" ]; do
  case "${1}" in
    "--zlib-is-really-here") LIBS_ARE_HERE=1 ;;
    "--libs-are-really-here") LIBS_ARE_HERE=1 ;;
    "--use-system-protobuf") USE_SYSTEM_PROTOBUF=1 ;;
    "--dont-scrub-cflags-even-though-it-may-break-things") DONT_SCRUB_CFLAGS_EVEN_THOUGH_IT_MAY_BREAK_THINGS=1 ;;
    "--dont-start-it") DONOTSTART=1 ;;
    "--dont-wait") DONOTWAIT=1 ;;
    "--auto-update" | "-u") ;;
    "--auto-update-type") ;;
    "--stable-channel") RELEASE_CHANNEL="stable" ;;
    "--nightly-channel") RELEASE_CHANNEL="nightly" ;;
    "--force-legacy-cxx") FORCE_LEGACY_CXX=1 ;;
    "--enable-plugin-freeipmi") ENABLE_FREEIPMI=1 ;;
    "--disable-plugin-freeipmi") ENABLE_FREEIPMI=0 ;;
    "--disable-https")
      warning "HTTPS cannot be disabled."
      ;;
    "--disable-dbengine") ENABLE_DBENGINE=0 ;;
    "--enable-plugin-go") ENABLE_GO=1 ;;
    "--disable-plugin-go") ENABLE_GO=0 ;;
    "--disable-go") ENABLE_GO=0 ;;
    "--enable-plugin-python") ENABLE_PYTHON=1 ;;
    "--disable-plugin-python") ENABLE_PYTHON=0 ;;
    "--enable-plugin-charts") ENABLE_CHARTS=1 ;;
    "--disable-plugin-charts") ENABLE_CHARTS=0 ;;
    "--enable-plugin-nfacct") ENABLE_NFACCT=1 ;;
    "--disable-plugin-nfacct") ENABLE_NFACCT=0 ;;
    "--enable-plugin-xenstat") ENABLE_XENSTAT=1 ;;
    "--disable-plugin-xenstat") ENABLE_XENSTAT=0 ;;
    "--enable-plugin-systemd-journal") ENABLE_SYSTEMD_JOURNAL=1 ;;
    "--disable-plugin-systemd-journal") ENABLE_SYSTEMD_JOURNAL=0 ;;
    "--enable-exporting-kinesis" | "--enable-backend-kinesis")
      # TODO: Needs CMake Support
      ;;
    "--disable-exporting-kinesis" | "--disable-backend-kinesis")
      # TODO: Needs CMake Support
      ;;
    "--enable-exporting-prometheus-remote-write" | "--enable-backend-prometheus-remote-write") EXPORTER_PROMETHEUS=1 ;;
    "--disable-exporting-prometheus-remote-write" | "--disable-backend-prometheus-remote-write") EXPORTER_PROMETHEUS=0 ;;
    "--enable-exporting-mongodb" | "--enable-backend-mongodb") EXPORTER_MONGODB=1 ;;
    "--disable-exporting-mongodb" | "--disable-backend-mongodb") EXPORTER_MONGODB=0 ;;
    "--enable-exporting-pubsub")
      # TODO: Needs CMake support
      ;;
    "--disable-exporting-pubsub")
      # TODO: Needs CMake support
      ;;
    "--enable-ml") NETDATA_ENABLE_ML=1 ;;
    "--disable-ml") NETDATA_ENABLE_ML=0 ;;
    "--enable-lto") NETDATA_ENABLE_LTO=1 ;;
    "--disable-lto") NETDATA_ENABLE_LTO=0 ;;
    "--disable-x86-sse")
      # XXX: No longer supported.
      ;;
    "--disable-telemetry") NETDATA_DISABLE_TELEMETRY=1 ;;
    "--enable-ebpf") ENABLE_EBPF=1 ;;
    "--disable-ebpf") ENABLE_EBPF=0 ;;
    "--skip-available-ram-check") SKIP_RAM_CHECK=1 ;;
    "--one-time-build")
      # XXX: No longer supported
      ;;
    "--disable-cloud")
      warning "Cloud cannot be disabled."
      ;;
    "--require-cloud") ;;
    "--build-json-c")
      NETDATA_BUILD_JSON_C=1
      ;;
    "--install-prefix")
      NETDATA_PREFIX="${2}/netdata"
      shift 1
      ;;
    "--install-no-prefix")
      NETDATA_PREFIX="${2}"
      shift 1
      ;;
    "--prepare-only")
      NETDATA_DISABLE_TELEMETRY=1
      NETDATA_PREPARE_ONLY=1
      DONOTWAIT=1
      ;;
    "--dev")
      REMOVE_BUILD=0
      ;;
    "--help" | "-h")
      usage
      exit 1
      ;;
    *)
      echo >&2 "Unrecognized option '${1}'."
      exit_reason "Unrecognized option '${1}'." I000E
      usage
      exit 1
      ;;
  esac
  shift 1
done

if [ ! "${DISABLE_TELEMETRY:-0}" -eq 0 ] ||
  [ -n "$DISABLE_TELEMETRY" ] ||
  [ ! "${DO_NOT_TRACK:-0}" -eq 0 ] ||
  [ -n "$DO_NOT_TRACK" ]; then
  NETDATA_DISABLE_TELEMETRY=1
fi

if [ -n "${MAKEOPTS}" ]; then
  JOBS="$(echo "${MAKEOPTS}" | grep -oE '\-j *[[:digit:]]+' | tr -d '\-j ')"
else
  JOBS="$(find_processors)"
fi

if [ "$(uname -s)" = "Linux" ] && [ -f /proc/meminfo ]; then
  mega="$((1024 * 1024))"
  base=1024
  scale=256

  target_ram="$((base * mega + (scale * mega * (JOBS - 1))))"
  total_ram="$(grep MemTotal /proc/meminfo | cut -d ':' -f 2 | tr -d ' kB')"
  total_ram="$((total_ram * 1024))"

  if [ "${total_ram}" -le "$((base * mega))" ] && [ -z "${NETDATA_ENABLE_ML}" ]; then
    NETDATA_ENABLE_ML=0
  fi

  if [ -z "${MAKEOPTS}" ]; then
    MAKEOPTS="-j${JOBS}"

    while [ "${target_ram}" -gt "${total_ram}" ] && [ "${JOBS}" -gt 1 ]; do
      JOBS="$((JOBS - 1))"
      target_ram="$((base * mega + (scale * mega * (JOBS - 1))))"
      MAKEOPTS="-j${JOBS}"
    done
  else
    if [ "${target_ram}" -gt "${total_ram}" ] && [ "${JOBS}" -gt 1 ] && [ -z "${SKIP_RAM_CHECK}" ]; then
      target_ram="$(echo "${target_ram}" | awk '{$1/=1024*1024*1024;printf "%.2fGiB\n",$1}')"
      total_ram="$(echo "${total_ram}" | awk '{$1/=1024*1024*1024;printf "%.2fGiB\n",$1}')"
      run_failed "Netdata needs ${target_ram} of RAM to safely install, but this system only has ${total_ram}. Try reducing the number of processes used for the install using the \$MAKEOPTS variable."
      exit_reason "Insufficient RAM to safely install." I000F
      exit 2
    fi
  fi
fi

# set default make options
if [ -z "${MAKEOPTS}" ]; then
  MAKEOPTS="-j$(find_processors)"
elif echo "${MAKEOPTS}" | grep -vqF -e "-j"; then
  MAKEOPTS="${MAKEOPTS} -j$(find_processors)"
fi

if [ "$(id -u)" -ne 0 ] && [ -z "${NETDATA_PREPARE_ONLY}" ]; then
  if [ -z "${NETDATA_PREFIX}" ]; then
    netdata_banner
    banner_nonroot_install "${@}"
    exit_reason "Attempted install as non-root user to /." I0010
    exit 1
  else
    banner_root_notify "${@}"
  fi
fi

netdata_banner
progress "Netdata, X-Ray Vision for your infrastructure!"
cat << BANNER1

  You are about to build and install netdata to your system.

  The build process will use ${TPUT_CYAN}${TMPDIR}${TPUT_RESET} for
  any temporary files. You can override this by setting \$TMPDIR to a
  writable directory where you can execute files.

  It will be installed at these locations:

   - the daemon     at ${TPUT_CYAN}${NETDATA_PREFIX}/usr/sbin/netdata${TPUT_RESET}
   - config files   in ${TPUT_CYAN}${NETDATA_PREFIX}/etc/netdata${TPUT_RESET}
   - web files      in ${TPUT_CYAN}${NETDATA_PREFIX}/usr/share/netdata${TPUT_RESET}
   - plugins        in ${TPUT_CYAN}${NETDATA_PREFIX}/usr/libexec/netdata${TPUT_RESET}
   - cache files    in ${TPUT_CYAN}${NETDATA_PREFIX}/var/cache/netdata${TPUT_RESET}
   - db files       in ${TPUT_CYAN}${NETDATA_PREFIX}/var/lib/netdata${TPUT_RESET}
   - log files      in ${TPUT_CYAN}${NETDATA_PREFIX}/var/log/netdata${TPUT_RESET}
BANNER1

[ "$(id -u)" -eq 0 ] && cat << BANNER2
   - pid file       at ${TPUT_CYAN}${NETDATA_PREFIX}/var/run/netdata.pid${TPUT_RESET}
   - logrotate file at ${TPUT_CYAN}/etc/logrotate.d/netdata${TPUT_RESET}
BANNER2

cat << BANNER3

  This installer allows you to change the installation path.
  Press Control-C and run the same command with --help for help.

BANNER3

if [ -z "$NETDATA_DISABLE_TELEMETRY" ]; then
  cat << BANNER4

  ${TPUT_YELLOW}${TPUT_BOLD}NOTE${TPUT_RESET}:
  Anonymous usage stats will be collected and sent to Netdata.
  To opt-out, pass --disable-telemetry option to the installer or export
  the environment variable DISABLE_TELEMETRY to a non-zero or non-empty value
  (e.g: export DISABLE_TELEMETRY=1).

BANNER4
fi

if ! command -v cmake >/dev/null 2>&1; then
    fatal "Could not find CMake, which is required to build Netdata." I0012
else
    cmake="$(command -v cmake)"
    progress "Found CMake at ${cmake}. CMake version: $(${cmake} --version | head -n 1)"
fi

if ! command -v "ninja" >/dev/null 2>&1; then
    progress "Could not find Ninja, will use Make instead."
else
    ninja="$(command -v ninja)"
    progress "Found Ninja at ${ninja}. Ninja version: $(${ninja} --version)"
    progress "Will use Ninja for this build instead of Make when possible."
fi

make="$(command -v make 2>/dev/null)"

if [ -z "${make}" ] && [ -z "${ninja}" ]; then
    fatal "Could not find a usable underlying build system (we support make and ninja)." I0014
fi

CMAKE_OPTS="${ninja:+-G Ninja}"
BUILD_OPTS="VERBOSE=1"
[ -n "${ninja}" ] && BUILD_OPTS="-k 1"

if [ ${DONOTWAIT} -eq 0 ]; then
  if [ -n "${NETDATA_PREFIX}" ]; then
    printf '%s' "${TPUT_BOLD}${TPUT_GREEN}Press ENTER to build and install netdata to '${TPUT_CYAN}${NETDATA_PREFIX}${TPUT_YELLOW}'${TPUT_RESET} > "
  else
    printf '%s' "${TPUT_BOLD}${TPUT_GREEN}Press ENTER to build and install netdata to your system${TPUT_RESET} > "
  fi
  read -r REPLY
  if [ "$REPLY" != '' ]; then
    exit_reason "User did not accept install attempt." I0011
    exit 1
  fi

fi

cmake_install() {
    # run cmake --install ${1}
    # The above command should be used to replace the logic below once we no longer support
    # versions of CMake less than 3.15.
    if [ -n "${ninja}" ]; then
        run ${ninja} -C "${1}" install
    else
        run ${make} -C "${1}" install
    fi
}

build_error() {
  netdata_banner
  trap - EXIT
  fatal "Netdata failed to build for an unknown reason." I0002
}

if [ ${LIBS_ARE_HERE} -eq 1 ]; then
  shift
  echo >&2 "ok, assuming libs are really installed."
  export ZLIB_CFLAGS=" "
  export ZLIB_LIBS="-lz"
  export UUID_CFLAGS=" "
  export UUID_LIBS="-luuid"
fi

trap build_error EXIT

# -----------------------------------------------------------------------------
# If we’re installing the Go plugin, ensure a working Go toolchain is installed.
if [ "${ENABLE_GO}" -eq 1 ]; then
  progress "Checking for a usable Go toolchain and attempting to install one to /usr/local/go if needed."
  . "${NETDATA_SOURCE_DIR}/packaging/check-for-go-toolchain.sh"

  if ! ensure_go_toolchain; then
    warning "Go ${GOLANG_MIN_VERSION} needed to build Go plugin, but could not find or install a usable toolchain: ${GOLANG_FAILURE_REASON}"
    ENABLE_GO=0
  fi
fi

# -----------------------------------------------------------------------------
# If we have the dashboard switching logic, make sure we're on the classic
# dashboard during the install (updates don't work correctly otherwise).
if [ -x "${NETDATA_PREFIX}/usr/libexec/netdata-switch-dashboard.sh" ]; then
  "${NETDATA_PREFIX}/usr/libexec/netdata-switch-dashboard.sh" classic
fi

# -----------------------------------------------------------------------------
# By default, `git` does not update local tags based on remotes. Because
# we use the most recent tag as part of our version determination in
# our build, this can lead to strange versions that look ancient but are
# actually really recent. To avoid this, try and fetch tags if we're
# working in a git checkout.
if [ -d ./.git ] ; then
  echo >&2
  progress "Updating tags in git to ensure a consistent version number"
  run git fetch -t || true
fi

# -----------------------------------------------------------------------------

echo >&2

[ -n "${GITHUB_ACTIONS}" ] && echo "::group::Configuring Netdata."
NETDATA_BUILD_DIR="${NETDATA_BUILD_DIR:-./build/}"
[ ${REMOVE_BUILD} -eq 1 ] && rm -rf "${NETDATA_BUILD_DIR}"

# function to extract values from the config file
config_option() {
  section="${1}"
  key="${2}"
  value="${3}"

  if [ -x "${NETDATA_PREFIX}/usr/sbin/netdata" ] && [ -r "${NETDATA_PREFIX}/etc/netdata/netdata.conf" ]; then
    "${NETDATA_PREFIX}/usr/sbin/netdata" \
      -c "${NETDATA_PREFIX}/etc/netdata/netdata.conf" \
      -W get "${section}" "${key}" "${value}" ||
      echo "${value}"
  else
    echo "${value}"
  fi
}

# the user netdata will run as
if [ "$(id -u)" = "0" ]; then
  NETDATA_USER="$(config_option "global" "run as user" "netdata")"
  ROOT_USER="root"
else
  NETDATA_USER="${USER}"
  ROOT_USER="${USER}"
fi
NETDATA_GROUP="$(id -g -n "${NETDATA_USER}" 2> /dev/null)"
[ -z "${NETDATA_GROUP}" ] && NETDATA_GROUP="${NETDATA_USER}"
echo >&2 "Netdata user and group set to: ${NETDATA_USER}/${NETDATA_GROUP}"

prepare_cmake_options

if [ -n "${NETDATA_PREPARE_ONLY}" ]; then
    progress "Exiting before building Netdata as requested."
    printf "Would have used the following CMake command line for configuration: %s\n" "${cmake} ${NETDATA_CMAKE_OPTIONS}"
    trap - EXIT
    exit 0
fi

# Let cmake know we don't want to link shared libs
if [ "${IS_NETDATA_STATIC_BINARY}" = "yes" ]; then
    NETDATA_CMAKE_OPTIONS="${NETDATA_CMAKE_OPTIONS} -DBUILD_SHARED_LIBS=Off"
fi

# shellcheck disable=SC2086
if ! run ${cmake} ${NETDATA_CMAKE_OPTIONS}; then
  fatal "Failed to configure Netdata sources." I000A
fi

[ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"

# remove the build_error hook
trap - EXIT

# -----------------------------------------------------------------------------
[ -n "${GITHUB_ACTIONS}" ] && echo "::group::Building Netdata."

# -----------------------------------------------------------------------------
progress "Compile netdata"

# shellcheck disable=SC2086
if ! run ${cmake} --build "${NETDATA_BUILD_DIR}" --parallel ${JOBS} -- ${BUILD_OPTS}; then
  fatal "Failed to build Netdata." I000B
fi

[ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"

# -----------------------------------------------------------------------------
[ -n "${GITHUB_ACTIONS}" ] && echo "::group::Installing Netdata."

# -----------------------------------------------------------------------------
progress "Install netdata"

if ! cmake_install "${NETDATA_BUILD_DIR}"; then
  fatal "Failed to install Netdata." I000C
fi

# -----------------------------------------------------------------------------
progress "Creating standard user and groups for netdata"

NETDATA_WANTED_GROUPS="docker nginx varnish haproxy adm nsd proxy squid ceph nobody"
NETDATA_ADDED_TO_GROUPS=""
if [ "$(id -u)" -eq 0 ]; then
  progress "Adding group 'netdata'"
  portable_add_group netdata || :

  progress "Adding user 'netdata'"
  portable_add_user netdata "${NETDATA_PREFIX}/var/lib/netdata" || :

  progress "Assign user 'netdata' to required groups"
  for g in ${NETDATA_WANTED_GROUPS}; do
    # shellcheck disable=SC2086
    portable_add_user_to_group ${g} netdata && NETDATA_ADDED_TO_GROUPS="${NETDATA_ADDED_TO_GROUPS} ${g}"
  done
  # Netdata must be able to read /etc/pve/qemu-server/* and /etc/pve/lxc/*
  # for reading VMs/containers names, CPU and memory limits on Proxmox.
  if [ -d "/etc/pve" ]; then
    portable_add_user_to_group "www-data" netdata && NETDATA_ADDED_TO_GROUPS="${NETDATA_ADDED_TO_GROUPS} www-data"
  fi
else
  run_failed "The installer does not run as root. Nothing to do for user and groups"
fi

# -----------------------------------------------------------------------------
progress "Install logrotate configuration for netdata"

install_netdata_logrotate

progress "Install journald configuration for netdata"

install_netdata_journald_conf

# -----------------------------------------------------------------------------
progress "Read installation options from netdata.conf"

# create an empty config if it does not exist
[ ! -f "${NETDATA_PREFIX}/etc/netdata/netdata.conf" ] &&
  touch "${NETDATA_PREFIX}/etc/netdata/netdata.conf"

# port
defport=19999
NETDATA_PORT="$(config_option "web" "default port" ${defport})"

# directories
NETDATA_LIB_DIR="$(config_option "global" "lib directory" "${NETDATA_PREFIX}/var/lib/netdata")"
NETDATA_CACHE_DIR="$(config_option "global" "cache directory" "${NETDATA_PREFIX}/var/cache/netdata")"
NETDATA_WEB_DIR="$(config_option "global" "web files directory" "${NETDATA_PREFIX}/usr/share/netdata/web")"
NETDATA_LOG_DIR="$(config_option "global" "log directory" "${NETDATA_PREFIX}/var/log/netdata")"
NETDATA_USER_CONFIG_DIR="$(config_option "global" "config directory" "${NETDATA_PREFIX}/etc/netdata")"
NETDATA_STOCK_CONFIG_DIR="$(config_option "global" "stock config directory" "${NETDATA_PREFIX}/usr/lib/netdata/conf.d")"
NETDATA_RUN_DIR="${NETDATA_PREFIX}/var/run"
NETDATA_CLAIMING_DIR="${NETDATA_LIB_DIR}/cloud.d"

cat << OPTIONSEOF

    Permissions
    - netdata user             : ${NETDATA_USER}
    - netdata group            : ${NETDATA_GROUP}
    - root user                : ${ROOT_USER}

    Directories
    - netdata user config dir  : ${NETDATA_USER_CONFIG_DIR}
    - netdata stock config dir : ${NETDATA_STOCK_CONFIG_DIR}
    - netdata log dir          : ${NETDATA_LOG_DIR}
    - netdata run dir          : ${NETDATA_RUN_DIR}
    - netdata lib dir          : ${NETDATA_LIB_DIR}
    - netdata web dir          : ${NETDATA_WEB_DIR}
    - netdata cache dir        : ${NETDATA_CACHE_DIR}

    Other
    - netdata port             : ${NETDATA_PORT}

OPTIONSEOF

# -----------------------------------------------------------------------------
progress "Fix permissions of netdata directories (using user '${NETDATA_USER}')"

if [ ! -d "${NETDATA_RUN_DIR}" ]; then
  # this is needed if NETDATA_PREFIX is not empty
  if ! run mkdir -p "${NETDATA_RUN_DIR}"; then
    warning "Failed to create ${NETDATA_RUN_DIR}, it must becreated by hand or the Netdata Agent will not be able to be started."
  fi
fi

# --- stock conf dir ----

[ ! -d "${NETDATA_STOCK_CONFIG_DIR}" ] && mkdir -p "${NETDATA_STOCK_CONFIG_DIR}"
[ -L "${NETDATA_USER_CONFIG_DIR}/orig" ] && run rm -f "${NETDATA_USER_CONFIG_DIR}/orig"
run ln -s "${NETDATA_STOCK_CONFIG_DIR}" "${NETDATA_USER_CONFIG_DIR}/orig"

# --- web dir ----

if [ ! -d "${NETDATA_WEB_DIR}" ]; then
  echo >&2 "Creating directory '${NETDATA_WEB_DIR}'"
  run mkdir -p "${NETDATA_WEB_DIR}" || exit 1
fi
run find "${NETDATA_WEB_DIR}" -type f -exec chmod 0664 {} \;
run find "${NETDATA_WEB_DIR}" -type d -exec chmod 0775 {} \;

# --- data dirs ----

for x in "${NETDATA_LIB_DIR}" "${NETDATA_CACHE_DIR}" "${NETDATA_LOG_DIR}"; do
  if [ ! -d "${x}" ]; then
    echo >&2 "Creating directory '${x}'"
    if ! run mkdir -p "${x}"; then
      warning "Failed to create ${x}, it must be created by hand or the Netdata Agent will not be able to be started."
    fi
  fi

  run chown -R "${NETDATA_USER}:${NETDATA_GROUP}" "${x}"
  #run find "${x}" -type f -exec chmod 0660 {} \;
  #run find "${x}" -type d -exec chmod 0770 {} \;
done

run chmod 755 "${NETDATA_LOG_DIR}"

# --- claiming dir ----

if [ ! -d "${NETDATA_CLAIMING_DIR}" ]; then
  echo >&2 "Creating directory '${NETDATA_CLAIMING_DIR}'"
  if ! run mkdir -p "${NETDATA_CLAIMING_DIR}"; then
    warning "failed to create ${NETDATA_CLAIMING_DIR}, it will need to be created manually."
  fi
fi
run chown -R "${NETDATA_USER}:${NETDATA_GROUP}" "${NETDATA_CLAIMING_DIR}"
run chmod 770 "${NETDATA_CLAIMING_DIR}"

# --- plugins ----

if [ "$(id -u)" -eq 0 ]; then
  # find the admin group
  admin_group=
  test -z "${admin_group}" && get_group root > /dev/null 2>&1 && admin_group="root"
  test -z "${admin_group}" && get_group daemon > /dev/null 2>&1 && admin_group="daemon"
  test -z "${admin_group}" && admin_group="${NETDATA_GROUP}"

  run chown "${NETDATA_USER}:${admin_group}" "${NETDATA_LOG_DIR}"
  run chown -R "root:${admin_group}" "${NETDATA_PREFIX}/usr/libexec/netdata"
  run find "${NETDATA_PREFIX}/usr/libexec/netdata" -type d -exec chmod 0755 {} \;
  run find "${NETDATA_PREFIX}/usr/libexec/netdata" -type f -exec chmod 0644 {} \;
  # shellcheck disable=SC2086
  run find "${NETDATA_PREFIX}/usr/libexec/netdata" -type f -a -name \*.plugin -exec chown :${NETDATA_GROUP} {} \;
  run find "${NETDATA_PREFIX}/usr/libexec/netdata" -type f -a -name \*.plugin -exec chmod 0750 {} \;
  run find "${NETDATA_PREFIX}/usr/libexec/netdata" -type f -a -name \*.sh -exec chmod 0755 {} \;

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"
    capabilities=0
    if ! iscontainer && command -v setcap 1> /dev/null 2>&1; then
      run chmod 0750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"
      if run setcap cap_dac_read_search,cap_sys_ptrace+ep "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"; then
        # if we managed to setcap, but we fail to execute apps.plugin setuid to root
        "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin" -t > /dev/null 2>&1 && capabilities=1 || capabilities=0
      fi
    fi

    if [ $capabilities -eq 0 ]; then
      # fix apps.plugin to be setuid to root
      run chmod 4750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"
    fi
  fi

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/debugfs.plugin" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/debugfs.plugin"
    capabilities=0
    if ! iscontainer && command -v setcap 1> /dev/null 2>&1; then
      run chmod 0750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/debugfs.plugin"
      if run setcap cap_dac_read_search+ep "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/debugfs.plugin"; then
        # if we managed to setcap, but we fail to execute debugfs.plugin setuid to root
        "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/debugfs.plugin" -t > /dev/null 2>&1 && capabilities=1 || capabilities=0
      fi
    fi

    if [ $capabilities -eq 0 ]; then
      # fix debugfs.plugin to be setuid to root
      run chmod 4750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/debugfs.plugin"
    fi
  fi

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/systemd-journal.plugin" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/systemd-journal.plugin"
    capabilities=0
    if ! iscontainer && command -v setcap 1> /dev/null 2>&1; then
      run chmod 0750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/systemd-journal.plugin"
      if run setcap cap_dac_read_search+ep "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/systemd-journal.plugin"; then
        capabilities=1
      fi
    fi

    if [ $capabilities -eq 0 ]; then
      run chmod 4750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/systemd-journal.plugin"
    fi
  fi

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/perf.plugin" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/perf.plugin"
    capabilities=0
    if ! iscontainer && command -v setcap 1>/dev/null 2>&1; then
      run chmod 0750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/perf.plugin"
      if run sh -c "setcap cap_perfmon+ep \"${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/perf.plugin\" || setcap cap_sys_admin+ep \"${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/perf.plugin\""; then
        capabilities=1
      fi
    fi

    if [ $capabilities -eq 0 ]; then
      run chmod 4750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/perf.plugin"
    fi
  fi

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/slabinfo.plugin" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/slabinfo.plugin"
    capabilities=0
    if ! iscontainer && command -v setcap 1>/dev/null 2>&1; then
      run chmod 0750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/slabinfo.plugin"
      if run setcap cap_dac_read_search+ep "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/slabinfo.plugin"; then
        capabilities=1
      fi
    fi

    if [ $capabilities -eq 0 ]; then
      run chmod 4750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/slabinfo.plugin"
    fi
  fi

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/freeipmi.plugin" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/freeipmi.plugin"
    run chmod 4750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/freeipmi.plugin"
  fi

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/nfacct.plugin" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/nfacct.plugin"
    run chmod 4750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/nfacct.plugin"
  fi

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/xenstat.plugin" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/xenstat.plugin"
    run chmod 4750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/xenstat.plugin"
  fi

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/ioping" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/ioping"
    run chmod 4750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/ioping"
  fi

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/ebpf.plugin" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/ebpf.plugin"
    run chmod 4750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/ebpf.plugin"
  fi

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/cgroup-network" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/cgroup-network"
    run chmod 4750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/cgroup-network"
  fi

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/cgroup-network-helper.sh" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/cgroup-network-helper.sh"
    run chmod 0750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/cgroup-network-helper.sh"
  fi

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/local-listeners" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/local-listeners"
    run chmod 4750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/local-listeners"
  fi

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/network-viewer.plugin" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/network-viewer.plugin"
    run chmod 4750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/network-viewer.plugin"
  fi

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/ndsudo" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/ndsudo"
    run chmod 4750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/ndsudo"
  fi

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/go.d.plugin" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/go.d.plugin"
    capabilities=0
    if ! iscontainer && command -v setcap 1> /dev/null 2>&1; then
      run chmod 0750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/go.d.plugin"
      if run setcap "cap_dac_read_search+epi cap_net_admin+epi cap_net_raw=eip" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/go.d.plugin"; then
        capabilities=1
      fi
    fi

    if [ $capabilities -eq 0 ]; then
      # fix go.d.plugin to be setuid to root
      run chmod 4750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/go.d.plugin"
    fi
  fi

else
  # non-privileged user installation
  run chown "${NETDATA_USER}:${NETDATA_GROUP}" "${NETDATA_LOG_DIR}"
  run chown -R "${NETDATA_USER}:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata"
  run find "${NETDATA_PREFIX}/usr/libexec/netdata" -type f -exec chmod 0755 {} \;
  run find "${NETDATA_PREFIX}/usr/libexec/netdata" -type d -exec chmod 0755 {} \;
fi

[ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"

# -----------------------------------------------------------------------------
progress "Telemetry configuration"

# Opt-out from telemetry program
if [ -n "${NETDATA_DISABLE_TELEMETRY+x}" ]; then
  run touch "${NETDATA_USER_CONFIG_DIR}/.opt-out-from-anonymous-statistics"
else
  printf "You can opt out from anonymous statistics via the --disable-telemetry option, or by creating an empty file %s \n\n" "${NETDATA_USER_CONFIG_DIR}/.opt-out-from-anonymous-statistics"
fi

# -----------------------------------------------------------------------------
progress "Install netdata at system init"

# By default we assume the shutdown/startup of the Netdata Agent are effectively
# without any system supervisor/init like SystemD or SysV. So we assume the most
# basic startup/shutdown commands...
NETDATA_STOP_CMD="${NETDATA_PREFIX}/usr/sbin/netdatacli shutdown-agent"
NETDATA_START_CMD="${NETDATA_PREFIX}/usr/sbin/netdata"

if grep -q docker /proc/1/cgroup > /dev/null 2>&1; then
  # If docker runs systemd for some weird reason, let the install proceed
  is_systemd_running="NO"
  if command -v pidof > /dev/null 2>&1; then
    is_systemd_running="$(pidof /usr/sbin/init || pidof systemd || echo "NO")"
  else
    is_systemd_running="$( (pgrep -q -f systemd && echo "1") || echo "NO")"
  fi

  if [ "${is_systemd_running}" = "1" ]; then
    echo >&2 "Found systemd within the docker container, running install_netdata_service() method"
    install_netdata_service || run_failed "Cannot install netdata init service."
  else
    echo >&2 "We are running within a docker container, will not be installing netdata service"
  fi
  echo >&2
else
  install_netdata_service || run_failed "Cannot install netdata init service."
fi

# -----------------------------------------------------------------------------
# check if we can re-start netdata

# TODO: Creation of configuration file should be handled by a build system. Additionally we shouldn't touch configuration files in /etc/netdata/...
started=0
if [ ${DONOTSTART} -eq 1 ]; then
  create_netdata_conf "${NETDATA_PREFIX}/etc/netdata/netdata.conf"
else
  if ! restart_netdata "${NETDATA_PREFIX}/usr/sbin/netdata" "${@}"; then
    fatal "Cannot start netdata!" I000D
  fi

  started=1
  run_ok "netdata started!"
  create_netdata_conf "${NETDATA_PREFIX}/etc/netdata/netdata.conf" "http://localhost:${NETDATA_PORT}/netdata.conf"
fi
run chmod 0644 "${NETDATA_PREFIX}/etc/netdata/netdata.conf"

if [ "$(uname)" = "Linux" ]; then
  # -------------------------------------------------------------------------
  progress "Check KSM (kernel memory deduper)"

  ksm_is_available_but_disabled() {
    cat << KSM1

${TPUT_BOLD}Memory de-duplication instructions${TPUT_RESET}

You have kernel memory de-duper (called Kernel Same-page Merging,
or KSM) available, but it is not currently enabled.

To enable it run:

    ${TPUT_YELLOW}${TPUT_BOLD}echo 1 >/sys/kernel/mm/ksm/run${TPUT_RESET}
    ${TPUT_YELLOW}${TPUT_BOLD}echo 1000 >/sys/kernel/mm/ksm/sleep_millisecs${TPUT_RESET}

If you enable it, you will save 40-60% of netdata memory.

KSM1
  }

  ksm_is_not_available() {
    cat << KSM2

${TPUT_BOLD}Memory de-duplication not present in your kernel${TPUT_RESET}

It seems you do not have kernel memory de-duper (called Kernel Same-page
Merging, or KSM) available.

To enable it, you need a kernel built with CONFIG_KSM=y

If you can have it, you will save 40-60% of netdata memory.

KSM2
  }

  if [ -f "/sys/kernel/mm/ksm/run" ]; then
    if [ "$(cat "/sys/kernel/mm/ksm/run")" != "1" ]; then
      ksm_is_available_but_disabled
    fi
  else
    ksm_is_not_available
  fi
fi

if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin" ]; then
  # -----------------------------------------------------------------------------
  progress "Check apps.plugin"

  if [ "$(id -u)" -ne 0 ]; then
    cat << SETUID_WARNING

${TPUT_BOLD}apps.plugin needs privileges${TPUT_RESET}

Since you have installed netdata as a normal user, to have apps.plugin collect
all the needed data, you have to give it the access rights it needs, by running
either of the following sets of commands:

To run apps.plugin with escalated capabilities:

    ${TPUT_YELLOW}${TPUT_BOLD}sudo chown root:${NETDATA_GROUP} "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"${TPUT_RESET}
    ${TPUT_YELLOW}${TPUT_BOLD}sudo chmod 0750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"${TPUT_RESET}
    ${TPUT_YELLOW}${TPUT_BOLD}sudo setcap cap_dac_read_search,cap_sys_ptrace+ep "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"${TPUT_RESET}

or, to run apps.plugin as root:

    ${TPUT_YELLOW}${TPUT_BOLD}sudo chown root:${NETDATA_GROUP} "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"${TPUT_RESET}
    ${TPUT_YELLOW}${TPUT_BOLD}sudo chmod 4750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/apps.plugin"${TPUT_RESET}

apps.plugin is performing a hard-coded function of data collection for all
running processes. It cannot be instructed from the netdata daemon to perform
any task, so it is pretty safe to do this.

SETUID_WARNING
  fi
fi

# -----------------------------------------------------------------------------
progress "Copy uninstaller"
if [ -f "${NETDATA_PREFIX}"/usr/libexec/netdata-uninstaller.sh ]; then
  echo >&2 "Removing uninstaller from old location"
  rm -f "${NETDATA_PREFIX}"/usr/libexec/netdata-uninstaller.sh
fi

sed "s|ENVIRONMENT_FILE=\"/etc/netdata/.environment\"|ENVIRONMENT_FILE=\"${NETDATA_PREFIX}/etc/netdata/.environment\"|" packaging/installer/netdata-uninstaller.sh > "${NETDATA_PREFIX}/usr/libexec/netdata/netdata-uninstaller.sh"
chmod 750 "${NETDATA_PREFIX}/usr/libexec/netdata/netdata-uninstaller.sh"

# -----------------------------------------------------------------------------
progress "Basic netdata instructions"

cat << END

netdata by default listens on all IPs on port ${NETDATA_PORT},
so you can access it with:

  ${TPUT_CYAN}${TPUT_BOLD}http://this.machine.ip:${NETDATA_PORT}/${TPUT_RESET}

To stop netdata run:

  ${TPUT_YELLOW}${TPUT_BOLD}${NETDATA_STOP_CMD}${TPUT_RESET}

To start netdata run:

  ${TPUT_YELLOW}${TPUT_BOLD}${NETDATA_START_CMD}${TPUT_RESET}

END
echo >&2 "Uninstall script copied to: ${TPUT_RED}${TPUT_BOLD}${NETDATA_PREFIX}/usr/libexec/netdata/netdata-uninstaller.sh${TPUT_RESET}"
echo >&2

# -----------------------------------------------------------------------------
progress "Installing (but not enabling) the netdata updater tool"
install_netdata_updater || run_failed "Cannot install netdata updater tool."

# -----------------------------------------------------------------------------
progress "Wrap up environment set up"

# Save environment variables
echo >&2 "Preparing .environment file"
cat << EOF > "${NETDATA_USER_CONFIG_DIR}/.environment"
# Created by installer
PATH="${PATH}"
CFLAGS="${CFLAGS}"
LDFLAGS="${LDFLAGS}"
MAKEOPTS="${MAKEOPTS}"
NETDATA_TMPDIR="${TMPDIR}"
NETDATA_PREFIX="${NETDATA_PREFIX}"
NETDATA_CMAKE_OPTIONS="${NETDATA_CMAKE_OPTIONS}"
NETDATA_ADDED_TO_GROUPS="${NETDATA_ADDED_TO_GROUPS}"
INSTALL_UID="$(id -u)"
NETDATA_GROUP="${NETDATA_GROUP}"
REINSTALL_OPTIONS="${REINSTALL_OPTIONS}"
RELEASE_CHANNEL="${RELEASE_CHANNEL}"
IS_NETDATA_STATIC_BINARY="${IS_NETDATA_STATIC_BINARY}"
NETDATA_LIB_DIR="${NETDATA_LIB_DIR}"
EOF
run chmod 0644 "${NETDATA_USER_CONFIG_DIR}/.environment"

echo >&2 "Setting netdata.tarball.checksum to 'new_installation'"
cat << EOF > "${NETDATA_LIB_DIR}/netdata.tarball.checksum"
new_installation
EOF

print_deferred_errors

# -----------------------------------------------------------------------------
echo >&2
progress "We are done!"

if [ ${started} -eq 1 ]; then
  netdata_banner
  progress "is installed and running now!"
else
  netdata_banner
  progress "is installed now!"
fi

echo >&2 "  Enjoy X-Ray Vision for your infrastructure..."
echo >&2
exit 0
