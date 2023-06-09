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

download_go() {
  download_file "${1}" "${2}" "go.d plugin" "go"
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
  --disable-go               Disable installation of go.d.plugin.
  --disable-ebpf             Disable eBPF Kernel plugin. Default: enabled.
  --disable-cloud            Disable all Netdata Cloud functionality.
  --require-cloud            Fail the install if it can't build Netdata Cloud support.
  --enable-plugin-freeipmi   Enable the FreeIPMI plugin. Default: enable it when libipmimonitoring is available.
  --disable-plugin-freeipmi  Explicitly disable the FreeIPMI plugin.
  --disable-https            Explicitly disable TLS support.
  --disable-dbengine         Explicitly disable DB engine support.
  --enable-plugin-nfacct     Enable nfacct plugin. Default: enable it when libmnl and libnetfilter_acct are available.
  --disable-plugin-nfacct    Explicitly disable the nfacct plugin.
  --enable-plugin-xenstat    Enable the xenstat plugin. Default: enable it when libxenstat and libyajl are available.
  --disable-plugin-xenstat   Explicitly disable the xenstat plugin.
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

Netdata will by default be compiled with gcc optimization -O2
If you need to pass different CFLAGS, use something like this:

  CFLAGS="<gcc options>" ${PROGRAM} [options]

If you also need to provide different LDFLAGS, use something like this:

  LDFLAGS="<extra ldflag options>" ${PROGRAM} [options]

or use the following if both LDFLAGS and CFLAGS need to be overridden:

  CFLAGS="<gcc options>" LDFLAGS="<extra ld options>" ${PROGRAM} [options]

For the installer to complete successfully, you will need these packages installed:

  gcc
  make
  autoconf
  automake
  pkg-config
  zlib1g-dev (or zlib-devel)
  uuid-dev (or libuuid-devel)

For the plugins, you will at least need:

  curl
  bash (v4+)
  python (v2 or v3)
  node.js

HEREDOC
}

DONOTSTART=0
DONOTWAIT=0
NETDATA_PREFIX=
LIBS_ARE_HERE=0
NETDATA_ENABLE_ML=""
NETDATA_CONFIGURE_OPTIONS="${NETDATA_CONFIGURE_OPTIONS-}"
RELEASE_CHANNEL="nightly" # valid values are 'nightly' and 'stable'
IS_NETDATA_STATIC_BINARY="${IS_NETDATA_STATIC_BINARY:-"no"}"
while [ -n "${1}" ]; do
  case "${1}" in
    "--zlib-is-really-here") LIBS_ARE_HERE=1 ;;
    "--libs-are-really-here") LIBS_ARE_HERE=1 ;;
    "--use-system-protobuf")
      USE_SYSTEM_PROTOBUF=1
      NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--without-bundled-protobuf}" | sed 's/$/ --without-bundled-protobuf/g')"
    ;;
    "--dont-scrub-cflags-even-though-it-may-break-things") DONT_SCRUB_CFLAGS_EVEN_THOUGH_IT_MAY_BREAK_THINGS=1 ;;
    "--dont-start-it") DONOTSTART=1 ;;
    "--dont-wait") DONOTWAIT=1 ;;
    "--auto-update" | "-u") ;;
    "--auto-update-type") ;;
    "--stable-channel") RELEASE_CHANNEL="stable" ;;
    "--nightly-channel") RELEASE_CHANNEL="nightly" ;;
    "--enable-plugin-freeipmi") NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--enable-plugin-freeipmi)}" | sed 's/$/ --enable-plugin-freeipmi/g')" ;;
    "--disable-plugin-freeipmi") NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-plugin-freeipmi)}" | sed 's/$/ --disable-plugin-freeipmi/g')" ;;
    "--disable-https") NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-https)}" | sed 's/$/ --disable-plugin-https/g')" ;;
    "--disable-dbengine")
      NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-dbengine)}" | sed 's/$/ --disable-dbengine/g')"
      ;;
    "--enable-plugin-nfacct") NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--enable-plugin-nfacct)}" | sed 's/$/ --enable-plugin-nfacct/g')" ;;
    "--disable-plugin-nfacct") NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-plugin-nfacct)}" | sed 's/$/ --disable-plugin-nfacct/g')" ;;
    "--enable-plugin-xenstat") NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--enable-plugin-xenstat)}" | sed 's/$/ --enable-plugin-xenstat/g')" ;;
    "--disable-plugin-xenstat") NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-plugin-xenstat)}" | sed 's/$/ --disable-plugin-xenstat/g')" ;;
    "--enable-exporting-kinesis" | "--enable-backend-kinesis")
      NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--enable-exporting-kinesis)}" | sed 's/$/ --enable-exporting-kinesis/g')" ;;
    "--disable-exporting-kinesis" | "--disable-backend-kinesis")
      NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-exporting-kinesis)}" | sed 's/$/ --disable-exporting-kinesis/g')" ;;
    "--enable-exporting-prometheus-remote-write" | "--enable-backend-prometheus-remote-write")
      NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--enable-exporting-prometheus-remote-write)}" | sed 's/$/ --enable-exporting-prometheus-remote-write/g')" ;;
    "--disable-exporting-prometheus-remote-write" | "--disable-backend-prometheus-remote-write")
      NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-exporting-prometheus-remote-write)}" | sed 's/$/ --disable-exporting-prometheus-remote-write/g')"
      NETDATA_DISABLE_PROMETHEUS=1
      ;;
    "--enable-exporting-mongodb" | "--enable-backend-mongodb")
      NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--enable-exporting-mongodb)}" | sed 's/$/ --enable-exporting-mongodb/g')" ;;
    "--disable-exporting-mongodb" | "--disable-backend-mongodb")
      NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-exporting-mongodb)}" | sed 's/$/ --disable-exporting-mongodb/g')" ;;
    "--enable-exporting-pubsub") NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--enable-exporting-pubsub)}" | sed  's/$/ --enable-exporting-pubsub/g')" ;;
    "--disable-exporting-pubsub") NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-exporting-pubsub)}" | sed 's/$/ --disable-exporting-pubsub/g')" ;;
    "--enable-lto") NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--enable-lto)}" | sed 's/$/ --enable-lto/g')" ;;
    "--enable-ml")
      NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--enable-ml)}" | sed 's/$/ --enable-ml/g')"
      NETDATA_ENABLE_ML=1
      ;;
    "--disable-ml")
      NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-ml)}" | sed 's/$/ --disable-ml/g')"
      NETDATA_ENABLE_ML=0
      ;;
    "--disable-lto") NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-lto)}" | sed 's/$/ --disable-lto/g')" ;;
    "--disable-x86-sse") NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-x86-sse)}" | sed 's/$/ --disable-x86-sse/g')" ;;
    "--disable-telemetry") NETDATA_DISABLE_TELEMETRY=1 ;;
    "--disable-go") NETDATA_DISABLE_GO=1 ;;
    "--enable-ebpf") NETDATA_DISABLE_EBPF=0 ;;
    "--disable-ebpf") NETDATA_DISABLE_EBPF=1 NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-ebpf)}" | sed 's/$/ --disable-ebpf/g')" ;;
    "--skip-available-ram-check") SKIP_RAM_CHECK=1 ;;
    "--one-time-build") NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-dependency-tracking)}" | sed 's/$/ --disable-dependency-tracking/g')" ;;
    "--disable-cloud")
      if [ -n "${NETDATA_REQUIRE_CLOUD}" ]; then
        warning "Cloud explicitly enabled, ignoring --disable-cloud."
      else
        NETDATA_DISABLE_CLOUD=1
        NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-cloud)}" | sed 's/$/ --disable-cloud/g')"
      fi
      ;;
    "--require-cloud")
      if [ -n "${NETDATA_DISABLE_CLOUD}" ]; then
        warning "Cloud explicitly disabled, ignoring --require-cloud."
      else
        NETDATA_REQUIRE_CLOUD=1
        NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--enable-cloud)}" | sed 's/$/ --enable-cloud/g')"
      fi
      ;;
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

