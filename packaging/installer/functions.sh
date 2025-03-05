#!/bin/sh

# SPDX-License-Identifier: GPL-3.0-or-later

# next unused error code: L0003

# make sure we have a UID
[ -z "${UID}" ] && UID="$(id -u)"
# -----------------------------------------------------------------------------

setup_terminal() {
  TPUT_RESET=""
  TPUT_BLACK=""
  TPUT_RED=""
  TPUT_GREEN=""
  TPUT_YELLOW=""
  TPUT_BLUE=""
  TPUT_PURPLE=""
  TPUT_CYAN=""
  TPUT_WHITE=""
  TPUT_BGBLACK=""
  TPUT_BGRED=""
  TPUT_BGGREEN=""
  TPUT_BGYELLOW=""
  TPUT_BGBLUE=""
  TPUT_BGPURPLE=""
  TPUT_BGCYAN=""
  TPUT_BGWHITE=""
  TPUT_BOLD=""
  TPUT_DIM=""
  TPUT_UNDERLINED=""
  TPUT_BLINK=""
  TPUT_INVERTED=""
  TPUT_STANDOUT=""
  TPUT_BELL=""
  TPUT_CLEAR=""

  # Is stderr on the terminal? If not, then fail
  test -t 2 || return 1

  if command -v tput 1> /dev/null 2>&1; then
    if [ $(($(tput colors 2> /dev/null))) -ge 8 ]; then
      # Enable colors
      TPUT_RESET="$(tput sgr 0)"
      # shellcheck disable=SC2034
      TPUT_BLACK="$(tput setaf 0)"
      # shellcheck disable=SC2034
      TPUT_RED="$(tput setaf 1)"
      TPUT_GREEN="$(tput setaf 2)"
      # shellcheck disable=SC2034
      TPUT_YELLOW="$(tput setaf 3)"
      # shellcheck disable=SC2034
      TPUT_BLUE="$(tput setaf 4)"
      # shellcheck disable=SC2034
      TPUT_PURPLE="$(tput setaf 5)"
      # shellcheck disable=SC2034
      TPUT_CYAN="$(tput setaf 6)"
      TPUT_WHITE="$(tput setaf 7)"
      # shellcheck disable=SC2034
      TPUT_BGBLACK="$(tput setab 0)"
      TPUT_BGRED="$(tput setab 1)"
      TPUT_BGGREEN="$(tput setab 2)"
      # shellcheck disable=SC2034
      TPUT_BGYELLOW="$(tput setab 3)"
      # shellcheck disable=SC2034
      TPUT_BGBLUE="$(tput setab 4)"
      # shellcheck disable=SC2034
      TPUT_BGPURPLE="$(tput setab 5)"
      # shellcheck disable=SC2034
      TPUT_BGCYAN="$(tput setab 6)"
      # shellcheck disable=SC2034
      TPUT_BGWHITE="$(tput setab 7)"
      TPUT_BOLD="$(tput bold)"
      TPUT_DIM="$(tput dim)"
      # shellcheck disable=SC2034
      TPUT_UNDERLINED="$(tput smul)"
      # shellcheck disable=SC2034
      TPUT_BLINK="$(tput blink)"
      # shellcheck disable=SC2034
      TPUT_INVERTED="$(tput rev)"
      # shellcheck disable=SC2034
      TPUT_STANDOUT="$(tput smso)"
      # shellcheck disable=SC2034
      TPUT_BELL="$(tput bel)"
      # shellcheck disable=SC2034
      TPUT_CLEAR="$(tput clear)"
    fi
  fi

  return 0
}
setup_terminal || echo > /dev/null

progress() {
  echo >&2 " --- ${TPUT_DIM}${TPUT_BOLD}${*}${TPUT_RESET} --- "
}

check_for_curl() {
  if [ -z "${curl}" ]; then
    curl="$(PATH="${PATH}:/opt/netdata/bin" command -v curl 2>/dev/null && true)"
  fi
}

get() {
  url="${1}"
  checked=0
  succeeded=0

  check_for_curl

  if [ -n "${curl}" ]; then
    checked=1

    if "${curl}" -q -o - -sSL --connect-timeout 10 --retry 3 "${url}"; then
      succeeded=1
    fi
  fi

  if [ "${succeeded}" -eq 0 ]; then
    if command -v wget > /dev/null 2>&1; then
      checked=1

      if wget -T 15 -O - "${url}"; then
        succeeded=1
      fi
    fi
  fi

  if [ "${succeeded}" -eq 1 ]; then
    return 0
  elif [ "${checked}" -eq 1 ]; then
    return 1
  else
    fatal "I need curl or wget to proceed, but neither is available on this system." "L0002"
  fi
}

download_file() {
  url="${1}"
  dest="${2}"
  name="${3}"
  opt="${4}"

  check_for_curl

  if [ -n "${curl}" ]; then
    checked=1

    if run "${curl}" -q -sSL --connect-timeout 10 --retry 3 --output "${dest}" "${url}"; then
      succeeded=1
    else
      rm -f "${dest}"
    fi
  fi

  if [ "${succeeded}" -eq 0 ]; then
    if command -v wget > /dev/null 2>&1; then
      checked=1

      if run wget -T 15 -O "${dest}" "${url}"; then
        succeeded=1
      fi
    fi
  fi

  if [ "${succeeded}" -eq 1 ]; then
    return 0
  elif [ "${checked}" -eq 1 ]; then
    return 1
  else
    echo >&2
    echo >&2 "Downloading ${name} from '${url}' failed because of missing mandatory packages."
    if [ -n "$opt" ]; then
      echo >&2 "Either add packages or disable it by issuing '--disable-${opt}' in the installer"
    fi
    echo >&2

    run_failed "I need curl or wget to proceed, but neither is available on this system."
  fi
}

