#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
#
# This is the netdata uninstaller script
#
# Variables needed by script and taken from '.environment' file:
#  - NETDATA_PREFIX
#  - NETDATA_ADDED_TO_GROUPS
#
# Next unused error code: R0005

usage="$(basename "$0") [-h] [-f ] -- program to calculate the answer to life, the universe and everything

where:
    -e, --env    path to environment file (defaults to '/etc/netdata/.environment'
    -f, --force  force uninstallation and do not ask any questions
    -h           show this help text
    -y, --yes    flag needs to be set to proceed with uninstallation"

FILE_REMOVAL_STATUS=0
ENVIRONMENT_FILE="/etc/netdata/.environment"
# shellcheck disable=SC2034
INTERACTIVITY="-i"
YES=0
while :; do
  case "$1" in
    -h | --help)
      echo "$usage" >&2
      exit 1
      ;;
    -f | --force)
      INTERACTIVITY="-f"
      shift
      ;;
    -y | --yes)
      YES=1
      FLAG=-y
      shift
      ;;
    -e | --env)
      ENVIRONMENT_FILE="$2"
      shift 2
      ;;
    -*)
      echo "$usage" >&2
      exit 1
      ;;
    *) break ;;
  esac
done

if [ -n "${script_source}" ]; then
  script_name="$(basename "${script_source}")"
else
  script_name="netdata-uninstaller.sh"
fi

info() {
  echo >&2 "$(date) : INFO: ${script_name}: " "${1}"
}

error() {
  echo >&2 "$(date) : ERROR: ${script_name}: " "${1}"
  if [ -n "${NETDATA_SAVE_WARNINGS}" ]; then
    NETDATA_WARNINGS="${NETDATA_WARNINGS}\n  - ${1}"
  fi
}

fatal() {
  echo >&2 "$(date) : FATAL: ${script_name}: FAILED TO UNINSTALL NETDATA: " "${1}"
  if [ -n "${NETDATA_SAVE_WARNINGS}" ]; then
    NETDATA_WARNINGS="${NETDATA_WARNINGS}\n  - ${1}"
  fi
  exit_reason "${1}" "${2}"
  exit 1
}

exit_reason() {
  if [ -n "${NETDATA_SAVE_WARNINGS}" ]; then
    EXIT_REASON="${1}"
    EXIT_CODE="${2}"
    if [ -n "${NETDATA_PROPAGATE_WARNINGS}" ]; then
      export EXIT_REASON
      export EXIT_CODE
      export NETDATA_WARNINGS
    fi
  fi
}

if [ "$YES" != "1" ]; then
  echo >&2 "This script will REMOVE netdata from your system."
  echo >&2 "Run it again with --yes to do it."
  exit_reason "User did not accept uninstalling." R0001
  exit 1
fi

if [ "$(id -u)" -ne 0 ]; then
  error "This script SHOULD be run as root or otherwise it won't delete all installed components."
  key="n"
  read -r 1 -p "Do you want to continue as non-root user [y/n] ? " key
  if [ "$key" != "y" ] && [ "$key" != "Y" ]; then
    exit_reason "User cancelled uninstall." R0002
    exit 1
  fi
fi

user_input() {
  if [ "${INTERACTIVITY}" = "-i" ]; then
    TEXT="$1 [y/n]"

    while true; do
      echo "$TEXT"
      read -r yn

      case "$yn" in
         [Yy]*) return 0;;
         [Nn]*) return 1;;
         *) echo "Please answer y[es] or n[o]";;
       esac
     done
  fi
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
        fatal "Unable to find a usable temporary directory. Please set \$TMPDIR to a path that is both writable and allows execution of files and try again." R0003
      else
        TMPDIR="${PWD}"
      fi
    else
      TMPDIR="/tmp"
    fi
  fi

  mktemp -d -t netdata-uninstaller-XXXXXXXXXX
}

tmpdir="$(create_tmp_directory)"

