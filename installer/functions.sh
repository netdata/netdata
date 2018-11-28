# no shebang necessary - this is a library to be sourced
# SPDX-License-Identifier: GPL-3.0-or-later
# shellcheck disable=SC1091,SC1117,SC2002,SC2004,SC2034,SC2046,SC2059,SC2086,SC2129,SC2148,SC2154,SC2155,SC2162,SC2166,SC2181,SC2193

# make sure we have a UID
[ -z "${UID}" ] && UID="$(id -u)"


# -----------------------------------------------------------------------------
# checking the availability of commands

which_cmd() {
    # shellcheck disable=SC2230
    which "${1}" 2>/dev/null || command -v "${1}" 2>/dev/null
}

check_cmd() {
    which_cmd "${1}" >/dev/null 2>&1 && return 0
    return 1
}


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

    if check_cmd tput
    then
        if [ $(( $(tput colors 2>/dev/null) )) -ge 8 ]
        then
            # Enable colors
            TPUT_RESET="$(tput sgr 0)"
            TPUT_BLACK="$(tput setaf 0)"
            TPUT_RED="$(tput setaf 1)"
            TPUT_GREEN="$(tput setaf 2)"
            TPUT_YELLOW="$(tput setaf 3)"
            TPUT_BLUE="$(tput setaf 4)"
            TPUT_PURPLE="$(tput setaf 5)"
            TPUT_CYAN="$(tput setaf 6)"
            TPUT_WHITE="$(tput setaf 7)"
            TPUT_BGBLACK="$(tput setab 0)"
            TPUT_BGRED="$(tput setab 1)"
            TPUT_BGGREEN="$(tput setab 2)"
            TPUT_BGYELLOW="$(tput setab 3)"
            TPUT_BGBLUE="$(tput setab 4)"
            TPUT_BGPURPLE="$(tput setab 5)"
            TPUT_BGCYAN="$(tput setab 6)"
            TPUT_BGWHITE="$(tput setab 7)"
            TPUT_BOLD="$(tput bold)"
            TPUT_DIM="$(tput dim)"
            TPUT_UNDERLINED="$(tput smul)"
            TPUT_BLINK="$(tput blink)"
            TPUT_INVERTED="$(tput rev)"
            TPUT_STANDOUT="$(tput smso)"
            TPUT_BELL="$(tput bel)"
            TPUT_CLEAR="$(tput clear)"
        fi
    fi

    return 0
}
setup_terminal || echo >/dev/null

progress() {
    echo >&2 " --- ${TPUT_DIM}${TPUT_BOLD}${*}${TPUT_RESET} --- "
}

# -----------------------------------------------------------------------------