# -----------------------------------------------------------------------------
# external component handling

fetch_and_verify() {
  component="${1}"
  url="${2}"
  base_name="${3}"
  tmp="${4}"
  override="${5}"

  if [ -z "${override}" ]; then
    download_file "${url}" "${tmp}/${base_name}" "${component}"
  else
    progress "Using provided ${component} archive ${override}"
    run cp "${override}" "${tmp}/${base_name}"
  fi

  if [ ! -f "${tmp}/${base_name}" ] || [ ! -s "${tmp}/${base_name}" ]; then
    run_failed "Unable to find usable archive for ${component}"
    return 1
  fi

  grep "${base_name}\$" "${INSTALLER_DIR}/packaging/${component}.checksums" > "${tmp}/sha256sums.txt" 2> /dev/null

  # Checksum validation
  if ! (cd "${tmp}" && safe_sha256sum -c "sha256sums.txt"); then
    run_failed "${component} files checksum validation failed."
    return 1
  fi
}

# -----------------------------------------------------------------------------

netdata_banner() {
    l1="  ^" \
    l2="  |.-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-" \
    l4="  +----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+--->" \
    space="  "
    l3f="  |   '-'   '-'   '-'   '-'   '-'"
    l3e="               '-'   '-'   '-'   '-'   '-'   "

    netdata="netdata"
    chartcolor="${TPUT_DIM}"

  echo >&2
  echo >&2 "${chartcolor}${l1}${TPUT_RESET}"
  echo >&2 "${chartcolor}${l2%-.   .-.   .-.   .-.   .-.   .-.   .-.   .-}${space}${TPUT_RESET}${TPUT_BOLD}${TPUT_GREEN}${netdata}${TPUT_RESET}${chartcolor}${l2#  |.-.   .-.   .-.   .-.   .-.   .-.   .-. }${TPUT_RESET}"
  echo >&2 "${chartcolor}${l3f}${l3e}${TPUT_RESET}"
  echo >&2 "${chartcolor}${l4}${TPUT_RESET}"
  echo >&2
}

# -----------------------------------------------------------------------------
# Feature management and configuration commands

enable_feature() {
  NETDATA_CMAKE_OPTIONS="$(echo "${NETDATA_CMAKE_OPTIONS}" | sed -e "s/-DENABLE_${1}=Off[[:space:]]*//g" -e "s/-DENABLE_${1}=On[[:space:]]*//g")"
  if [ "${2}" -eq 1 ]; then
    NETDATA_CMAKE_OPTIONS="$(echo "${NETDATA_CMAKE_OPTIONS}" | sed "s/$/ -DENABLE_${1}=On/")"
  else
    NETDATA_CMAKE_OPTIONS="$(echo "${NETDATA_CMAKE_OPTIONS}" | sed "s/$/ -DENABLE_${1}=Off/")"
  fi
}

check_for_module() {
  if [ -z "${pkgconf}" ]; then
    pkgconf="$(command -v pkgconf 2>/dev/null)"
    [ -z "${pkgconf}" ] && pkgconf="$(command -v pkg-config 2>/dev/null)"
    [ -z "${pkgconf}" ] && fatal "Unable to find a usable pkgconf/pkg-config command, cannot build Netdata." I0013
  fi

  "${pkgconf}" "${1}"
  return "${?}"
}

check_for_feature() {
  feature_name="${1}"
  feature_state="${2}"
  shift 2
  feature_modules="${*}"

  if [ -z "${feature_state}" ]; then
    # shellcheck disable=SC2086
    if check_for_module ${feature_modules}; then
      enable_feature "${feature_name}" 1
    else
      enable_feature "${feature_name}" 0
    fi
  else
    enable_feature "${feature_name}" "${feature_state}"
  fi
}