make="make"
# See: https://github.com/netdata/netdata/issues/9163
if [ "$(uname -s)" = "FreeBSD" ]; then
  make="gmake"
  NETDATA_CONFIGURE_OPTIONS="$NETDATA_CONFIGURE_OPTIONS --disable-dependency-tracking"
fi

if [ "$(uname -s)" = "Linux" ] && [ -f /proc/meminfo ]; then
  mega="$((1024 * 1024))"
  base=1024
  scale=256

  # shellcheck disable=SC2086
  if [ -n "${MAKEOPTS}" ]; then
    proc_count="$(echo ${MAKEOPTS} | grep -oE '\-j *[[:digit:]]+' | tr -d '\-j ')"
  else
    proc_count="$(find_processors)"
  fi

  target_ram="$((base * mega + (scale * mega * (proc_count - 1))))"
  total_ram="$(grep MemTotal /proc/meminfo | cut -d ':' -f 2 | tr -d ' kB')"
  total_ram="$((total_ram * 1024))"

  if [ "${total_ram}" -le "$((base * mega))" ] && [ -z "${NETDATA_ENABLE_ML}" ]; then
    NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-ml)}" | sed 's/$/ --disable-ml/g')"
    NETDATA_ENABLE_ML=0
  fi

  if [ -z "${MAKEOPTS}" ]; then
    MAKEOPTS="-j${proc_count}"

    while [ "${target_ram}" -gt "${total_ram}" ] && [ "${proc_count}" -gt 1 ]; do
      proc_count="$((proc_count - 1))"
      target_ram="$((base * mega + (scale * mega * (proc_count - 1))))"
      MAKEOPTS="-j${proc_count}"
    done
  else
    if [ "${target_ram}" -gt "${total_ram}" ] && [ "${proc_count}" -gt 1 ] && [ -z "${SKIP_RAM_CHECK}" ]; then
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

if [ "$(id -u)" -ne 0 ]; then
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
progress "real-time performance monitoring, done right!"
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

have_autotools=
if [ "$(type autoreconf 2> /dev/null)" ]; then
  autoconf_maj_min() {
    OLDIFS=$IFS
    IFS=.-
    maj=$1
    min=$2

    # shellcheck disable=SC2046
    set -- $(autoreconf -V | sed -ne '1s/.* \([^ ]*\)$/\1/p')
    # shellcheck disable=SC2086
    eval $maj=\$1 $min=\$2
    IFS=$OLDIFS
  }
  autoconf_maj_min AMAJ AMIN

  if [ "$AMAJ" -gt 2 ]; then
    have_autotools=Y
  elif [ "$AMAJ" -eq 2 ] && [ "$AMIN" -ge 60 ]; then
    have_autotools=Y
  else
    echo "Found autotools $AMAJ.$AMIN"
  fi
else
  echo "No autotools found"
fi

if [ ! "$have_autotools" ]; then
  if [ -f configure ]; then
    echo "Will skip autoreconf step"
  else
    fatal "Could not find a usable version of GNU autotools, which is required for building Netdata. Version 2.60 or later of GNU autotools is required." I0001
  fi
