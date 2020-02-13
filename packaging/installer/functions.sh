#!/bin/bash

# SPDX-License-Identifier: GPL-3.0-or-later

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
      TPUT_RED="$(tput setaf 1)"
      TPUT_GREEN="$(tput setaf 2)"
      # shellcheck disable=SC2034
      TPUT_YELLOW="$(tput setaf 3)"
      # shellcheck disable=SC2034
      TPUT_BLUE="$(tput setaf 4)"
      # shellcheck disable=SC2034
      TPUT_PURPLE="$(tput setaf 5)"
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

get() {
  url="${1}"
  if command -v curl > /dev/null 2>&1; then
    curl -sSL -o - --connect-timeout 10 --retry 3 "${url}"
  elif command -v wget > /dev/null 2>&1; then
    wget -T 15 -O - "${url}"
  else
    fatal "I need curl or wget to proceed, but neither is available on this system."
  fi
}

# -----------------------------------------------------------------------------

netdata_banner() {
  local l1="  ^" \
    l2="  |.-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-" \
    l3="  |   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'  " \
    l4="  +----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+--->" \
    sp="                                                                              " \
    netdata="netdata" start end msg="${*}" chartcolor="${TPUT_DIM}"

  [ ${#msg} -lt ${#netdata} ] && msg="${msg}${sp:0:$((${#netdata} - ${#msg}))}"
  [ ${#msg} -gt $((${#l2} - 20)) ] && msg="${msg:0:$((${#l2} - 23))}..."

  start="$((${#l2} / 2 - 4))"
  [ $((start + ${#msg} + 4)) -gt ${#l2} ] && start=$((${#l2} - ${#msg} - 4))
  end=$((start + ${#msg} + 4))

  echo >&2
  echo >&2 "${chartcolor}${l1}${TPUT_RESET}"
  echo >&2 "${chartcolor}${l2:0:start}${sp:0:2}${TPUT_RESET}${TPUT_BOLD}${TPUT_GREEN}${netdata}${TPUT_RESET}${chartcolor}${sp:0:$((end - start - 2 - ${#netdata}))}${l2:end:$((${#l2} - end))}${TPUT_RESET}"
  echo >&2 "${chartcolor}${l3:0:start}${sp:0:2}${TPUT_RESET}${TPUT_BOLD}${TPUT_CYAN}${msg}${TPUT_RESET}${chartcolor}${sp:0:2}${l3:end:$((${#l2} - end))}${TPUT_RESET}"
  echo >&2 "${chartcolor}${l4}${TPUT_RESET}"
  echo >&2
}

# -----------------------------------------------------------------------------
# portable service command

service_cmd="$(command -v service 2> /dev/null)"
rcservice_cmd="$(command -v rc-service 2> /dev/null)"
systemctl_cmd="$(command -v systemctl 2> /dev/null)"
service() {

  local cmd="${1}" action="${2}"

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
  local pidof_cmd
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
  local cpus
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
fatal() {
  printf >&2 "%s ABORTED %s %s \n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD}" "${TPUT_RESET}" "${*}"
  exit 1
}

run_ok() {
  printf >&2 "%s OK %s %s \n\n" "${TPUT_BGGREEN}${TPUT_WHITE}${TPUT_BOLD}" "${TPUT_RESET}" "${*}"
}

run_failed() {
  printf >&2 "%s FAILED %s %s \n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD}" "${TPUT_RESET}" "${*}"
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
    printf "%s" "${info}"
    escaped_print "${@}"
    printf "%s" " ... "
  } >> "${run_logfile}"

  printf >&2 "%s" "${info_console}${TPUT_BOLD}${TPUT_YELLOW}"
  escaped_print >&2 "${@}"
  printf >&2 "%s" "${TPUT_RESET}\n"

  "${@}"

  local ret=$?
  if [ ${ret} -ne 0 ]; then
    run_failed
    printf >> "${run_logfile}" "FAILED with exit code %s\n" "${ret}"
  else
    run_ok
    printf >> "${run_logfile}" "OK\n"
  fi

  return ${ret}
}

iscontainer() {
  # man systemd-detect-virt
  local cmd
  cmd=$(command -v systemd-detect-virt 2> /dev/null)
  if [ -n "${cmd}" ] && [ -x "${cmd}" ]; then
    "${cmd}" --container > /dev/null 2>&1 && return 0
  fi

  # /proc/1/sched exposes the host's pid of our init !
  # http://stackoverflow.com/a/37016302
  local pid
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

issystemd() {
  local pids p myns ns systemctl

  # if the directory /lib/systemd/system OR /usr/lib/systemd/system (SLES 12.x) does not exit, it is not systemd
  if [ ! -d /lib/systemd/system ] && [ ! -d /usr/lib/systemd/system ]; then
    return 1
  fi

  # if there is no systemctl command, it is not systemd
  systemctl=$(command -v systemctl 2> /dev/null)
  if [ -z "${systemctl}" ] || [ ! -x "${systemctl}" ]; then
    return 1
  fi

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

install_non_systemd_init() {
  [ "${UID}" != 0 ] && return 1

  local key="unknown"
  if [ -f /etc/os-release ]; then
    # shellcheck disable=SC1091
    source /etc/os-release || return 1
    key="${ID}-${VERSION_ID}"

  elif [ -f /etc/redhat-release ]; then
    key=$(< /etc/redhat-release)
  fi

  if [ -d /etc/init.d ] && [ ! -f /etc/init.d/netdata ]; then
    if [[ ${key} =~ ^(gentoo|alpine).* ]]; then
      echo >&2 "Installing OpenRC init file..."
      run cp system/netdata-openrc /etc/init.d/netdata &&
        run chmod 755 /etc/init.d/netdata &&
        run rc-update add netdata default &&
        return 0

    elif [[ ${key} =~ ^devuan* ]] || [ "${key}" = "debian-7" ] || [ "${key}" = "ubuntu-12.04" ] || [ "${key}" = "ubuntu-14.04" ]; then
      echo >&2 "Installing LSB init file..."
      run cp system/netdata-lsb /etc/init.d/netdata &&
        run chmod 755 /etc/init.d/netdata &&
        run update-rc.d netdata defaults &&
        run update-rc.d netdata enable &&
        return 0
    elif [[ ${key} =~ ^(amzn-201[5678]|ol|CentOS release 6|Red Hat Enterprise Linux Server release 6|Scientific Linux CERN SLC release 6|CloudLinux Server release 6).* ]]; then
      echo >&2 "Installing init.d file..."
      run cp system/netdata-init-d /etc/init.d/netdata &&
        run chmod 755 /etc/init.d/netdata &&
        run chkconfig netdata on &&
        return 0
    else
      echo >&2 "I don't know what init file to install on system '${key}'. Open a github issue to help us fix it."
      return 1
    fi
  elif [ -f /etc/init.d/netdata ]; then
    echo >&2 "file '/etc/init.d/netdata' already exists."
    return 0
  else
    echo >&2 "I don't know what init file to install on system '${key}'. Open a github issue to help us fix it."
  fi

  return 1
}

NETDATA_START_CMD="netdata"
NETDATA_INSTALLER_START_CMD=""

install_netdata_service() {
  local uname
  uname="$(uname 2> /dev/null)"

  if [ "${UID}" -eq 0 ]; then
    if [ "${uname}" = "Darwin" ]; then

      if [ -f "/Library/LaunchDaemons/com.github.netdata.plist" ]; then
        echo >&2 "file '/Library/LaunchDaemons/com.github.netdata.plist' already exists."
        return 0
      else
        echo >&2 "Installing MacOS X plist file..."
        run cp system/netdata.plist /Library/LaunchDaemons/com.github.netdata.plist &&
          run launchctl load /Library/LaunchDaemons/com.github.netdata.plist &&
          return 0
      fi

    elif [ "${uname}" = "FreeBSD" ]; then

      run cp system/netdata-freebsd /etc/rc.d/netdata && NETDATA_START_CMD="service netdata start" &&
        NETDATA_INSTALLER_START_CMD="service netdata onestart" &&
        myret=$?

      echo >&2 "Note: To explicitly enable netdata automatic start, set 'netdata_enable' to 'YES' in /etc/rc.conf"
      echo >&2 ""

      return ${myret}

    elif issystemd; then
      # systemd is running on this system
      NETDATA_START_CMD="systemctl start netdata"
      NETDATA_INSTALLER_START_CMD="${NETDATA_START_CMD}"

      SYSTEMD_DIRECTORY=""

      if [ -w "/lib/systemd/system" ]; then
        SYSTEMD_DIRECTORY="/lib/systemd/system"
      elif [ -w "/usr/lib/systemd/system" ]; then
        SYSTEMD_DIRECTORY="/usr/lib/systemd/system"
      elif [ -w "/etc/systemd/system" ]; then
        SYSTEMD_DIRECTORY="/etc/systemd/system"
      fi

      if [ "${SYSTEMD_DIRECTORY}x" != "x" ]; then
        ENABLE_NETDATA_IF_PREVIOUSLY_ENABLED="run systemctl enable netdata"
        IS_NETDATA_ENABLED="$(systemctl is-enabled netdata 2> /dev/null || echo "Netdata not there")"
        if [ "${IS_NETDATA_ENABLED}" == "disabled" ]; then
          echo >&2 "Netdata was there and disabled, make sure we don't re-enable it ourselves"
          ENABLE_NETDATA_IF_PREVIOUSLY_ENABLED="true"
        fi

        echo >&2 "Installing systemd service..."
        run cp system/netdata.service "${SYSTEMD_DIRECTORY}/netdata.service" &&
          run systemctl daemon-reload &&
          ${ENABLE_NETDATA_IF_PREVIOUSLY_ENABLED} &&
          return 0
      else
        echo >&2 "no systemd directory; cannot install netdata.service"
      fi
    else
      install_non_systemd_init
      local ret=$?

      if [ ${ret} -eq 0 ]; then
        if [ -n "${service_cmd}" ]; then
          NETDATA_START_CMD="service netdata start"
        elif [ -n "${rcservice_cmd}" ]; then
          NETDATA_START_CMD="rc-service netdata start"
        fi
        NETDATA_INSTALLER_START_CMD="${NETDATA_START_CMD}"
      fi

      return ${ret}
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
  local pid="${1}" ret=0 count=0

  pidisnetdata "${pid}" || return 0

  printf >&2 "Stopping netdata on pid %s ..." "${pid}"
  while [ -n "$pid" ] && [ ${ret} -eq 0 ]; do
    if [ ${count} -gt 24 ]; then
      echo >&2 "Cannot stop the running netdata on pid ${pid}."
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
    echo >&2 "SORRY! CANNOT STOP netdata ON PID ${pid} !"
    return 1
  fi

  echo >&2 "netdata on pid ${pid} stopped."
  return 0
}

netdata_pids() {
  local p myns ns

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
  local p uname

  if [ "${UID}" -eq 0 ]; then
    uname="$(uname 2> /dev/null)"

    # Any of these may fail, but we need to not bail if they do.
    if issystemd; then
      if systemctl stop netdata; then
        sleep 5
      fi
    elif [ "${uname}" = "Darwin" ]; then
      if launchctl stop netdata; then
        sleep 5
      fi
    elif [ "${uname}" = "FreeBSD" ]; then
      if /etc/rc.d/netdata stop; then
        sleep 5
      fi
    else
      if service netdata stop; then
        sleep 5
      fi
    fi
  fi

  if [ -n "$(netdata_pids)" ] || [ -n "$(builtin type -P netdatacli)" ]; then
    netdatacli shutdown-agent
    sleep 20
  fi

  for p in $(netdata_pids); do
    # shellcheck disable=SC2086
    stop_netdata_on_pid ${p}
  done
}

# -----------------------------------------------------------------------------
# restart netdata

restart_netdata() {
  local netdata="${1}"
  shift

  local started=0

  progress "Restarting netdata instance"

  if [ -z "${NETDATA_INSTALLER_START_CMD}" ]; then
    NETDATA_INSTALLER_START_CMD="${netdata}"
  fi

  if [ "${UID}" -eq 0 ]; then
    echo >&2
    echo >&2 "Stopping all netdata threads"
    run stop_all_netdata

    echo >&2 "Starting netdata using command '${NETDATA_INSTALLER_START_CMD}'"
    # shellcheck disable=SC2086
    run ${NETDATA_INSTALLER_START_CMD} && started=1

    if [ ${started} -eq 1 ] && [ -z "$(netdata_pids)" ]; then
      echo >&2 "Ooops! it seems netdata is not started."
      started=0
    fi

    if [ ${started} -eq 0 ]; then
      echo >&2 "Attempting another netdata start using command '${NETDATA_INSTALLER_START_CMD}'"
      # shellcheck disable=SC2086
      run ${NETDATA_INSTALLER_START_CMD} && started=1
    fi
  fi

  if [ ${started} -eq 1 ] && [ -z "$(netdata_pids)" ]; then
    echo >&2 "Hm... it seems netdata is still not started."
    started=0
  fi

  if [ ${started} -eq 0 ]; then
    # still not started... another forced attempt, just run the binary
    echo >&2 "Netdata service still not started, attempting another forced restart by running '${netdata} ${*}'"
    run stop_all_netdata
    run "${netdata}" "${@}"
    return $?
  fi

  return 0
}

# -----------------------------------------------------------------------------
# install netdata logrotate

install_netdata_logrotate() {
  if [ "${UID}" -eq 0 ]; then
    if [ -d /etc/logrotate.d ]; then
      if [ ! -f /etc/logrotate.d/netdata ]; then
        run cp system/netdata.logrotate /etc/logrotate.d/netdata
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
# create netdata.conf

create_netdata_conf() {
  local path="${1}" url="${2}"

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

    if command -v curl 1> /dev/null 2>&1; then
      run curl -sSL --connect-timeout 10 --retry 3 "${url}" > "${path}.new"
    elif command -v wget 1> /dev/null 2>&1; then
      run wget -T 15 -O - "${url}" > "${path}.new"
    fi

    if [ -s "${path}.new" ]; then
      run mv "${path}.new" "${path}"
      run_ok "New configuration saved for you to edit at ${path}"
    else
      [ -f "${path}.new" ] && rm "${path}.new"
      run_failed "Cannnot download configuration from netdata daemon using url '${url}'"
      url=''
    fi
  fi

  if [ -z "$url" ]; then
    echo "# netdata can generate its own config which is available at 'http://<netdata_ip>/netdata.conf'" > "${path}"
    echo "# You can download it with command like: 'wget -O ${path} http://localhost:19999/netdata.conf'" >> "${path}"
  fi

}

portable_add_user() {
  local username="${1}" homedir="${2}"

  [ -z "${homedir}" ] && homedir="/tmp"

  # Check if user exists
  if cut -d ':' -f 1 < /etc/passwd | grep "^${username}$" 1> /dev/null 2>&1; then
    echo >&2 "User '${username}' already exists."
    return 0
  fi

  echo >&2 "Adding ${username} user account with home ${homedir} ..."

  local nologin
  nologin="$(command -v nologin || echo '/bin/false')"

  # Linux
  if command -v useradd 1> /dev/null 2>&1; then
    run useradd -r -g "${username}" -c "${username}" -s "${nologin}" --no-create-home -d "${homedir}" "${username}" && return 0
  fi

  # FreeBSD
  if command -v pw 1> /dev/null 2>&1; then
    run pw useradd "${username}" -d "${homedir}" -g "${username}" -s "${nologin}" && return 0
  fi

  # BusyBox
  if command -v adduser 1> /dev/null 2>&1; then
    run adduser -h "${homedir}" -s "${nologin}" -D -G "${username}" "${username}" && return 0
  fi

  # mac OS
  if command -v sysadminctl 1> /dev/null 2>&1; then
    run sysadminctl -addUser "${username}" && return 0
  fi

  echo >&2 "Failed to add ${username} user account !"

  return 1
}

portable_add_group() {
  local groupname="${1}"

  # Check if group exist
  if cut -d ':' -f 1 < /etc/group | grep "^${groupname}$" 1> /dev/null 2>&1; then
    echo >&2 "Group '${groupname}' already exists."
    return 0
  fi

  echo >&2 "Adding ${groupname} user group ..."

  # Linux
  if command -v groupadd 1> /dev/null 2>&1; then
    run groupadd -r "${groupname}" && return 0
  fi

  # FreeBSD
  if command -v pw 1> /dev/null 2>&1; then
    run pw groupadd "${groupname}" && return 0
  fi

  # BusyBox
  if command -v addgroup 1> /dev/null 2>&1; then
    run addgroup "${groupname}" && return 0
  fi

  # mac OS
  if command -v dseditgroup 1> /dev/null 2>&1; then
    dseditgroup -o create "${groupname}" && return 0
  fi

  echo >&2 "Failed to add ${groupname} user group !"
  return 1
}

portable_add_user_to_group() {
  local groupname="${1}" username="${2}"

  # Check if group exist
  if ! cut -d ':' -f 1 < /etc/group | grep "^${groupname}$" > /dev/null 2>&1; then
    echo >&2 "Group '${groupname}' does not exist."
    return 1
  fi

  # Check if user is in group
  if [[ ",$(grep "^${groupname}:" < /etc/group | cut -d ':' -f 4)," =~ ,${username}, ]]; then
    # username is already there
    echo >&2 "User '${username}' is already in group '${groupname}'."
    return 0
  else
    # username is not in group
    echo >&2 "Adding ${username} user to the ${groupname} group ..."

    # Linux
    if command -v usermod 1> /dev/null 2>&1; then
      run usermod -a -G "${groupname}" "${username}" && return 0
    fi

    # FreeBSD
    if command -v pw 1> /dev/null 2>&1; then
      run pw groupmod "${groupname}" -m "${username}" && return 0
    fi

    # BusyBox
    if command -v addgroup 1> /dev/null 2>&1; then
      run addgroup "${username}" "${groupname}" && return 0
    fi

    # mac OS
    if command -v dseditgroup 1> /dev/null 2>&1; then
      dseditgroup -u "${username}" "${groupname}" && return 0
    fi
    echo >&2 "Failed to add user ${username} to group ${groupname} !"
    return 1
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

_get_crondir() {
  if [ -d /etc/cron.daily ]; then
    echo /etc/cron.daily
  elif [ -d /etc/periodic/daily ]; then
    echo /etc/periodic/daily
  else
    echo >&2 "Cannot figure out the cron directory to handle netdata-updater.sh activation/deactivation"
    return 1
  fi

  return 0
}

_check_crondir_permissions() {
  if [ "${UID}" -ne "0" ]; then
    # We cant touch cron if we are not running as root
    echo >&2 "You need to run the installer as root for auto-updating via cron"
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

  sed -i -e "s|THIS_SHOULD_BE_REPLACED_BY_INSTALLER_SCRIPT|${NETDATA_USER_CONFIG_DIR}/.environment|" "${NETDATA_PREFIX}/usr/libexec/netdata/netdata-updater.sh" || return 1

  chmod 0755 "${NETDATA_PREFIX}/usr/libexec/netdata/netdata-updater.sh"
  echo >&2 "Update script is located at ${TPUT_GREEN}${TPUT_BOLD}${NETDATA_PREFIX}/usr/libexec/netdata/netdata-updater.sh${TPUT_RESET}"
  echo >&2

  return 0
}

cleanup_old_netdata_updater() {
  if [ -f "${NETDATA_PREFIX}"/usr/libexec/netdata-updater.sh ]; then
    echo >&2 "Removing updater from deprecated location"
    rm -f "${NETDATA_PREFIX}"/usr/libexec/netdata-updater.sh
  fi

  crondir="$(_get_crondir)" || return 1
  _check_crondir_permissions "${crondir}" || return 1

  if [ -f "${crondir}/netdata-updater.sh" ]; then
    echo >&2 "Removing incorrect netdata-updater filename in cron"
    rm -f "${crondir}/netdata-updater.sh"
  fi

  return 0
}

enable_netdata_updater() {
  crondir="$(_get_crondir)" || return 1
  _check_crondir_permissions "${crondir}" || return 1

  echo >&2 "Adding to cron"

  rm -f "${crondir}/netdata-updater"
  ln -sf "${NETDATA_PREFIX}/usr/libexec/netdata/netdata-updater.sh" "${crondir}/netdata-updater"

  echo >&2 "Auto-updating has been enabled. Updater script linked to: ${TPUT_RED}${TPUT_BOLD}${crondir}/netdata-updater${TPUT_RESET}"
  echo >&2
  echo >&2 "${TPUT_DIM}${TPUT_BOLD}netdata-updater.sh${TPUT_RESET}${TPUT_DIM} works from cron. It will trigger an email from cron"
  echo >&2 "only if it fails (it should not print anything when it can update netdata).${TPUT_RESET}"
  echo >&2

  return 0
}

disable_netdata_updater() {
  crondir="$(_get_crondir)" || return 1
  _check_crondir_permissions "${crondir}" || return 1

  echo >&2 "You chose *NOT* to enable auto-update, removing any links to the updater from cron (it may have happened if you are reinstalling)"
  echo >&2

  if [ -f "${crondir}/netdata-updater" ]; then
    echo >&2 "Removing cron reference: ${crondir}/netdata-updater"
    echo >&2
    rm -f "${crondir}/netdata-updater"
  else
    echo >&2 "Did not find any cron entries to remove"
    echo >&2
  fi

  return 0
}

set_netdata_updater_channel() {
  sed -i -e "s/^RELEASE_CHANNEL=.*/RELEASE_CHANNEL=\"${RELEASE_CHANNEL}\"/" "${NETDATA_USER_CONFIG_DIR}/.environment"
}