prepare_cmake_options() {
  NETDATA_CMAKE_OPTIONS="-S ./ -B ${NETDATA_BUILD_DIR} ${CMAKE_OPTS} ${NETDATA_PREFIX+-DCMAKE_INSTALL_PREFIX="${NETDATA_PREFIX}"} ${NETDATA_USER:+-DNETDATA_USER=${NETDATA_USER}} ${NETDATA_CMAKE_OPTIONS} "

  NEED_OLD_CXX=0

  if [ "${FORCE_LEGACY_CXX:-0}" -eq 1 ]; then
    NEED_OLD_CXX=1
  else
    if command -v gcc >/dev/null 2>&1; then
      if [ "$(gcc --version | head -n 1 | sed 's/(.*) //' | cut -f 2 -d ' ' | cut -f 1 -d '.')" -lt 5 ]; then
        NEED_OLD_CXX=1
      fi
    fi

    if command -v clang >/dev/null 2>&1; then
      if [ "$(clang --version | head -n 1 | cut -f 3 -d ' ' | cut -f 1 -d '.')" -lt 4 ]; then
        NEED_OLD_CXX=1
      fi
    fi
  fi

  if [ "${NEED_OLD_CXX}" -eq 1 ]; then
    NETDATA_CMAKE_OPTIONS="${NETDATA_CMAKE_OPTIONS} -DUSE_CXX_11=On"
  fi

  if [ -n "${NETDATA_ENABLE_LTO}" ]; then
    if [ "${NETDATA_ENABLE_LTO}" -eq 1 ]; then
      NETDATA_CMAKE_OPTIONS="${NETDATA_CMAKE_OPTIONS} -DDISABLE_LTO=Off"
    else
      NETDATA_CMAKE_OPTIONS="${NETDATA_CMAKE_OPTIONS} -DDISABLE_LTO=On"
    fi
  fi

  if [ "${ENABLE_GO:-1}" -eq 1 ]; then
    enable_feature PLUGIN_GO 1
  else
    enable_feature PLUGIN_GO 0
  fi

  if [ "${ENABLE_PYTHON:-1}" -eq 1 ]; then
    enable_feature PLUGIN_PYTHON 1
  else
    enable_feature PLUGIN_PYTHON 0
  fi

  if [ "${ENABLE_CHARTS:-1}" -eq 1 ]; then
    enable_feature PLUGIN_CHARTS 1
  else
    enable_feature PLUGIN_CHARTS 0
  fi

  if [ "${USE_SYSTEM_PROTOBUF:-0}" -eq 1 ]; then
    enable_feature BUNDLED_PROTOBUF 0
  else
    enable_feature BUNDLED_PROTOBUF 1
  fi

  if [ -z "${ENABLE_SYSTEMD_JOURNAL}" ]; then
      if check_for_module libsystemd; then
          if check_for_module libelogind; then
              ENABLE_SYSTEMD_JOURNAL=0
          else
              ENABLE_SYSTEMD_JOURNAL=1
          fi
      else
          ENABLE_SYSTEMD_JOURNAL=0
      fi
  fi

  enable_feature PLUGIN_SYSTEMD_JOURNAL "${ENABLE_SYSTEMD_JOURNAL}"

  if command -v cups-config >/dev/null 2>&1 || check_for_module libcups || check_for_module cups; then
    ENABLE_CUPS=1
  else
    ENABLE_CUPS=0
  fi

  enable_feature PLUGIN_CUPS "${ENABLE_CUPS}"

  IS_LINUX=0
  [ "$(uname -s)" = "Linux" ] && IS_LINUX=1
  enable_feature PLUGIN_DEBUGFS "${IS_LINUX}"
  enable_feature PLUGIN_PERF "${IS_LINUX}"
  enable_feature PLUGIN_SLABINFO "${IS_LINUX}"
  enable_feature PLUGIN_CGROUP_NETWORK "${IS_LINUX}"
  enable_feature PLUGIN_LOCAL_LISTENERS "${IS_LINUX}"
  enable_feature PLUGIN_NETWORK_VIEWER "${IS_LINUX}"
  enable_feature PLUGIN_EBPF "${ENABLE_EBPF:-0}"

  enable_feature BUNDLED_JSONC "${NETDATA_BUILD_JSON_C:-0}"
  enable_feature DBENGINE "${ENABLE_DBENGINE:-1}"
  enable_feature H2O "${ENABLE_H2O:-0}"
  enable_feature ML "${NETDATA_ENABLE_ML:-1}"
  enable_feature PLUGIN_APPS "${ENABLE_APPS:-1}"

  check_for_feature EXPORTER_PROMETHEUS_REMOTE_WRITE "${EXPORTER_PROMETHEUS}" snappy
  check_for_feature EXPORTER_MONGODB "${EXPORTER_MONGODB}" libmongoc-1.0
  check_for_feature PLUGIN_FREEIPMI "${ENABLE_FREEIPMI}" libipmimonitoring
  check_for_feature PLUGIN_NFACCT "${ENABLE_NFACCT}" libnetfilter_acct libnml
  check_for_feature PLUGIN_XENSTAT "${ENABLE_XENSTAT}" xenstat xenlight
}

# -----------------------------------------------------------------------------
# portable service command

service_cmd="$(command -v service 2> /dev/null || true)"
rcservice_cmd="$(command -v rc-service 2> /dev/null || true)"
systemctl_cmd="$(command -v systemctl 2> /dev/null || true)"
service() {

  cmd="${1}"
  action="${2}"

  if [ -n "${systemctl_cmd}" ]; then
    run "${systemctl_cmd}" "${action}" "${cmd}"
    return $?
  elif [ -n "${service_cmd}" ]; then
    run "${service_cmd}" "${cmd}" "${action}"
    return $?
  elif [ -n "${rcservice_cmd}" ]; then
    run "${rcservice_cmd}" "${cmd}" "${action}"
    return $?
  fi
  return 1
}

# -----------------------------------------------------------------------------
# portable pidof

safe_pidof() {
  pidof_cmd="$(command -v pidof 2> /dev/null)"
  if [ -n "${pidof_cmd}" ]; then
    ${pidof_cmd} "${@}"
    return $?
  else
    ps -acxo pid,comm |
      sed "s/^ *//g" |
      grep netdata |
      cut -d ' ' -f 1
    return $?
  fi
}