detect_existing_install() {
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
    envfile="${ndprefix}/etc/netdata/.environment"
    if [ -r "${typefile}" ]; then
      ${ROOTCMD} sh -c "cat \"${typefile}\" > \"${tmpdir}/install-type\""
      # shellcheck disable=SC1090,SC1091
      . "${tmpdir}/install-type"
    else
      INSTALL_TYPE="unknown"
    fi

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

pkg_installed() {
  case "${DISTRO_COMPAT_NAME}" in
    debian|ubuntu)
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

disable_updater() {
  if [ -x "${NETDATA_PREFIX}/usr/libexec/netdata-updater.sh" ]; then
    "${NETDATA_PREFIX}/usr/libexec/netdata-updater.sh" --disable-auto-updates
  else
    rm_file /etc/periodic/daily/netdata-updater
    rm_file /etc/cron.daily/netdata-updater
    rm_file /etc/cron.d/netdata-updater
    rm_file /etc/cron.d/netdata-updater-daily

    if command -v systemctl >/dev/null 2>&1 ; then
      systemctl stop netdata-updater.timer >/dev/null 2>&1 || true
      systemctl disable netdata-updater.timer >/dev/null 2>&1 || true
    fi
  fi
}

cleanup_data_and_config() {
  rm_dir "${NETDATA_PREFIX}/var/lib/netdata"
  rm_dir "${NETDATA_PREFIX}/var/cache/netdata"
  rm_dir "${NETDATA_PREFIX}/var/log/netdata"
  rm_dir "${NETDATA_PREFIX}/etc/netdata"
}

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

  if command -v tput 1> /dev/null 2>&1; then
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
    printf "%s" " ... "
  } >> "${run_logfile}"

  printf "%s" "${info_console}${TPUT_BOLD}${TPUT_YELLOW}" >&2
  escaped_print >&2 "${@}"
  printf "%s\n" "${TPUT_RESET}" >&2

  "${@}"

  ret=$?
  if [ ${ret} -ne 0 ]; then
    printf >&2 "%s FAILED %s\n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD}" "${TPUT_RESET}"
    printf >> "${run_logfile}" "FAILED with exit code %s\n" "${ret}"
    NETDATA_WARNINGS="${NETDATA_WARNINGS}\n  - Command \"${*}\" failed with exit code ${ret}."
  else
    printf >&2 "%s OK %s\n\n" "${TPUT_BGGREEN}${TPUT_WHITE}${TPUT_BOLD}" "${TPUT_RESET}"
    printf >> "${run_logfile}" "OK\n"
  fi

  return ${ret}
}

rm_file() {
  FILE="$1"
  if [ -f "${FILE}" ]; then
    if user_input "Do you want to delete this file '$FILE' ? "; then
      run rm -v "${FILE}"
    fi
  fi
}

rm_dir() {
  DIR="$1"
  if [ -n "$DIR" ] && [ -d "$DIR" ]; then
    if user_input "Do you want to delete this directory '$DIR' ? "; then
      run rm -v -f -R "${DIR}"
    fi
  fi
}

detect_existing_install
disable_updater

if [ -x "$(command -v apt-get)" ] && [ "${INSTALL_TYPE}" = "binpkg-deb" ]; then
  if dpkg -s netdata > /dev/null; then
    echo "Found netdata native installation"
    if user_input "Do you want to remove netdata? "; then
      # shellcheck disable=SC2086
      apt-get remove netdata ${FLAG}
    fi
    if dpkg -s netdata-repo-edge > /dev/null; then
      if user_input "Do you want to remove netdata-repo-edge? "; then
        # shellcheck disable=SC2086
        apt-get remove netdata-repo-edge ${FLAG}
      fi
    fi
    if dpkg -s netdata-repo > /dev/null; then
      if user_input "Do you want to remove netdata-repo? "; then
        # shellcheck disable=SC2086
        apt-get remove netdata-repo ${FLAG}
      fi
    fi
    cleanup_data_and_config
    exit 0
  fi