netdata_banner() {
    local   l1="  ^"                                                                            \
            l2="  |.-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-"  \
            l3="  |   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'  "  \
            l4="  +----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+--->" \
            sp="                                                                              " \
            netdata="netdata" start end msg="${*}" chartcolor="${TPUT_DIM}"

    [ ${#msg} -lt ${#netdata} ] && msg="${msg}${sp:0:$(( ${#netdata} - ${#msg}))}"
    [ ${#msg} -gt $(( ${#l2} - 20 )) ] && msg="${msg:0:$(( ${#l2} - 23 ))}..."

    start="$(( ${#l2} / 2 - 4 ))"
    [ $(( start + ${#msg} + 4 )) -gt ${#l2} ] && start=$((${#l2} - ${#msg} - 4))
    end=$(( ${start} + ${#msg} + 4 ))

    echo >&2
    echo >&2 "${chartcolor}${l1}${TPUT_RESET}"
    echo >&2 "${chartcolor}${l2:0:start}${sp:0:2}${TPUT_RESET}${TPUT_BOLD}${TPUT_GREEN}${netdata}${TPUT_RESET}${chartcolor}${sp:0:$((end - start - 2 - ${#netdata}))}${l2:end:$((${#l2} - end))}${TPUT_RESET}"
    echo >&2 "${chartcolor}${l3:0:start}${sp:0:2}${TPUT_RESET}${TPUT_BOLD}${TPUT_CYAN}${msg}${TPUT_RESET}${chartcolor}${sp:0:2}${l3:end:$((${#l2} - end))}${TPUT_RESET}"
    echo >&2 "${chartcolor}${l4}${TPUT_RESET}"
    echo >&2
}

# -----------------------------------------------------------------------------
# portable service command

service_cmd="$(which_cmd service)"
rcservice_cmd="$(which_cmd rc-service)"
systemctl_cmd="$(which_cmd systemctl)"
service() {
    local cmd="${1}" action="${2}"

    if [ ! -z "${systemctl_cmd}" ]
    then
        run "${systemctl_cmd}" "${action}" "${cmd}"
        return $?
    elif [ ! -z "${service_cmd}" ]
    then
        run "${service_cmd}" "${cmd}" "${action}"
        return $?
    elif [ ! -z "${rcservice_cmd}" ]
    then
        run "${rcservice_cmd}" "${cmd}" "${action}"
        return $?
    fi
    return 1
}

# -----------------------------------------------------------------------------
# portable pidof

pidof_cmd="$(which_cmd pidof)"
pidof() {
    if [ ! -z "${pidof_cmd}" ]
    then
        ${pidof_cmd} "${@}"
        return $?
    else
        ps -acxo pid,comm |\
            sed "s/^ *//g" |\
            grep netdata |\
            cut -d ' ' -f 1
        return $?
    fi
}

# -----------------------------------------------------------------------------
# portable delete recursively interactively

portable_deletedir_recursively_interactively() {
    if [ ! -z "$1" -a -d "$1" ]
        then
        if [ "$(uname -s)" = "Darwin" ]
        then
            echo >&2
            read >&2 -p "Press ENTER to recursively delete directory '$1' > "
            echo >&2 "Deleting directory '$1' ..."
            run rm -R "$1"
        else
            echo >&2
            echo >&2 "Deleting directory '$1' ..."
            run rm -I -R "$1"
        fi
    else
        echo "Directory '$1' does not exist."
    fi
}


# -----------------------------------------------------------------------------

export SYSTEM_CPUS=1
portable_find_processors() {
    if [ -f "/proc/cpuinfo" ]
    then
        # linux
        SYSTEM_CPUS=$(grep -c ^processor /proc/cpuinfo)
    else
        # freebsd
        SYSTEM_CPUS=$(sysctl hw.ncpu 2>/dev/null | grep ^hw.ncpu | cut -d ' ' -f 2)
    fi
    [ -z "${SYSTEM_CPUS}" -o $(( SYSTEM_CPUS )) -lt 1 ] && SYSTEM_CPUS=1
}
portable_find_processors

# -----------------------------------------------------------------------------

run_ok() {
    printf >&2 "${TPUT_BGGREEN}${TPUT_WHITE}${TPUT_BOLD} OK ${TPUT_RESET} ${*} \n\n"
}

run_failed() {
    printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} FAILED ${TPUT_RESET} ${*} \n\n"
}

ESCAPED_PRINT_METHOD=
printf "%q " test >/dev/null 2>&1
[ $? -eq 0 ] && ESCAPED_PRINT_METHOD="printfq"
escaped_print() {
    if [ "${ESCAPED_PRINT_METHOD}" = "printfq" ]
    then
        printf "%q " "${@}"
    else
        printf "%s" "${*}"
    fi
    return 0
}

run_logfile="/dev/null"
run() {
    local user="${USER--}" dir="${PWD}" info info_console

    if [ "${UID}" = "0" ]
        then
        info="[root ${dir}]# "
        info_console="[${TPUT_DIM}${dir}${TPUT_RESET}]# "
    else
        info="[${user} ${dir}]$ "
        info_console="[${TPUT_DIM}${dir}${TPUT_RESET}]$ "
    fi

    printf >> "${run_logfile}" "${info}"
    escaped_print >> "${run_logfile}" "${@}"
    printf >> "${run_logfile}" " ... "

    printf >&2 "${info_console}${TPUT_BOLD}${TPUT_YELLOW}"
    escaped_print >&2 "${@}"
    printf >&2 "${TPUT_RESET}\n"

    "${@}"

    local ret=$?
    if [ ${ret} -ne 0 ]
        then
        run_failed
        printf >> "${run_logfile}" "FAILED with exit code ${ret}\n"
    else
        run_ok
        printf >> "${run_logfile}" "OK\n"
    fi

    return ${ret}
}

getent_cmd="$(which_cmd getent)"
portable_check_user_exists() {
    local username="${1}" found=

    if [ ! -z "${getent_cmd}" ]
        then
        "${getent_cmd}" passwd "${username}" >/dev/null 2>&1
        return $?
    fi

    found="$(cut -d ':' -f 1 </etc/passwd | grep "^${username}$")"
    [ "${found}" = "${username}" ] && return 0
    return 1
}

portable_check_group_exists() {
    local groupname="${1}" found=

    if [ ! -z "${getent_cmd}" ]
        then
        "${getent_cmd}" group "${groupname}" >/dev/null 2>&1
        return $?
    fi

    found="$(cut -d ':' -f 1 </etc/group | grep "^${groupname}$")"
    [ "${found}" = "${groupname}" ] && return 0
    return 1
}

portable_check_user_in_group() {
    local username="${1}" groupname="${2}" users=

    if [ ! -z "${getent_cmd}" ]
        then
        users="$(getent group "${groupname}" | cut -d ':' -f 4)"
    else
        users="$(grep "^${groupname}:" </etc/group | cut -d ':' -f 4)"
    fi

    [[ ",${users}," =~ ,${username}, ]] && return 0
    return 1
}

portable_add_user() {
    local username="${1}" homedir="${2}"

    [ -z "${homedir}" ] && homedir="/tmp"

    portable_check_user_exists "${username}"
    [ $? -eq 0 ] && echo >&2 "User '${username}' already exists." && return 0

    echo >&2 "Adding ${username} user account with home ${homedir} ..."

    # shellcheck disable=SC2230
    local nologin="$(which nologin 2>/dev/null || command -v nologin 2>/dev/null || echo '/bin/false')"

    # Linux
    if check_cmd useradd
    then
        run useradd -r -g "${username}" -c "${username}" -s "${nologin}" --no-create-home -d "${homedir}" "${username}" && return 0
    fi

    # FreeBSD
    if check_cmd pw
    then
        run pw useradd "${username}" -d "${homedir}" -g "${username}" -s "${nologin}" && return 0
    fi

    # BusyBox
    if check_cmd adduser
    then
        run adduser -h "${homedir}" -s "${nologin}" -D -G "${username}" "${username}" && return 0
    fi

    echo >&2 "Failed to add ${username} user account !"

    return 1
}

portable_add_group() {
    local groupname="${1}"

    portable_check_group_exists "${groupname}"
    [ $? -eq 0 ] && echo >&2 "Group '${groupname}' already exists." && return 0

    echo >&2 "Adding ${groupname} user group ..."

    # Linux
    if check_cmd groupadd
    then
        run groupadd -r "${groupname}" && return 0
    fi

    # FreeBSD
    if check_cmd pw
    then
        run pw groupadd "${groupname}" && return 0
    fi

    # BusyBox
    if check_cmd addgroup
    then
        run addgroup "${groupname}" && return 0
    fi

    echo >&2 "Failed to add ${groupname} user group !"
    return 1
}

portable_add_user_to_group() {
    local groupname="${1}" username="${2}"

    portable_check_group_exists "${groupname}"
    [ $? -ne 0 ] && echo >&2 "Group '${groupname}' does not exist." && return 1

    # find the user is already in the group
    if portable_check_user_in_group "${username}" "${groupname}"
        then
        # username is already there
        echo >&2 "User '${username}' is already in group '${groupname}'."
        return 0
    else
        # username is not in group
        echo >&2 "Adding ${username} user to the ${groupname} group ..."

        # Linux
        if check_cmd usermod
        then
            run usermod -a -G "${groupname}" "${username}" && return 0
        fi

        # FreeBSD
        if check_cmd pw
        then
            run pw groupmod "${groupname}" -m "${username}" && return 0
        fi

        # BusyBox
        if check_cmd addgroup
        then
            run addgroup "${username}" "${groupname}" && return 0
        fi

        echo >&2 "Failed to add user ${username} to group ${groupname} !"
        return 1
    fi
}

iscontainer() {
    # man systemd-detect-virt
    local cmd=$(which_cmd systemd-detect-virt)
    if [ ! -z "${cmd}" -a -x "${cmd}" ]
        then
        "${cmd}" --container >/dev/null 2>&1 && return 0
    fi

    # /proc/1/sched exposes the host's pid of our init !
    # http://stackoverflow.com/a/37016302
    local pid=$( cat /proc/1/sched 2>/dev/null | head -n 1 | { IFS='(),#:' read name pid th threads; echo $pid; } )
    pid=$(( pid + 0 ))
    [ ${pid} -ne 1 ] && return 0

    # lxc sets environment variable 'container'
    [ ! -z "${container}" ] && return 0

    # docker creates /.dockerenv
    # http://stackoverflow.com/a/25518345
    [ -f "/.dockerenv" ] && return 0

    # ubuntu and debian supply /bin/running-in-container
    # https://www.apt-browse.org/browse/ubuntu/trusty/main/i386/upstart/1.12.1-0ubuntu4/file/bin/running-in-container
    if [ -x "/bin/running-in-container" ]
        then
        "/bin/running-in-container" >/dev/null 2>&1 && return 0
    fi

    return 1
}

issystemd() {
    local pids p myns ns systemctl

    # if the directory /lib/systemd/system OR /usr/lib/systemd/system (SLES 12.x) does not exit, it is not systemd
    [ ! -d /lib/systemd/system -a ! -d /usr/lib/systemd/system ] && return 1

    # if there is no systemctl command, it is not systemd
    # shellcheck disable=SC2230
    systemctl=$(which systemctl 2>/dev/null || command -v systemctl 2>/dev/null)
    [ -z "${systemctl}" -o ! -x "${systemctl}" ] && return 1

    # if pid 1 is systemd, it is systemd
    [ "$(basename $(readlink /proc/1/exe) 2>/dev/null)" = "systemd" ] && return 0

    # if systemd is not running, it is not systemd
    pids=$(pidof systemd 2>/dev/null)
    [ -z "${pids}" ] && return 1

    # check if the running systemd processes are not in our namespace
    myns="$(readlink /proc/self/ns/pid 2>/dev/null)"
    for p in ${pids}
    do
        ns="$(readlink /proc/${p}/ns/pid 2>/dev/null)"

        # if pid of systemd is in our namespace, it is systemd
        [ ! -z "${myns}" ] && [ "${myns}" = "${ns}" ] && return 0
    done

    # else, it is not systemd
    return 1
}

install_non_systemd_init() {
    [ "${UID}" != 0 ] && return 1

    local key="unknown"
    if [ -f /etc/os-release ]
        then
        source /etc/os-release || return 1
        key="${ID}-${VERSION_ID}"

    elif [ -f /etc/redhat-release ]
        then
        key=$(</etc/redhat-release)
    fi

    if [ -d /etc/init.d -a ! -f /etc/init.d/netdata ]
        then
        if [[ "${key}" =~ ^(gentoo|alpine).* ]]
            then
            echo >&2 "Installing OpenRC init file..."
            run cp system/netdata-openrc /etc/init.d/netdata && \
            run chmod 755 /etc/init.d/netdata && \
            run rc-update add netdata default && \
            return 0
        
        elif [ "${key}" = "debian-7" \
            -o "${key}" = "ubuntu-12.04" \
            -o "${key}" = "ubuntu-14.04" \
            ]
            then
            echo >&2 "Installing LSB init file..."
            run cp system/netdata-lsb /etc/init.d/netdata && \
            run chmod 755 /etc/init.d/netdata && \
            run update-rc.d netdata defaults && \
            run update-rc.d netdata enable && \
            return 0
        elif [[ "${key}" =~ ^(amzn-201[5678]|ol|CentOS release 6|Red Hat Enterprise Linux Server release 6|Scientific Linux CERN SLC release 6|CloudLinux Server release 6).* ]]
            then
            echo >&2 "Installing init.d file..."
            run cp system/netdata-init-d /etc/init.d/netdata && \
            run chmod 755 /etc/init.d/netdata && \
            run chkconfig netdata on && \
            return 0
        else
            echo >&2 "I don't know what init file to install on system '${key}'. Open a github issue to help us fix it."
            return 1
        fi
    elif [ -f /etc/init.d/netdata ]
        then
        echo >&2 "file '/etc/init.d/netdata' already exists."
        return 0
    else
        echo >&2 "I don't know what init file to install on system '${key}'. Open a github issue to help us fix it."
    fi

    return 1
}

NETDATA_START_CMD="netdata"
NETDATA_STOP_CMD="killall netdata"

install_netdata_service() {
    local uname="$(uname 2>/dev/null)"

    if [ "${UID}" -eq 0 ]
    then
        if [ "${uname}" = "Darwin" ]
        then

            if [ -f "/Library/LaunchDaemons/com.github.netdata.plist" ]
                then
                echo >&2 "file '/Library/LaunchDaemons/com.github.netdata.plist' already exists."
                return 0
            else
                echo >&2 "Installing MacOS X plist file..."
                run cp system/netdata.plist /Library/LaunchDaemons/com.github.netdata.plist && \
                run launchctl load /Library/LaunchDaemons/com.github.netdata.plist && \
                return 0
            fi

        elif [ "${uname}" = "FreeBSD" ]
        then

            run cp system/netdata-freebsd /etc/rc.d/netdata && \
                NETDATA_START_CMD="service netdata start" && \
                NETDATA_STOP_CMD="service netdata stop" && \
                return 0

        elif issystemd
        then
            # systemd is running on this system
            NETDATA_START_CMD="systemctl start netdata"
            NETDATA_STOP_CMD="systemctl stop netdata"

            SYSTEMD_DIRECTORY=""

            if [ -d "/lib/systemd/system" ]
            then
                SYSTEMD_DIRECTORY="/lib/systemd/system"
            elif [ -d "/usr/lib/systemd/system" ]
            then
                SYSTEMD_DIRECTORY="/usr/lib/systemd/system"
            fi

            if [ "${SYSTEMD_DIRECTORY}x" != "x" ]
            then
                echo >&2 "Installing systemd service..."
                run cp system/netdata.service "${SYSTEMD_DIRECTORY}/netdata.service" && \
                    run systemctl daemon-reload && \
                    run systemctl enable netdata && \
                    return 0
            else
                echo >&2 "no systemd directory; cannot install netdata.service"
            fi
        else
            install_non_systemd_init
            local ret=$?

            if [ ${ret} -eq 0 ]
            then
                if [ ! -z "${service_cmd}" ]
                then
                    NETDATA_START_CMD="service netdata start"
                    NETDATA_STOP_CMD="service netdata stop"
                elif [ ! -z "${rcservice_cmd}" ]
                then
                    NETDATA_START_CMD="rc-service netdata start"
                    NETDATA_STOP_CMD="rc-service netdata stop"
                fi
            fi

            return ${ret}
        fi
    fi

    return 1
}


# -----------------------------------------------------------------------------
# stop netdata

pidisnetdata() {
    if [ -d /proc/self ]
    then
        [ -z "$1" -o ! -f "/proc/$1/stat" ] && return 1
        [ "$(cat "/proc/$1/stat" | cut -d '(' -f 2 | cut -d ')' -f 1)" = "netdata" ] && return 0
        return 1
    fi
    return 0
}

stop_netdata_on_pid() {
    local pid="${1}" ret=0 count=0

    pidisnetdata ${pid} || return 0

    printf >&2 "Stopping netdata on pid ${pid} ..."
    while [ ! -z "$pid" -a ${ret} -eq 0 ]
    do
        if [ ${count} -gt 45 ]
            then
            echo >&2 "Cannot stop the running netdata on pid ${pid}."
            return 1
        fi

        count=$(( count + 1 ))

        run kill ${pid} 2>/dev/null
        ret=$?

        test ${ret} -eq 0 && printf >&2 "." && sleep 2
    done

    echo >&2
    if [ ${ret} -eq 0 ]
    then
        echo >&2 "SORRY! CANNOT STOP netdata ON PID ${pid} !"
        return 1
    fi

    echo >&2 "netdata on pid ${pid} stopped."
    return 0
}

netdata_pids() {
    local p myns ns

    myns="$(readlink /proc/self/ns/pid 2>/dev/null)"

    # echo >&2 "Stopping a (possibly) running netdata (namespace '${myns}')..."

    for p in \
        $(cat /var/run/netdata.pid 2>/dev/null) \
        $(cat /var/run/netdata/netdata.pid 2>/dev/null) \
        $(pidof netdata 2>/dev/null)
    do
        ns="$(readlink /proc/${p}/ns/pid 2>/dev/null)"

        if [ -z "${myns}" -o -z "${ns}" -o "${myns}" = "${ns}" ]
            then
            pidisnetdata ${p} && echo "${p}"
        fi
    done
}

stop_all_netdata() {
    local p
    for p in $(netdata_pids)
    do
        stop_netdata_on_pid ${p}
    done
}

# -----------------------------------------------------------------------------
# restart netdata

restart_netdata() {
    local netdata="${1}"
    shift

    local started=0

    progress "Start netdata"

    if [ "${UID}" -eq 0 ]
        then
        service netdata stop
        stop_all_netdata
        service netdata restart && started=1

        if [ ${started} -eq 1 -a -z "$(netdata_pids)" ]
        then
            echo >&2 "Ooops! it seems netdata is not started."
            started=0
        fi

        if [ ${started} -eq 0 ]
        then
            service netdata start && started=1
        fi
    fi

    if [ ${started} -eq 1 -a -z "$(netdata_pids)" ]
    then
        echo >&2 "Hm... it seems netdata is still not started."
        started=0
    fi

    if [ ${started} -eq 0 ]
    then
        # still not started...

        run stop_all_netdata
        run "${netdata}" "${@}"
        return $?
    fi

    return 0
}

# -----------------------------------------------------------------------------
# install netdata logrotate

install_netdata_logrotate() {
    if [ ${UID} -eq 0 ]
        then
        if [ -d /etc/logrotate.d ]
            then
            if [ ! -f /etc/logrotate.d/netdata ]
                then
                run cp system/netdata.logrotate /etc/logrotate.d/netdata
            fi
            
            if [ -f /etc/logrotate.d/netdata ]
                then
                run chmod 644 /etc/logrotate.d/netdata
            fi

            return 0
        fi
    fi
    
    return 1
}

# -----------------------------------------------------------------------------
# download netdata.conf

fix_netdata_conf() {
    local owner="${1}"

    if [ "${UID}" -eq 0 ]
        then
        run chown "${owner}" "${filename}"
    fi
    run chmod 0664 "${filename}"
}

generate_netdata_conf() {
    local owner="${1}" filename="${2}" url="${3}"

    if [ ! -s "${filename}" ]
        then
        cat >"${filename}" <<EOFCONF
# netdata can generate its own config.
# Get it with:
#
# wget -O ${filename} "${url}"
#
# or
#
# curl -s -o ${filename} "${url}"
#
EOFCONF
        fix_netdata_conf "${owner}"
    fi
}

download_netdata_conf() {
    local owner="${1}" filename="${2}" url="${3}"

    if [ ! -s "${filename}" ]
        then
        echo >&2
        echo >&2 "-------------------------------------------------------------------------------"
        echo >&2
        echo >&2 "Downloading default configuration from netdata..."
        sleep 5

        # remove a possibly obsolete download
        [ -f "${filename}.new" ] && rm "${filename}.new"

        # disable a proxy to get data from the local netdata
        export http_proxy=
        export https_proxy=

        # try curl
        run curl -s -o "${filename}.new" "${url}"
        ret=$?

        if [ ${ret} -ne 0 -o ! -s "${filename}.new" ]
            then
            # try wget
            run wget -O "${filename}.new" "${url}"
            ret=$?
        fi

        if [ ${ret} -eq 0 -a -s "${filename}.new" ]
            then
            run mv "${filename}.new" "${filename}"
            run_ok "New configuration saved for you to edit at ${filename}"
        else
            [ -f "${filename}.new" ] && rm "${filename}.new"
            run_failed "Cannnot download configuration from netdata daemon using url '${url}'"

            generate_netdata_conf "${owner}" "${filename}" "${url}"
        fi

        fix_netdata_conf "${owner}"
    fi
}


# -----------------------------------------------------------------------------
# add netdata user and group

NETDATA_WANTED_GROUPS="docker nginx varnish haproxy adm nsd proxy squid ceph nobody"
NETDATA_ADDED_TO_GROUPS=""
add_netdata_user_and_group() {
    local homedir="${1}" g

    if [ ${UID} -eq 0 ]
        then
        portable_add_group netdata || return 1
        portable_add_user netdata "${homedir}"  || return 1

        for g in ${NETDATA_WANTED_GROUPS}
        do
            portable_add_user_to_group ${g} netdata && NETDATA_ADDED_TO_GROUPS="${NETDATA_ADDED_TO_GROUPS} ${g}"
        done

        [ ~netdata = / ] && cat <<USERMOD

The netdata user has its home directory set to /
You may want to change it, using this command:

# usermod -d "${homedir}" netdata

USERMOD
        return 0
    fi

    return 1
}