# -----------------------------------------------------------------------------
find_processors() {
  # Most UNIX systems have `nproc` as part of their userland (including Linux and BSD)
  if command -v nproc > /dev/null; then
    nproc && return
  fi

  # macOS has no nproc but it may have gnproc installed from Homebrew or from Macports.
  if command -v gnproc > /dev/null; then
    gnproc && return
  fi

  if [ -f "/proc/cpuinfo" ]; then
    # linux
    cpus=$(grep -c ^processor /proc/cpuinfo)
  else
  # freebsd
    cpus=$(sysctl hw.ncpu 2> /dev/null | grep ^hw.ncpu | cut -d ' ' -f 2)
  fi
  if [ -z "${cpus}" ] || [ $((cpus)) -lt 1 ]; then
    echo 1
  else
    echo "${cpus}"
  fi
}

# -----------------------------------------------------------------------------
exit_reason() {
  if [ -n "${NETDATA_SAVE_WARNINGS}" ]; then
    EXIT_REASON="${1}"
    EXIT_CODE="${2}"
    if [ -n "${NETDATA_PROPAGATE_WARNINGS}" ]; then
      if [ -n "${NETDATA_SCRIPT_STATUS_PATH}" ]; then
        {
          echo "EXIT_REASON=\"${EXIT_REASON}\""
          echo "EXIT_CODE=\"${EXIT_CODE}\""
          echo "NETDATA_WARNINGS=\"${NETDATA_WARNINGS}${SAVED_WARNINGS}\""
        } >> "${NETDATA_SCRIPT_STATUS_PATH}"
      else
        export EXIT_REASON
        export EXIT_CODE
        export NETDATA_WARNINGS="${NETDATA_WARNINGS}${SAVED_WARNINGS}"
      fi
    fi
  fi
}

fatal() {
  printf >&2 "%s ABORTED %s %s \n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD}" "${TPUT_RESET}" "${1}"
  if [ -n "${NETDATA_SAVE_WARNINGS}" ]; then
    SAVED_WARNINGS="${SAVED_WARNINGS}\n  - ${1}"
  fi
  exit_reason "${1}" "${2}"
  exit 1
}

warning() {
  printf >&2 "%s WARNING %s %s\n\n" "${TPUT_BGYELLOW}${TPUT_BLACK}${TPUT_BOLD}" "${TPUT_RESET}" "${1}"
  if [ -n "${NETDATA_SAVE_WARNINGS}" ]; then
    SAVED_WARNINGS="${SAVED_WARNINGS}\n  - ${1}"
  fi
}

run_ok() {
  printf >&2 "%s OK %s %s\n\n" "${TPUT_BGGREEN}${TPUT_WHITE}${TPUT_BOLD}" "${TPUT_RESET}" "${1:-''}"
}

run_failed() {
  printf >&2 "%s FAILED %s %s\n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD}" "${TPUT_RESET}" "${1:-''}"
  if [ -n "${NETDATA_SAVE_WARNINGS}" ] && [ -n "${1:-''}" ]; then
    SAVED_WARNINGS="${SAVED_WARNINGS}\n  - ${1}"
  fi
}

ESCAPED_PRINT_METHOD=
if printf "%s " test > /dev/null 2>&1; then
  ESCAPED_PRINT_METHOD="printfq"
fi
escaped_print() {
  if [ "${ESCAPED_PRINT_METHOD}" = "printfq" ]; then
    printf "%s " "${@}"
  else
    printf "%s" "${*}"
  fi
  return 0
}

run_logfile="/dev/null"
run() {
  local_user="${USER--}"
  local_dir="${PWD}"
  if [ "${UID}" = "0" ]; then
    info="[root ${local_dir}]# "
    info_console="[${TPUT_DIM}${local_dir}${TPUT_RESET}]# "
  else
    info="[${local_user} ${local_dir}]$ "
    info_console="[${TPUT_DIM}${local_dir}${TPUT_RESET}]$ "
  fi

  {
    printf "%s" "${info}"
    escaped_print "${@}"
    printf "%s" " ... "
  } >> "${run_logfile}"

  printf >&2 "%s" "${info_console}${TPUT_BOLD}${TPUT_YELLOW}"
  escaped_print >&2 "${@}"
  printf >&2 "%s\n" "${TPUT_RESET}"

  "${@}"

  ret=$?
  if [ ${ret} -ne 0 ]; then
    run_failed
    printf >> "${run_logfile}" "FAILED with exit code %s\n" "${ret}"
    if [ -n "${NETDATA_SAVE_WARNINGS}" ]; then
      SAVED_WARNINGS="${SAVED_WARNINGS}\n  - Command '${*}' failed with exit code ${ret}."
    fi
  else
    run_ok
    printf >> "${run_logfile}" "OK\n"
  fi

  return ${ret}
}