elif [ -x "$(command -v dnf)" ] && [ "${INSTALL_TYPE}" = "binpkg-rpm" ]; then
  if rpm -q netdata > /dev/null; then
    echo "Found netdata native installation."
    if user_input "Do you want to remove netdata? "; then
      # shellcheck disable=SC2086
      dnf remove netdata ${FLAG}
    fi
    if rpm -q netdata-repo-edge > /dev/null; then
      if user_input "Do you want to remove netdata-repo-edge? "; then
        # shellcheck disable=SC2086
        dnf remove netdata-repo-edge ${FLAG}
      fi
    fi
    if rpm -q netdata-repo > /dev/null; then
      if user_input "Do you want to remove netdata-repo? "; then
        # shellcheck disable=SC2086
        dnf remove netdata-repo ${FLAG}
      fi
    fi
    cleanup_data_and_config
    exit 0
  fi
elif [ -x "$(command -v yum)" ] && [ "${INSTALL_TYPE}" = "binpkg-rpm" ]; then
  if rpm -q netdata > /dev/null; then
    echo "Found netdata native installation."
    if user_input "Do you want to remove netdata? "; then
      # shellcheck disable=SC2086
      yum remove netdata ${FLAG}
    fi
    if rpm -q netdata-repo-edge > /dev/null; then
      if user_input "Do you want to remove netdata-repo-edge? "; then
        # shellcheck disable=SC2086
        yum remove netdata-repo-edge ${FLAG}
      fi
    fi
    if rpm -q netdata-repo > /dev/null; then
      if user_input "Do you want to remove netdata-repo? "; then
        # shellcheck disable=SC2086
        yum remove netdata-repo ${FLAG}
      fi
    fi
    cleanup_data_and_config
    exit 0
  fi
elif [ -x "$(command -v zypper)" ] && [ "${INSTALL_TYPE}" = "binpkg-rpm" ]; then
  if [ "${FLAG}" = "-y" ]; then
    FLAG=-n
  fi
  if zypper search -i netdata > /dev/null; then
    echo "Found netdata native installation."
    if user_input "Do you want to remove netdata? "; then
      # shellcheck disable=SC2086
      zypper ${FLAG} remove netdata
    fi
    if zypper search -i netdata-repo-edge > /dev/null; then
      if user_input "Do you want to remove netdata-repo-edge? "; then
        # shellcheck disable=SC2086
        zypper ${FLAG} remove netdata-repo-edge
      fi
    fi
    if zypper search -i netdata-repo > /dev/null; then
      if user_input "Do you want to remove netdata-repo? "; then
        # shellcheck disable=SC2086
        zypper ${FLAG} remove netdata-repo
      fi
    fi
    cleanup_data_and_config
    exit 0
  fi
fi

# -----------------------------------------------------------------------------
# portable service command

service_cmd="$(command -v service 2> /dev/null)"
rcservice_cmd="$(command -v rc-service 2> /dev/null)"
systemctl_cmd="$(command -v systemctl 2> /dev/null)"
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