fi

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
build_protobuf() {
  env_cmd=''

  if [ -z "${DONT_SCRUB_CFLAGS_EVEN_THOUGH_IT_MAY_BREAK_THINGS}" ]; then
    env_cmd="env CFLAGS='-fPIC -pipe' CXXFLAGS='-fPIC -pipe' LDFLAGS="
  fi

  cd "${1}" > /dev/null || return 1
  if ! run eval "${env_cmd} ./configure --disable-shared --without-zlib --disable-dependency-tracking --with-pic"; then
    cd - > /dev/null || return 1
    return 1
  fi

  if ! run eval "${env_cmd} ${make} ${MAKEOPTS}"; then
    cd - > /dev/null || return 1
    return 1
  fi

  cd - > /dev/null || return 1
}

copy_protobuf() {
  target_dir="${PWD}/externaldeps/protobuf"

  run mkdir -p "${target_dir}" || return 1
  run cp -a "${1}/src" "${target_dir}" || return 1
}

bundle_protobuf() {
  if [ -n "${NETDATA_DISABLE_CLOUD}" ] && [ -n "${NETDATA_DISABLE_PROMETHEUS}" ]; then
    echo "Skipping protobuf"
    return 0
  fi

  if [ -n "${USE_SYSTEM_PROTOBUF}" ]; then
    echo "Skipping protobuf"
    warning "You have requested use of a system copy of protobuf. This should work, but it is not recommended as it's very likely to break if you upgrade the currently installed version of protobuf."
    return 0
  fi

  [ -n "${GITHUB_ACTIONS}" ] && echo "::group::Bundling protobuf."

  PROTOBUF_PACKAGE_VERSION="$(cat packaging/protobuf.version)"

  if [ -f "${PWD}/externaldeps/protobuf/.version" ] && [ "${PROTOBUF_PACKAGE_VERSION}" = "$(cat "${PWD}/externaldeps/protobuf/.version")" ]
  then
    echo >&2 "Found compiled protobuf, same version, not compiling it again. Remove file '${PWD}/externaldeps/protobuf/.version' to recompile."
    NETDATA_CONFIGURE_OPTIONS="${NETDATA_CONFIGURE_OPTIONS} --with-bundled-protobuf"
    return 0
  fi

  tmp="$(mktemp -d -t netdata-protobuf-XXXXXX)"
  PROTOBUF_PACKAGE_BASENAME="protobuf-cpp-${PROTOBUF_PACKAGE_VERSION}.tar.gz"

  if fetch_and_verify "protobuf" \
    "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOBUF_PACKAGE_VERSION}/${PROTOBUF_PACKAGE_BASENAME}" \
    "${PROTOBUF_PACKAGE_BASENAME}" \
    "${tmp}" \
    "${NETDATA_LOCAL_TARBALL_VERRIDE_PROTOBUF}"; then
    if run tar --no-same-owner -xf "${tmp}/${PROTOBUF_PACKAGE_BASENAME}" -C "${tmp}" &&
      build_protobuf "${tmp}/protobuf-${PROTOBUF_PACKAGE_VERSION}" &&
      copy_protobuf "${tmp}/protobuf-${PROTOBUF_PACKAGE_VERSION}" &&
      echo "${PROTOBUF_PACKAGE_VERSION}" >"${PWD}/externaldeps/protobuf/.version" &&
      rm -rf "${tmp}"; then
      run_ok "protobuf built and prepared."
      NETDATA_CONFIGURE_OPTIONS="${NETDATA_CONFIGURE_OPTIONS} --with-bundled-protobuf"
    else
      run_failed "Failed to build protobuf. Netdata Cloud support will not be available in this build."
    fi
  else
    run_failed "Unable to fetch sources for protobuf. Netdata Cloud support will not be available in this build."
  fi

  [ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"
}

bundle_protobuf

# -----------------------------------------------------------------------------
build_jsonc() {
  env_cmd=''

  if [ -z "${DONT_SCRUB_CFLAGS_EVEN_THOUGH_IT_MAY_BREAK_THINGS}" ]; then
    env_cmd="env CFLAGS='-fPIC -pipe' CXXFLAGS='-fPIC -pipe' LDFLAGS="
  fi

  cd "${1}" > /dev/null || exit 1
  run eval "${env_cmd} cmake -DBUILD_SHARED_LIBS=OFF ."
  run eval "${env_cmd} ${make} ${MAKEOPTS}"
  cd - > /dev/null || return 1
}

copy_jsonc() {
  target_dir="${PWD}/externaldeps/jsonc"

  run mkdir -p "${target_dir}" "${target_dir}/json-c" || return 1

  run cp "${1}/libjson-c.a" "${target_dir}/libjson-c.a" || return 1
  # shellcheck disable=SC2086
  run cp ${1}/*.h "${target_dir}/json-c" || return 1
}

bundle_jsonc() {
  # If --build-json-c flag or not json-c on system, then bundle our own json-c
  if [ -z "${NETDATA_BUILD_JSON_C}" ] && pkg-config json-c; then
    return 0
  fi

  if [ -z "$(command -v cmake)" ]; then
    run_failed "Could not find cmake, which is required to build JSON-C. The install process will continue, but Netdata Cloud support will be disabled."
    return 0
  fi

  [ -n "${GITHUB_ACTIONS}" ] && echo "::group::Bundling JSON-C."

  progress "Prepare JSON-C"

  JSONC_PACKAGE_VERSION="$(cat packaging/jsonc.version)"

  tmp="$(mktemp -d -t netdata-jsonc-XXXXXX)"
  JSONC_PACKAGE_BASENAME="json-c-${JSONC_PACKAGE_VERSION}.tar.gz"

  if fetch_and_verify "jsonc" \
    "https://github.com/json-c/json-c/archive/${JSONC_PACKAGE_BASENAME}" \
    "${JSONC_PACKAGE_BASENAME}" \
    "${tmp}" \
    "${NETDATA_LOCAL_TARBALL_OVERRIDE_JSONC}"; then
    if run tar --no-same-owner -xf "${tmp}/${JSONC_PACKAGE_BASENAME}" -C "${tmp}" &&
      build_jsonc "${tmp}/json-c-json-c-${JSONC_PACKAGE_VERSION}" &&
      copy_jsonc "${tmp}/json-c-json-c-${JSONC_PACKAGE_VERSION}" &&
      rm -rf "${tmp}"; then
      run_ok "JSON-C built and prepared."
    else
      run_failed "Failed to build JSON-C, Netdata Cloud support will be disabled in this build."
    fi
  else
    run_failed "Unable to fetch sources for JSON-C, Netdata Cloud support will be disabled in this build."
  fi

  [ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"
}

bundle_jsonc

# -----------------------------------------------------------------------------
build_yaml() {
  env_cmd=''

  if [ -z "${DONT_SCRUB_CFLAGS_EVEN_THOUGH_IT_MAY_BREAK_THINGS}" ]; then
    env_cmd="env CFLAGS='-fPIC -pipe -Wno-unused-value' CXXFLAGS='-fPIC -pipe' LDFLAGS="
  fi

  cd "${1}" > /dev/null || return 1
  run eval "${env_cmd} ./configure --disable-shared --disable-dependency-tracking --with-pic"
  run eval "${env_cmd} ${make} ${MAKEOPTS}"
  cd - > /dev/null || return 1
}

copy_yaml() {
  target_dir="${PWD}/externaldeps/libyaml"

  run mkdir -p "${target_dir}" || return 1

  run cp "${1}/src/.libs/libyaml.a" "${target_dir}/libyaml.a" || return 1
  run cp "${1}/include/yaml.h" "${target_dir}/" || return 1
}

bundle_yaml() {
  if pkg-config yaml-0.1; then
    return 0
  fi

  if [ -z "$(command -v cmake)" ]; then
    run_failed "Could not find cmake, which is required to build YAML. Critical error."
    return 0
  fi

  [ -n "${GITHUB_ACTIONS}" ] && echo "::group::Bundling YAML."

  progress "Prepare YAML"

  YAML_PACKAGE_VERSION="$(cat packaging/yaml.version)"

  tmp="$(mktemp -d -t netdata-yaml-XXXXXX)"
  YAML_PACKAGE_BASENAME="yaml-${YAML_PACKAGE_VERSION}.tar.gz"

  if fetch_and_verify "yaml" \
    "https://github.com/yaml/libyaml/releases/download/${YAML_PACKAGE_VERSION}/${YAML_PACKAGE_BASENAME}" \
    "${YAML_PACKAGE_BASENAME}" \
    "${tmp}" \
    "${NETDATA_LOCAL_TARBALL_OVERRIDE_YAML}"; then
    if run tar --no-same-owner -xf "${tmp}/${YAML_PACKAGE_BASENAME}" -C "${tmp}" &&
      build_yaml "${tmp}/yaml-${YAML_PACKAGE_VERSION}" &&
      copy_yaml "${tmp}/yaml-${YAML_PACKAGE_VERSION}" &&
      rm -rf "${tmp}"; then
      run_ok "YAML built and prepared."
    else
      run_failed "Failed to build YAML, critical error."
    fi
  else
    run_failed "Unable to fetch sources for YAML, critical error."
  fi

  [ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"
}

bundle_yaml

# -----------------------------------------------------------------------------

get_kernel_version() {
  r="$(uname -r | cut -f 1 -d '-')"

  tmpfile="$(mktemp)"
  echo "${r}" | tr '.' ' ' > "${tmpfile}"

  read -r maj min patch _ < "${tmpfile}"

  rm -f "${tmpfile}"

  printf "%03d%03d%03d" "${maj}" "${min}" "${patch}"
}

detect_libc() {
  libc=
  if ldd --version 2>&1 | grep -q -i glibc; then
    echo >&2 " Detected GLIBC"
    libc="glibc"
  elif ldd --version 2>&1 | grep -q -i 'gnu libc'; then
    echo >&2 " Detected GLIBC"
    libc="glibc"
  elif ldd --version 2>&1 | grep -q -i musl; then
    echo >&2 " Detected musl"
    libc="musl"
  else
      cmd=$(ldd /bin/sh | grep -w libc | cut -d" " -f 3)
      if bash -c "${cmd}" 2>&1 | grep -q -i "GNU C Library"; then
        echo >&2 " Detected GLIBC"
        libc="glibc"
      fi
  fi

  if [ -z "$libc" ]; then
    warning "Cannot detect a supported libc on your system, eBPF support will be disabled."
    return 1
  fi

  echo "${libc}"
  return 0
}

build_libbpf() {
  cd "${1}/src" > /dev/null || return 1
  mkdir root build
  # shellcheck disable=SC2086
  run env CFLAGS='-fPIC -pipe' CXXFLAGS='-fPIC -pipe' LDFLAGS= BUILD_STATIC_ONLY=y OBJDIR=build DESTDIR=.. ${make} ${MAKEOPTS} install
  cd - > /dev/null || return 1
}

copy_libbpf() {
  target_dir="${PWD}/externaldeps/libbpf"

  if [ "$(uname -m)" = x86_64 ]; then
    lib_subdir="lib64"
  else
    lib_subdir="lib"
  fi

  run mkdir -p "${target_dir}" || return 1

  run cp "${1}/usr/${lib_subdir}/libbpf.a" "${target_dir}/libbpf.a" || return 1
  run cp -r "${1}/usr/include" "${target_dir}" || return 1
  run cp -r "${1}/include/uapi" "${target_dir}/include" || return 1
}

bundle_libbpf() {
  if { [ -n "${NETDATA_DISABLE_EBPF}" ] && [ "${NETDATA_DISABLE_EBPF}" = 1 ]; } || [ "$(uname -s)" != Linux ]; then
    return 0
  fi

  # When libc is not detected, we do not have necessity to compile libbpf and we should not do download of eBPF programs
  libc="${EBPF_LIBC:-"$(detect_libc)"}"
  if [ -z "$libc" ]; then
      NETDATA_DISABLE_EBPF=1
      NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-ebpf)}" | sed 's/$/ --disable-ebpf/g')"
      return 0
  fi

  [ -n "${GITHUB_ACTIONS}" ] && echo "::group::Bundling libbpf."

  progress "Prepare libbpf"

  if [ "$(get_kernel_version)" -ge "004014000" ]; then
    LIBBPF_PACKAGE_VERSION="$(cat packaging/current_libbpf.version)"
    LIBBPF_PACKAGE_COMPONENT="current_libbpf"
  else
    LIBBPF_PACKAGE_VERSION="$(cat packaging/libbpf_0_0_9.version)"
    LIBBPF_PACKAGE_COMPONENT="libbpf_0_0_9"
  fi

  tmp="$(mktemp -d -t netdata-libbpf-XXXXXX)"
  LIBBPF_PACKAGE_BASENAME="v${LIBBPF_PACKAGE_VERSION}.tar.gz"

  if fetch_and_verify "${LIBBPF_PACKAGE_COMPONENT}" \
    "https://github.com/netdata/libbpf/archive/${LIBBPF_PACKAGE_BASENAME}" \
    "${LIBBPF_PACKAGE_BASENAME}" \
    "${tmp}" \
    "${NETDATA_LOCAL_TARBALL_OVERRIDE_LIBBPF}"; then
    if run tar --no-same-owner -xf "${tmp}/${LIBBPF_PACKAGE_BASENAME}" -C "${tmp}" &&
      build_libbpf "${tmp}/libbpf-${LIBBPF_PACKAGE_VERSION}" &&
      copy_libbpf "${tmp}/libbpf-${LIBBPF_PACKAGE_VERSION}" &&
      rm -rf "${tmp}"; then
      run_ok "libbpf built and prepared."
    else
      if [ -n "${NETDATA_DISABLE_EBPF}" ] && [ "${NETDATA_DISABLE_EBPF}" = 0 ]; then
        fatal "failed to build libbpf." I0005
      else
        run_failed "Failed to build libbpf. eBPF support will be disabled"
      fi
    fi
  else
    if [ -n "${NETDATA_DISABLE_EBPF}" ] && [ "${NETDATA_DISABLE_EBPF}" = 0 ]; then
      fatal "Failed to fetch sources for libbpf." I0006
    else
      run_failed "Unable to fetch sources for libbpf. eBPF support will be disabled"
    fi
  fi

  [ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"
}

bundle_libbpf

copy_co_re() {
  cp -R "${1}/includes" "collectors/ebpf.plugin/"
}

bundle_ebpf_co_re() {
  if { [ -n "${NETDATA_DISABLE_EBPF}" ] && [ "${NETDATA_DISABLE_EBPF}" = 1 ]; } || [ "$(uname -s)" != Linux ]; then
    return 0
  fi

  [ -n "${GITHUB_ACTIONS}" ] && echo "::group::Bundling libbpf."

  progress "eBPF CO-RE"

  CORE_PACKAGE_VERSION="$(cat packaging/ebpf-co-re.version)"

  tmp="$(mktemp -d -t netdata-ebpf-co-re-XXXXXX)"
  CORE_PACKAGE_BASENAME="netdata-ebpf-co-re-glibc-${CORE_PACKAGE_VERSION}.tar.xz"

  if fetch_and_verify "ebpf-co-re" \
    "https://github.com/netdata/ebpf-co-re/releases/download/${CORE_PACKAGE_VERSION}/${CORE_PACKAGE_BASENAME}" \
    "${CORE_PACKAGE_BASENAME}" \
    "${tmp}" \
    "${NETDATA_LOCAL_TARBALL_OVERRIDE_CORE}"; then
    if run tar --no-same-owner -xf "${tmp}/${CORE_PACKAGE_BASENAME}" -C "${tmp}" &&
      copy_co_re "${tmp}" &&
      rm -rf "${tmp}"; then
      run_ok "libbpf built and prepared."
    else
      if [ -n "${NETDATA_DISABLE_EBPF}" ] && [ "${NETDATA_DISABLE_EBPF}" = 0 ]; then
        fatal "Failed to get eBPF CO-RE files." I0007
      else
        run_failed "Failed to get eBPF CO-RE files. eBPF support will be disabled"
        NETDATA_DISABLE_EBPF=1
        NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-ebpf)}" | sed 's/$/ --disable-ebpf/g')"
      fi
    fi
  else
    if [ -n "${NETDATA_DISABLE_EBPF}" ] && [ "${NETDATA_DISABLE_EBPF}" = 0 ]; then
      fatal "Failed to fetch eBPF CO-RE files." I0008
    else
      run_failed "Failed to fetch eBPF CO-RE files. eBPF support will be disabled"
      NETDATA_DISABLE_EBPF=1
      NETDATA_CONFIGURE_OPTIONS="$(echo "${NETDATA_CONFIGURE_OPTIONS%--disable-ebpf)}" | sed 's/$/ --disable-ebpf/g')"
    fi
  fi

  [ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"
}

bundle_ebpf_co_re

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
progress "Run autotools to configure the build environment"

if [ "$have_autotools" ]; then
  if ! run autoreconf -ivf; then
    fatal "Failed to prepare Netdata sources." I0009
  fi
fi

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
NETDATA_GROUP="$(id -g -n "${NETDATA_USER}")"
[ -z "${NETDATA_GROUP}" ] && NETDATA_GROUP="${NETDATA_USER}"
echo >&2 "Netdata user and group set to: ${NETDATA_USER}/${NETDATA_GROUP}"

# shellcheck disable=SC2086
if ! run ./configure \
         --prefix="${NETDATA_PREFIX}/usr" \
         --sysconfdir="${NETDATA_PREFIX}/etc" \
         --localstatedir="${NETDATA_PREFIX}/var" \
         --libexecdir="${NETDATA_PREFIX}/usr/libexec" \
         --libdir="${NETDATA_PREFIX}/usr/lib" \
         --with-zlib \
         --with-math \
         --with-user="${NETDATA_USER}" \
         ${NETDATA_CONFIGURE_OPTIONS} \
         CFLAGS="${CFLAGS}" LDFLAGS="${LDFLAGS}"; then
  fatal "Failed to configure Netdata sources." I000A
fi


[ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"

# remove the build_error hook
trap - EXIT

# -----------------------------------------------------------------------------
[ -n "${GITHUB_ACTIONS}" ] && echo "::group::Building Netdata."

progress "Cleanup compilation directory"

run $make clean

# -----------------------------------------------------------------------------
progress "Compile netdata"

# shellcheck disable=SC2086
if ! run $make ${MAKEOPTS}; then
  fatal "Failed to build Netdata." I000B
fi

[ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"

# -----------------------------------------------------------------------------
[ -n "${GITHUB_ACTIONS}" ] && echo "::group::Installing Netdata."

# -----------------------------------------------------------------------------
progress "Install netdata"

if ! run $make install; then
  fatal "Failed to install Netdata." I000C
fi

# -----------------------------------------------------------------------------
progress "Fix generated files permissions"

run chmod 755 ./system/*/init.d/netdata ./system/*/rc.d/netdata ./system/runit/run ./system/install-service.sh

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

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/perf.plugin" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/perf.plugin"
    run chmod 0750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/perf.plugin"
    run sh -c "setcap cap_perfmon+ep \"${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/perf.plugin\" || setcap cap_sys_admin+ep \"${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/perf.plugin\""
  fi

  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/slabinfo.plugin" ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/slabinfo.plugin"
    run chmod 0750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/slabinfo.plugin"
    run setcap cap_dac_read_search+ep "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/slabinfo.plugin"
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
else
  # non-privileged user installation
  run chown "${NETDATA_USER}:${NETDATA_GROUP}" "${NETDATA_LOG_DIR}"
  run chown -R "${NETDATA_USER}:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata"
  run find "${NETDATA_PREFIX}/usr/libexec/netdata" -type f -exec chmod 0755 {} \;
  run find "${NETDATA_PREFIX}/usr/libexec/netdata" -type d -exec chmod 0755 {} \;
fi

[ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"

# -----------------------------------------------------------------------------

# govercomp compares go.d.plugin versions. Exit codes:
# 0 - version1 == version2
# 1 - version1 > version2
# 2 - version2 > version1
# 3 - error

# shellcheck disable=SC2086
govercomp() {
  # version in file:
  # - v0.14.0
  #
  # 'go.d.plugin -v' output variants:
  # - go.d.plugin, version: unknown
  # - go.d.plugin, version: v0.14.1
  # - go.d.plugin, version: v0.14.1-dirty
  # - go.d.plugin, version: v0.14.1-1-g4c5f98c
  # - go.d.plugin, version: v0.14.1-1-g4c5f98c-dirty

  # we need to compare only MAJOR.MINOR.PATCH part
  ver1=$(echo "$1" | grep -E -o "[0-9]+\.[0-9]+\.[0-9]+")
  ver2=$(echo "$2" | grep -E -o "[0-9]+\.[0-9]+\.[0-9]+")

  if [ ${#ver1} -eq 0 ] || [ ${#ver2} -eq 0 ]; then
    return 3
  fi

  num1=$(echo $ver1 | grep -o -E '\.' | wc -l)
  num2=$(echo $ver2 | grep -o -E '\.' | wc -l)

  if [ ${num1} -ne ${num2} ]; then
          return 3
  fi

  for i in $(seq 1 $((num1+1))); do
          x=$(echo $ver1 | cut -d'.' -f$i)
          y=$(echo $ver2 | cut -d'.' -f$i)
    if [ "${x}" -gt "${y}" ]; then
      return 1
    elif [ "${y}" -gt "${x}" ]; then
      return 2
    fi
  done

  return 0
}

should_install_go() {
  if [ -n "${NETDATA_DISABLE_GO+x}" ]; then
    return 1
  fi

  version_in_file="$(cat packaging/go.d.version 2> /dev/null)"
  binary_version=$("${NETDATA_PREFIX}"/usr/libexec/netdata/plugins.d/go.d.plugin -v 2> /dev/null)

  govercomp "$version_in_file" "$binary_version"
  case $? in
    0) return 1 ;; # =
    2) return 1 ;; # <
    *) return 0 ;; # >, error
  esac
}

install_go() {
  if ! should_install_go; then
    return 0
  fi

  [ -n "${GITHUB_ACTIONS}" ] && echo "::group::Installing go.d.plugin."

  # When updating this value, ensure correct checksums in packaging/go.d.checksums
  GO_PACKAGE_VERSION="$(cat packaging/go.d.version)"
  ARCH_MAP='
    i386::386
    i686::386
    x86_64::amd64
    aarch64::arm64
    armv64::arm64
    armv6l::arm
    armv7l::arm
    armv5tel::arm
  '

  progress "Install go.d.plugin"
  ARCH=$(uname -m)
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')

  for index in ${ARCH_MAP}; do
    KEY="${index%%::*}"
    VALUE="${index##*::}"
    if [ "$KEY" = "$ARCH" ]; then
      ARCH="${VALUE}"
      break
    fi
  done
  tmp="$(mktemp -d -t netdata-go-XXXXXX)"
  GO_PACKAGE_BASENAME="go.d.plugin-${GO_PACKAGE_VERSION}.${OS}-${ARCH}.tar.gz"

  if [ -z "${NETDATA_LOCAL_TARBALL_OVERRIDE_GO_PLUGIN}" ]; then
    download_go "https://github.com/netdata/go.d.plugin/releases/download/${GO_PACKAGE_VERSION}/${GO_PACKAGE_BASENAME}" "${tmp}/${GO_PACKAGE_BASENAME}"
  else
    progress "Using provided go.d tarball ${NETDATA_LOCAL_TARBALL_OVERRIDE_GO_PLUGIN}"
    run cp "${NETDATA_LOCAL_TARBALL_OVERRIDE_GO_PLUGIN}" "${tmp}/${GO_PACKAGE_BASENAME}"
  fi

  if [ -z "${NETDATA_LOCAL_TARBALL_OVERRIDE_GO_PLUGIN_CONFIG}" ]; then
    download_go "https://github.com/netdata/go.d.plugin/releases/download/${GO_PACKAGE_VERSION}/config.tar.gz" "${tmp}/config.tar.gz"
  else
    progress "Using provided config file for go.d ${NETDATA_LOCAL_TARBALL_OVERRIDE_GO_PLUGIN_CONFIG}"
    run cp "${NETDATA_LOCAL_TARBALL_OVERRIDE_GO_PLUGIN_CONFIG}" "${tmp}/config.tar.gz"
  fi

  if [ ! -f "${tmp}/${GO_PACKAGE_BASENAME}" ] || [ ! -f "${tmp}/config.tar.gz" ] || [ ! -s "${tmp}/config.tar.gz" ] || [ ! -s "${tmp}/${GO_PACKAGE_BASENAME}" ]; then
    run_failed "go.d plugin download failed, go.d plugin will not be available"
    echo >&2 "Either check the error or consider disabling it by issuing '--disable-go' in the installer"
    echo >&2
    [ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"
    return 0
  fi

  grep "${GO_PACKAGE_BASENAME}\$" "${INSTALLER_DIR}/packaging/go.d.checksums" > "${tmp}/sha256sums.txt" 2> /dev/null
  grep "config.tar.gz" "${INSTALLER_DIR}/packaging/go.d.checksums" >> "${tmp}/sha256sums.txt" 2> /dev/null

  # Checksum validation
  if ! (cd "${tmp}" && safe_sha256sum -c "sha256sums.txt"); then

    echo >&2 "go.d plugin checksum validation failure."
    echo >&2 "Either check the error or consider disabling it by issuing '--disable-go' in the installer"
    echo >&2

    run_failed "go.d.plugin package files checksum validation failed. go.d.plugin will not be available."
    [ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"
    return 0
  fi

  # Install new files
  run rm -rf "${NETDATA_STOCK_CONFIG_DIR}/go.d"
  run rm -rf "${NETDATA_STOCK_CONFIG_DIR}/go.d.conf"
  run tar --no-same-owner -xf "${tmp}/config.tar.gz" -C "${NETDATA_STOCK_CONFIG_DIR}/"
  run chown -R "${ROOT_USER}:${ROOT_GROUP}" "${NETDATA_STOCK_CONFIG_DIR}"

  run tar --no-same-owner -xf "${tmp}/${GO_PACKAGE_BASENAME}"
  run mv "${GO_PACKAGE_BASENAME%.tar.gz}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/go.d.plugin"
  if [ "$(id -u)" -eq 0 ]; then
    run chown "root:${NETDATA_GROUP}" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/go.d.plugin"
  fi
  run chmod 0750 "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/go.d.plugin"
  rm -rf "${tmp}"

  [ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"
}

install_go

if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/go.d.plugin" ]; then
  if command -v setcap 1>/dev/null 2>&1; then
    run setcap "cap_net_admin+epi cap_net_raw=eip" "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/go.d.plugin"
  fi
fi

should_install_ebpf() {
  if [ "${NETDATA_DISABLE_EBPF:=0}" -eq 1 ]; then
    run_failed "eBPF has been explicitly disabled, it will not be available in this install."
    return 1
  fi

  if [ "$(uname -s)" != "Linux" ] || [ "$(uname -m)" != "x86_64" ]; then
    if [ "${NETDATA_DISABLE_EBPF:=1}" -eq 0 ]; then
      run_failed "Currently eBPF is only supported on Linux on X86_64."
    fi

    return 1
  fi

  # Check Kernel Config
  if ! run "${INSTALLER_DIR}"/packaging/check-kernel-config.sh; then
    warning "Kernel unsupported or missing required config (eBPF may not work on your system)"
  fi

  return 0
}

remove_old_ebpf() {
  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/ebpf_process.plugin" ]; then
    echo >&2 "Removing alpha eBPF collector."
    rm -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/ebpf_process.plugin"
  fi

  if [ -f "${NETDATA_PREFIX}/usr/lib/netdata/conf.d/ebpf_process.conf" ]; then
    echo >&2 "Removing alpha eBPF stock file"
    rm -f "${NETDATA_PREFIX}/usr/lib/netdata/conf.d/ebpf_process.conf"
  fi

  if [ -f "${NETDATA_PREFIX}/etc/netdata/ebpf_process.conf" ]; then
    echo >&2 "Renaming eBPF configuration file."
    mv "${NETDATA_PREFIX}/etc/netdata/ebpf_process.conf" "${NETDATA_PREFIX}/etc/netdata/ebpf.d.conf"
  fi

  # Added to remove eBPF programs with name pattern: NAME_VERSION.SUBVERSION.PATCH
  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/pnetdata_ebpf_process.3.10.0.o" ]; then
    echo >&2 "Removing old eBPF programs with patch."
    rm -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/rnetdata_ebpf"*.?.*.*.o
    rm -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/pnetdata_ebpf"*.?.*.*.o
  fi

  # Remove old eBPF program to store new eBPF program inside subdirectory
  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/pnetdata_ebpf_process.3.10.o" ]; then
    echo >&2 "Removing old eBPF programs installed in old directory."
    rm -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/rnetdata_ebpf"*.?.*.o
    rm -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/pnetdata_ebpf"*.?.*.o
  fi

  # Remove old eBPF programs that did not have "rhf" suffix
  if [ ! -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/ebpf.d/pnetdata_ebpf_process.3.10.rhf.o" ]; then
    rm -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/ebpf.d/"*.o
  fi

  # Remove old reject list from previous directory
  if [ -f "${NETDATA_PREFIX}/usr/lib/netdata/conf.d/ebpf_kernel_reject_list.txt" ]; then
    echo >&2 "Removing old ebpf_kernel_reject_list.txt."
    rm -f "${NETDATA_PREFIX}/usr/lib/netdata/conf.d/ebpf_kernel_reject_list.txt"
  fi

  # Remove old reset script
  if [ -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/reset_netdata_trace.sh" ]; then
    echo >&2 "Removing old reset_netdata_trace.sh."
    rm -f "${NETDATA_PREFIX}/usr/libexec/netdata/plugins.d/reset_netdata_trace.sh"
  fi
}

install_ebpf() {
  if ! should_install_ebpf; then
    return 0
  fi

  [ -n "${GITHUB_ACTIONS}" ] && echo "::group::Installing eBPF code."

  remove_old_ebpf

  progress "Installing eBPF plugin"

  # Detect libc
  libc="${EBPF_LIBC:-"$(detect_libc)"}"

  EBPF_VERSION="$(cat packaging/ebpf.version)"
  EBPF_TARBALL="netdata-kernel-collector-${libc}-${EBPF_VERSION}.tar.xz"

  tmp="$(mktemp -d -t netdata-ebpf-XXXXXX)"

  if ! fetch_and_verify "ebpf" \
    "https://github.com/netdata/kernel-collector/releases/download/${EBPF_VERSION}/${EBPF_TARBALL}" \
    "${EBPF_TARBALL}" \
    "${tmp}" \
    "${NETDATA_LOCAL_TARBALL_OVERRIDE_EBPF}"; then
    run_failed "Failed to download eBPF collector package"
    echo 2>&" Removing temporary directory ${tmp} ..."
    rm -rf "${tmp}"

    [ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"
    return 1
  fi

  echo >&2 " Extracting ${EBPF_TARBALL} ..."
  tar --no-same-owner -xf "${tmp}/${EBPF_TARBALL}" -C "${tmp}"

  # chown everything to root:netdata before we start copying out of our package
  run chown -R root:netdata "${tmp}"

  if [ ! -d "${NETDATA_PREFIX}"/usr/libexec/netdata/plugins.d/ebpf.d ]; then
    mkdir "${NETDATA_PREFIX}"/usr/libexec/netdata/plugins.d/ebpf.d
    RET=$?
    if [ "${RET}" != "0" ]; then
      rm -rf "${tmp}"

      [ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"
      return 1
    fi
  fi

  run cp -a -v "${tmp}"/*netdata_ebpf_*.o "${NETDATA_PREFIX}"/usr/libexec/netdata/plugins.d/ebpf.d

  rm -rf "${tmp}"

  [ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"
}

progress "eBPF Kernel Collector"
install_ebpf

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

# TODO(paulfantom): Creation of configuration file should be handled by a build system. Additionally we shouldn't touch configuration files in /etc/netdata/...
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

# -----------------------------------------------------------------------------
progress "Check version.txt"

if [ ! -s web/gui/version.txt ]; then
  cat << VERMSG

${TPUT_BOLD}Version update check warning${TPUT_RESET}

The way you downloaded netdata, we cannot find its version. This means the
Update check on the dashboard, will not work.

If you want to have version update check, please re-install it
following the procedure in:

https://docs.netdata.cloud/packaging/installer/

VERMSG
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
NETDATA_CONFIGURE_OPTIONS="${NETDATA_CONFIGURE_OPTIONS}"
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

echo >&2 "  enjoy real-time performance and health monitoring..."
echo >&2
exit 0