iscontainer() {
  # man systemd-detect-virt
  cmd=$(command -v systemd-detect-virt 2> /dev/null)
  if [ -n "${cmd}" ] && [ -x "${cmd}" ]; then
    "${cmd}" --container > /dev/null 2>&1 && return 0
  fi

  # /proc/1/sched exposes the host's pid of our init !
  # http://stackoverflow.com/a/37016302
  pid=$(head -n 1 /proc/1/sched 2> /dev/null | {
    # shellcheck disable=SC2034
    IFS='(),#:' read -r name pid th threads
    echo "$pid"
  })
  if [ -n "${pid}" ]; then
    pid=$((pid + 0))
    [ ${pid} -gt 1 ] && return 0
  fi

  # lxc sets environment variable 'container'
  # shellcheck disable=SC2154
  [ -n "${container}" ] && return 0

  # docker creates /.dockerenv
  # http://stackoverflow.com/a/25518345
  [ -f "/.dockerenv" ] && return 0

  # ubuntu and debian supply /bin/running-in-container
  # https://www.apt-browse.org/browse/ubuntu/trusty/main/i386/upstart/1.12.1-0ubuntu4/file/bin/running-in-container
  if [ -x "/bin/running-in-container" ]; then
    "/bin/running-in-container" > /dev/null 2>&1 && return 0
  fi

  return 1
}

get_os_key() {
  if [ -f /etc/os-release ]; then
    # shellcheck disable=SC1091
    . /etc/os-release || return 1
    echo "${ID}-${VERSION_ID}"

  elif [ -f /etc/redhat-release ]; then
    cat /etc/redhat-release
  else
    echo "unknown"
  fi
}

get_group(){
  if command -v getent > /dev/null 2>&1; then
    getent group "${1:-""}"
  else
    grep "^${1}:" /etc/group
  fi
}

issystemd() {
  pids=''
  p=''
  myns=''
  ns=''
  systemctl=''

  # if there is no systemctl command, it is not systemd
  systemctl=$(command -v systemctl 2> /dev/null)
  if [ -z "${systemctl}" ] || [ ! -x "${systemctl}" ]; then
    return 1
  fi

  # Check the output of systemctl is-system-running.
  # If this reports 'offline', it’s not systemd. If it reports 'unknown'
  # or nothing at all (which indicates the command is not supported), it
  # may or may not be systemd, so continue to other checks. If it reports
  # anything else, it is systemd.
  case "$(systemctl is-system-running)" in
    offline) return 1 ;;
    unknown) : ;;
    "") : ;;
    *) return 0 ;;
  esac

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

get_systemd_service_dir() {
  unit_paths="$(systemctl show -p UnitPath | cut -f 2- -d '=' | tr ' ' '\n')"

  if [ -n "${unit_paths}" ]; then
    lib_paths="$(echo "${unit_paths}" | grep -vE '^/(run|etc)' | awk '{line[NR] = $0} END {for (i = NR; i > 0; i--) print line[i]}')"
    etc_paths="$(echo "${unit_paths}" | grep -E '^/etc' | grep -vE '(attached|control)$')"
  else
    lib_paths="/usr/lib/systemd/system /lib/systemd/system /usr/local/lib/systemd/system"
    etc_paths="/etc/systemd/system"
  fi

  for path in ${lib_paths} ${etc_paths}; do
    if [ -d "${path}" ] && [ -w "${path}" ]; then
      echo "${path}"
      return 0
    fi
  done
}

run_install_service_script() {
  if [ -z "${tmpdir}" ]; then
    tmpdir="${TMPDIR:-/tmp}"
  fi

  # shellcheck disable=SC2154
  save_path="${tmpdir}/netdata-service-cmds"
  # shellcheck disable=SC2068
  "${NETDATA_PREFIX}/usr/libexec/netdata/install-service.sh" --save-cmds "${save_path}" ${@}

  case $? in
    0)
      if [ -r "${save_path}" ]; then
        # shellcheck disable=SC1090
        . "${save_path}"
      fi

      if [ -z "${NETDATA_INSTALLER_START_CMD}" ]; then
        if [ -n "${NETDATA_START_CMD}" ]; then
          NETDATA_INSTALLER_START_CMD="${NETDATA_START_CMD}"
        else
          NETDATA_INSTALLER_START_CMD="netdata"
        fi
      fi
      ;;
    1)
      if [ -z "${NETDATA_SERVICE_WARNED_1}" ]; then
        warning "Intenral error encountered while attempting to install or manage Netdata as a system service. This is probably a bug."
        NETDATA_SERVICE_WARNED_1=1
      fi
      ;;
    2)
      if [ -z "${NETDATA_SERVICE_WARNED_2}" ]; then
        warning "Failed to detect system service manager type. Cannot cleanly install or manage Netdata as a system service. If you are running this script in a container, this is expected and can safely be ignored."
        NETDATA_SERVICE_WARNED_2=1
      fi
      ;;
    3)
      if [ -z "${NETDATA_SERVICE_WARNED_3}" ]; then
        warning "Detected an unsupported system service manager. Manual setup will be required to manage Netdata as a system service."
        NETDATA_SERVICE_WARNED_3=1
      fi
      ;;
    4)
      if [ -z "${NETDATA_SERVICE_WARNED_4}" ]; then
        warning "Detected a supported system service manager, but failed to install Netdata as a system service. Usually this is a result of incorrect permissions. Manually running ${NETDATA_PREFIX}/usr/libexec/netdata/install-service.sh may provide more information about the exact issue."
        NETDATA_SERVICE_WARNED_4=1
      fi
      ;;
    5)
      if [ -z "${NETDATA_SERVICE_WARNED_5}" ]; then
        warning "We do not support managing Netdata as a system service on this platform. Manual setup will be required."
        NETDATA_SERVICE_WARNED_5=1
      fi
      ;;
  esac
}