portable_del_group() {
  groupname="${1}"

  # Check if group exist
  info "Removing ${groupname} user group ..."

  # Linux
  if command -v groupdel 1> /dev/null 2>&1; then
    if get_group "${groupname}" > /dev/null 2>&1; then
      run groupdel "${groupname}" && return 0
    else
      info "Group ${groupname} already removed in a previous step."
      return 0
    fi
  fi

  # mac OS
  if command -v dseditgroup 1> /dev/null 2>&1; then
    if dseditgroup -o read netdata 1> /dev/null 2>&1; then
      run dseditgroup -o delete "${groupname}" && return 0
    else
      info "Could not find group ${groupname}, nothing to do"
      return 0
    fi
  fi

  # DMS ( Synology )
  if command -v synogroup 1> /dev/null 2>&1; then
    if get_group "${groupname}" > /dev/null 2>&1; then
      run synogroup --del "${groupname}" && return 0
    else
      info "Could not find group ${groupname}, nothing to do"
      return 0
    fi;
  fi

  error "Group ${groupname} was not automatically removed, you might have to remove it manually"
  return 1
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

portable_del_user() {
  username="${1}"
  info "Deleting ${username} user account ..."

  # Linux
  if command -v userdel 1> /dev/null 2>&1; then
    run userdel -f "${username}" && return 0
  fi

  # mac OS
  if command -v sysadminctl 1> /dev/null 2>&1; then
    run sysadminctl -deleteUser "${username}" && return 0
  fi

  # DMS ( Synology )
  if command -v synouser 1> /dev/null 2>&1; then
    run synouser --del "${username}" && return 0
  fi

  error "User ${username} could not be deleted from system, you might have to remove it manually"
  return 1
}

portable_del_user_from_group() {
  groupname="${1}"
  username="${2}"

  if command -v getent > /dev/null 2>&1; then
    getent group "${1:-""}" | grep -q "${2}"
  else
    grep "^${1}:" /etc/group | grep -q "${2}"
  fi

  ret=$?
  [ "${ret}" != "0" ] && return 0

  # username is not in group
  info "Deleting ${username} user from ${groupname} group ..."

  # Linux
  if command -v gpasswd 1> /dev/null 2>&1; then
    run gpasswd -d "netdata" "${group}" && return 0
  fi

  # FreeBSD
  if command -v pw 1> /dev/null 2>&1; then
    run pw groupmod "${groupname}" -d "${username}" && return 0
  fi

  # BusyBox
  if command -v delgroup 1> /dev/null 2>&1; then
    run delgroup "${username}" "${groupname}" && return 0
  fi

  # mac OS
  if command -v dseditgroup 1> /dev/null 2>&1; then
    run dseditgroup -o delete -u "${username}" "${groupname}" && return 0
  fi

  # DSM ( Synology )
  if command -v synogroup 1> /dev/null 2>&1; then
    # Get current members of the group removing username
    current_members_list=$(synogroup --get "${groupname}" | grep -v "\[${username}\]" | grep '^[0-9]' )
    current_members=$(echo "${current_members_list}" | grep -oP '\[\K[^\]]+' | tr '\n' ' ' | sed 's/ $//')

    # Set the new list of members
    # shellcheck disable=SC2086
    run synogroup --member "${groupname}" ${current_members} && return 0
  fi

  error "Failed to delete user ${username} from group ${groupname} !"
  return 1
}

quit_msg() {
  echo
  if [ "$FILE_REMOVAL_STATUS" -eq 0 ]; then
    fatal "Failed to completely remove Netdata from this system." R0004
  else
    info "Netdata files were successfully removed from your system"
  fi
}

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

  info "Stopping netdata on pid ${pid} ..."
  while [ -n "$pid" ] && [ ${ret} -eq 0 ]; do
    if [ ${count} -gt 24 ]; then
      error "Cannot stop the running netdata on pid ${pid}."
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
    error "SORRY! CANNOT STOP netdata ON PID ${pid} !"
    return 1
  fi

  info "netdata on pid ${pid} stopped."
  return 0
}

netdata_pids() {
  p=''
  ns=''

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
  p=''
  stop_success=0

  if [ "$(id -u)" -eq 0 ]; then
    uname="$(uname 2> /dev/null)"

    # Any of these may fail, but we need to not bail if they do.
    if issystemd; then
      if systemctl stop netdata; then
        stop_success=1
        sleep 5
      fi
    elif [ "${uname}" = "Darwin" ]; then
      if launchctl stop netdata; then
        stop_success=1
        sleep 5
      fi
    elif [ "${uname}" = "FreeBSD" ]; then
      if /etc/rc.d/netdata stop; then
        stop_success=1
        sleep 5
      fi
    else
      if service netdata stop; then
        stop_success=1
        sleep 5
      fi
    fi
  fi

  if [ "$stop_success" = "0" ]; then
    if [ -n "$(netdata_pids)" ] && [ -n "$(command -v netdatacli)" ]; then
      netdatacli shutdown-agent
      sleep 20
    fi

    for p in $(netdata_pids); do
      # shellcheck disable=SC2086
      stop_netdata_on_pid ${p}
    done
  fi
}