install_netdata_service() {
  if [ "${UID}" -eq 0 ]; then
    if [ -x "${NETDATA_PREFIX}/usr/libexec/netdata/install-service.sh" ]; then
      run_install_service_script && return 0
    else
      warning "Could not find service install script, not installing Netdata as a system service."
    fi
  fi

  return 1
}

# -----------------------------------------------------------------------------
# stop netdata

pidisnetdata() {
  if [ -d /proc/self ]; then
    if [ -z "$1" ] || [ ! -f "/proc/$1/stat" ]; then
      return 1
    fi
    [ "$(cut -d '(' -f 2 "/proc/$1/stat" | cut -d ')' -f 1)" = "netdata" ] && return 0
    return 1
  fi
  return 0
}

stop_netdata_on_pid() {
  pid="${1}"
  ret=0
  count=0

  pidisnetdata "${pid}" || return 0

  printf >&2 "Stopping netdata on pid %s ..." "${pid}"
  while [ -n "${pid}" ] && [ ${ret} -eq 0 ]; do
    if [ ${count} -gt 24 ]; then
      warning "Cannot stop netdata agent with PID ${pid}."
      return 1
    fi

    count=$((count + 1))

    pidisnetdata "${pid}" || ret=1
    if [ ${ret} -eq 1 ]; then
      break
    fi

    if [ ${count} -lt 12 ]; then
      run kill "${pid}" 2> /dev/null
      ret=$?
    else
      run kill -9 "${pid}" 2> /dev/null
      ret=$?
    fi

    test ${ret} -eq 0 && printf >&2 "." && sleep 5

  done

  echo >&2
  if [ ${ret} -eq 0 ]; then
    warning "Failed to stop netdata agent process with PID ${pid}."
    return 1
  fi

  echo >&2 "netdata on pid ${pid} stopped."
  return 0
}

netdata_pids() {
  myns="$(readlink /proc/self/ns/pid 2> /dev/null)"

  for p in \
    $(cat /var/run/netdata.pid 2> /dev/null) \
    $(cat /var/run/netdata/netdata.pid 2> /dev/null) \
    $(safe_pidof netdata 2> /dev/null); do
    ns="$(readlink "/proc/${p}/ns/pid" 2> /dev/null)"

    if [ -z "${myns}" ] || [ -z "${ns}" ] || [ "${myns}" = "${ns}" ]; then
      pidisnetdata "${p}" && echo "${p}"
    fi
  done
}

stop_all_netdata() {
  stop_success=0

  if [ -x "${NETDATA_PREFIX}/usr/libexec/netdata/install-service.sh" ]; then
    run_install_service_script --cmds-only
  fi

  if [ "${UID}" -eq 0 ]; then

    uname="$(uname 2>/dev/null)"

    # Any of these may fail, but we need to not bail if they do.
    if [ -n "${NETDATA_STOP_CMD}" ]; then
      if ${NETDATA_STOP_CMD}; then
        stop_success=1
      fi
    elif issystemd; then
      if systemctl stop netdata; then
        stop_success=1
      fi
    elif [ "${uname}" = "Darwin" ]; then
      if launchctl stop netdata; then
        stop_success=1
      fi
    elif [ "${uname}" = "FreeBSD" ]; then
      if /etc/rc.d/netdata stop; then
        stop_success=1
      fi
    else
      if service netdata stop; then
        stop_success=1
      fi
    fi
  fi

  if [ "${stop_success}" = "1" ]; then
    sleep 30

    if [ -n "$(netdata_pids)" ]; then
      stop_success=0
    fi
  fi

  if [ "$stop_success" = "0" ]; then
    if [ -n "$(netdata_pids)" ] && [ -n "$(command -v netdatacli)" ]; then
      for p in /tmp/netdata-ipc /run/netdata/netdata.pipe /var/run/netdata/netdata.pipe /tmp/netdata/netdata.pipe; do
        if [ -f "${p}" ]; then
          NETDATA_PIPENAME="${p}" netdatacli shutdown-agent && break
        fi
      done

      sleep 30
    fi

    for p in $(netdata_pids); do
      # shellcheck disable=SC2086
      stop_netdata_on_pid ${p}
    done
  fi
}

# -----------------------------------------------------------------------------
# restart netdata

restart_netdata() {
  netdata="${1}"
  shift

  started=0

  progress "Restarting netdata instance"

  if [ -x "${NETDATA_PREFIX}/usr/libexec/netdata/install-service.sh" ]; then
    run_install_service_script --cmds-only
  fi

  if [ -z "${NETDATA_INSTALLER_START_CMD}" ]; then
    if [ -n "${NETDATA_START_CMD}" ]; then
      NETDATA_INSTALLER_START_CMD="${NETDATA_START_CMD}"
    else
      NETDATA_INSTALLER_START_CMD="${netdata}"
    fi
  fi

  if [ "${UID}" -eq 0 ]; then
    echo >&2
    echo >&2 "Stopping all netdata threads"
    run stop_all_netdata

    echo >&2 "Starting netdata using command '${NETDATA_INSTALLER_START_CMD}'"
    # shellcheck disable=SC2086
    run ${NETDATA_INSTALLER_START_CMD} && started=1

    if [ ${started} -eq 1 ] && sleep 5 && [ -z "$(netdata_pids)" ]; then
      echo >&2 "Ooops! it seems netdata is not started."
      started=0
    fi

    if [ ${started} -eq 0 ]; then
      echo >&2 "Attempting another netdata start using command '${NETDATA_INSTALLER_START_CMD}'"
      # shellcheck disable=SC2086
      run ${NETDATA_INSTALLER_START_CMD} && started=1
    fi

    if [ ${started} -eq 1 ] && sleep 5 && [ -z "$(netdata_pids)" ]; then
      echo >&2 "Hm... it seems netdata is still not started."
      started=0
    fi
  fi


  if [ ${started} -eq 0 ]; then
    # still not started... another forced attempt, just run the binary
    warning "Netdata service still not started, attempting another forced restart by running '${netdata} ${*}'"
    run stop_all_netdata
    run "${netdata}" "${@}"
    return $?
  fi

  return 0
}

# -----------------------------------------------------------------------------
# install netdata logrotate

install_netdata_logrotate() {
  src="${NETDATA_PREFIX}/usr/lib/netdata/system/logrotate/netdata"

  if [ "${UID}" -eq 0 ]; then
    if [ -d /etc/logrotate.d ]; then
      if [ ! -f /etc/logrotate.d/netdata ]; then
        run cp "${src}" /etc/logrotate.d/netdata
      fi

      if [ -f /etc/logrotate.d/netdata ]; then
        run chmod 644 /etc/logrotate.d/netdata
      fi

      return 0
    fi
  fi

  return 1
}

# -----------------------------------------------------------------------------
# install netdata journald configuration

install_netdata_journald_conf() {
  src="${NETDATA_PREFIX}/usr/lib/netdata/system/systemd/journald@netdata.conf"

  [ ! -d /usr/lib/systemd/ ] && return 0
  [ "${UID}" -ne 0 ] && return 1

  if [ ! -d /usr/lib/systemd/journald@netdata.conf.d/ ]; then
    run mkdir /usr/lib/systemd/journald@netdata.conf.d/
  fi

  run cp "${src}" /usr/lib/systemd/journald@netdata.conf.d/netdata.conf

  if [ -f /usr/lib/systemd/journald@netdata.conf.d/netdata.conf ]; then
    run chmod 644 /usr/lib/systemd/journald@netdata.conf.d/netdata.conf
  fi

  return 0
}

# -----------------------------------------------------------------------------
# create netdata.conf

create_netdata_conf() {
  path="${1}"
  url="${2}"

  if [ -s "${path}" ]; then
    return 0
  fi

  if [ -n "$url" ]; then
    echo >&2 "Downloading default configuration from netdata..."
    sleep 5

    # remove a possibly obsolete configuration file
    [ -f "${path}.new" ] && rm "${path}.new"

    # disable a proxy to get data from the local netdata
    export http_proxy=
    export https_proxy=

    check_for_curl

    if [ -n "${curl}" ]; then
      run "${curl}" -sSL --connect-timeout 10 --retry 3 "${url}" > "${path}.new"
    elif command -v wget 1> /dev/null 2>&1; then
      run wget -T 15 -O - "${url}" > "${path}.new"
    fi

    if [ -s "${path}.new" ]; then
      run mv "${path}.new" "${path}"
      run_ok "New configuration saved for you to edit at ${path}"
    else
      [ -f "${path}.new" ] && rm "${path}.new"
      run_failed "Cannot download configuration from netdata daemon using url '${url}'"
      url=''
    fi
  fi

  if [ -z "$url" ]; then
    cat << EOF > "${path}"
# netdata can generate its own config which is available at 'http://<IP>:19999/netdata.conf'
# You can download it using:
#    curl -o ${path} http://localhost:19999/netdata.conf
# or
#    wget -O ${path} http://localhost:19999/netdata.conf
EOF
  fi

}

portable_add_user() {
  username="${1}"
  homedir="${2}"

  [ -z "${homedir}" ] && homedir="/tmp"

  # Check if user exists
  if command -v getent > /dev/null 2>&1; then
    if getent passwd "${username}" > /dev/null 2>&1; then
        echo >&2 "User '${username}' already exists."
        return 0
    fi
  elif command -v dscl > /dev/null 2>&1; then
    if dscl . read /Users/"${username}" >/dev/null 2>&1; then
      echo >&2 "User '${username}' already exists."
      return 0
    fi
  else
    if cut -d ':' -f 1 < /etc/passwd | grep "^${username}$" 1> /dev/null 2>&1; then
        echo >&2 "User '${username}' already exists."
        return 0
    fi
  fi

  echo >&2 "Adding ${username} user account with home ${homedir} ..."

  nologin="$(command -v nologin || echo '/bin/false')"

  if command -v useradd 1> /dev/null 2>&1; then
    run useradd -r -g "${username}" -c "${username}" -s "${nologin}" --no-create-home -d "${homedir}" "${username}" && return 0
  elif command -v pw 1> /dev/null 2>&1; then
    run pw useradd "${username}" -d "${homedir}" -g "${username}" -s "${nologin}" && return 0
  elif command -v adduser 1> /dev/null 2>&1; then
    run adduser -h "${homedir}" -s "${nologin}" -D -G "${username}" "${username}" && return 0
  elif command -v sysadminctl 1> /dev/null 2>&1; then
    gid=$(dscl . read /Groups/"${username}" 2>/dev/null | grep PrimaryGroupID | grep -Eo "[0-9]+")
    if run sysadminctl -addUser "${username}" -shell /usr/bin/false -home /var/empty -GID "$gid"; then
      # FIXME: I think the proper solution is to create a role account:
      # -roleAccount + name starting with _ and UID in 200-400 range.
      run dscl . create /Users/"${username}" IsHidden 1
      return 0
    fi
  fi

  warning "Failed to add ${username} user account!"

  return 1
}