trap quit_msg EXIT

# shellcheck source=/dev/null
# shellcheck disable=SC1090
. "${ENVIRONMENT_FILE}" || exit 1

#### STOP NETDATA
info "Stopping a possibly running netdata..."
stop_all_netdata

if [ "$(uname -s)" = "Darwin" ]; then
  launchctl unload /Library/LaunchDaemons/com.github.netdata.plist 2>/dev/null
fi

#### REMOVE NETDATA FILES

if issystemd; then
  for unit in netdata.service netdata-updater.timer; do
    systemctl disable "${unit}"
    systemctl stop "${unit}"
  done

  for unit in netdata.service netdata-updater.service netdata-updater.timer; do
    unit_path="$(systemctl show -p FragmentPath "${unit}" | cut -f 2- -d '=')"
    override_paths="$(systemctl show -p DropInPaths "${unit}" | cut -f 2- -d '=')"
    for path in "${unit_path}" ${override_paths} ; do
      rm_file "${path}"
    done
  done

  rm_file /usr/lib/systemd/journald@netdata.conf.d/netdata.conf
  rm_file /lib/systemd/journald@netdata.conf.d/netdata.conf
  rm_dir /usr/lib/systemd/journald@netdata.conf.d/
  rm_file /usr/lib/systemd/system-preset/50-netdata.preset
  rm_file /lib/systemd/system-preset/50-netdata.preset

  systemctl daemon-reload
fi

rm_file /etc/logrotate.d/netdata
rm_file /etc/init.d/netdata
rm_file /Library/LaunchDaemons/com.github.netdata.plist

if [ -n "${NETDATA_PREFIX}" ] && [ -d "${NETDATA_PREFIX}" ] && [ "netdata" = "$(basename "$NETDATA_PREFIX")" ] ; then
  rm_dir "${NETDATA_PREFIX}"
else
  rm_file "${NETDATA_PREFIX}/usr/sbin/netdata"
  rm_file "${NETDATA_PREFIX}/usr/sbin/netdatacli"
  rm_file "${NETDATA_PREFIX}/usr/sbin/netdata-claim.sh"
  rm_file "${NETDATA_PREFIX}/usr/sbin/log2journal"
  rm_file "${NETDATA_PREFIX}/usr/sbin/systemd-cat-native"
  rm_file "/tmp/netdata-ipc"
  rm_file "/tmp/netdata-service-cmds"
  rm_dir "${NETDATA_PREFIX}/usr/share/netdata"
  rm_dir "${NETDATA_PREFIX}/usr/libexec/netdata"
  cleanup_data_and_config
fi

if [ -n "${tmpdir}" ]; then
  run rm -rf "${tmpdir}" || true
fi

FILE_REMOVAL_STATUS=1

#### REMOVE USER
if user_input "Do you want to delete 'netdata' system user ? "; then
  portable_del_user "netdata" || :
elif [ -n "$NETDATA_ADDED_TO_GROUPS" ]; then
  if user_input "Do you want to delete 'netdata' from following groups: '$NETDATA_ADDED_TO_GROUPS' ? "; then
    for group in $NETDATA_ADDED_TO_GROUPS; do
      portable_del_user_from_group "${group}" "netdata"
    done
  fi
fi

### REMOVE GROUP
if user_input "Do you want to delete 'netdata' system group ? "; then
  portable_del_group "netdata" || :
fi