portable_add_group() {
  groupname="${1}"

  # Check if group exist
  if get_group "${groupname}" > /dev/null 2>&1; then
    echo >&2 "Group '${groupname}' already exists."
    return 0
  fi

  echo >&2 "Adding ${groupname} user group ..."

  # Linux
  if command -v groupadd 1> /dev/null 2>&1; then
    run groupadd -r "${groupname}" && return 0
  elif command -v pw 1> /dev/null 2>&1; then
    run pw groupadd "${groupname}" && return 0
  elif command -v addgroup 1> /dev/null 2>&1; then
    run addgroup "${groupname}" && return 0
  elif command -v dseditgroup 1> /dev/null 2>&1; then
    dseditgroup -o create "${groupname}" && return 0
  fi

  warning >&2 "Failed to add ${groupname} user group !"
  return 1
}

portable_add_user_to_group() {
  groupname="${1}"
  username="${2}"

  # Check if group exist
  if ! get_group "${groupname}" > /dev/null 2>&1; then
    echo >&2 "Group '${groupname}' does not exist."
    # Don’t treat this as a failure, if the group does not exist we should not be trying to add the user to it.
    return 0
  fi

  # Check if user is in group
  if get_group "${groupname}" | cut -d ':' -f 4 | grep -wq "${username}"; then
    # username is already there
    echo >&2 "User '${username}' is already in group '${groupname}'."
    return 0
  else
    # username is not in group
    echo >&2 "Adding ${username} user to the ${groupname} group ..."

    # Linux
    if command -v usermod 1> /dev/null 2>&1; then
      run usermod -a -G "${groupname}" "${username}" && return 0
    elif command -v pw 1> /dev/null 2>&1; then
      run pw groupmod "${groupname}" -m "${username}" && return 0
    elif command -v addgroup 1> /dev/null 2>&1; then
      run addgroup "${username}" "${groupname}" && return 0
    elif command -v dseditgroup 1> /dev/null 2>&1; then
      dseditgroup -u "${username}" "${groupname}" && return 0
    fi

    warning >&2 "Failed to add user ${username} to group ${groupname}!"
    return 1
  fi
}

safe_sha256sum() {
  # Within the context of the installer, we only use -c option that is common between the two commands
  # We will have to reconsider if we start non-common options
  if command -v sha256sum > /dev/null 2>&1; then
    sha256sum "$@"
  elif command -v shasum > /dev/null 2>&1; then
    shasum -a 256 "$@"
  else
    fatal "I could not find a suitable checksum binary to use" "L0001"
  fi
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

install_netdata_updater() {
  if [ "${INSTALLER_DIR}" ] && [ -f "${INSTALLER_DIR}/packaging/installer/netdata-updater.sh" ]; then
    cat "${INSTALLER_DIR}/packaging/installer/netdata-updater.sh" > "${NETDATA_PREFIX}/usr/libexec/netdata/netdata-updater.sh" || return 1
  fi

  if [ "${NETDATA_SOURCE_DIR}" ] && [ -f "${NETDATA_SOURCE_DIR}/packaging/installer/netdata-updater.sh" ]; then
    cat "${NETDATA_SOURCE_DIR}/packaging/installer/netdata-updater.sh" > "${NETDATA_PREFIX}/usr/libexec/netdata/netdata-updater.sh" || return 1
  fi

  # these files are installed by cmake
  libsysdir="${NETDATA_PREFIX}/usr/lib/netdata/system/systemd/"
  if [ -d "${libsysdir}" ] && issystemd && [ -n "$(get_systemd_service_dir)" ]; then
    cat "${libsysdir}/netdata-updater.timer" > "$(get_systemd_service_dir)/netdata-updater.timer"
    cat "${libsysdir}/netdata-updater.service" > "$(get_systemd_service_dir)/netdata-updater.service"
  fi

  sed -i -e "s|THIS_SHOULD_BE_REPLACED_BY_INSTALLER_SCRIPT|${NETDATA_USER_CONFIG_DIR}/.environment|" "${NETDATA_PREFIX}/usr/libexec/netdata/netdata-updater.sh" || return 1

  chmod 0755 "${NETDATA_PREFIX}/usr/libexec/netdata/netdata-updater.sh"
  echo >&2 "Update script is located at ${TPUT_GREEN}${TPUT_BOLD}${NETDATA_PREFIX}/usr/libexec/netdata/netdata-updater.sh${TPUT_RESET}"
  echo >&2

  return 0
}

set_netdata_updater_channel() {
  sed -i -e "s/^RELEASE_CHANNEL=.*/RELEASE_CHANNEL=\"${RELEASE_CHANNEL}\"/" "${NETDATA_USER_CONFIG_DIR}/.environment"
}
