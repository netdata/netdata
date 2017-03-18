# no shebang necessary - this is a library to be sourced

# -----------------------------------------------------------------------------
# checking the availability of commands

which_cmd() {
    which "${1}" 2>/dev/null || \
        command -v "${1}" 2>/dev/null
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
setup_terminal

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
systemctl_cmd="$(which_cmd systemctl)"
service() {
    local cmd="${1}" action="${2}"

    if [ ! -z "${service_cmd}" ]
    then
        run "${service_cmd}" "${cmd}" "${action}"
        return $?
    elif [ ! -z "${systemctl_cmd}" ]
    then
        run "${systemctl_cmd}" "${action}" "${cmd}"
        return $?
    fi
    return 1
}

# -----------------------------------------------------------------------------

run_ok() {
    printf >&2 "${TPUT_BGGREEN}${TPUT_WHITE}${TPUT_BOLD} OK ${TPUT_RESET} ${*} \n\n"
}

run_failed() {
    printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} FAILED ${TPUT_RESET} ${*} \n\n"
}

run_logfile="/dev/null"
run() {
    local user="${USER}" dir="$(basename "${PWD}")" info info_console

    if [ "${UID}" = "0" ]
        then
        info="[root ${dir}]# "
        info_console="[${TPUT_DIM}${dir}${TPUT_RESET}]# "
    else
        info="[${user} ${dir}]$ "
        info_console="[${TPUT_DIM}${dir}${TPUT_RESET}]$ "
    fi

    printf >> "${run_logfile}" "${info}"
    printf >> "${run_logfile}" "%q " "${@}"
    printf >> "${run_logfile}" " ... "

    printf >&2 "${info_console}${TPUT_BOLD}${TPUT_YELLOW}"
    printf >&2 "%q " "${@}"
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

portable_add_user() {
    local username="${1}"

    getent passwd "${username}" > /dev/null 2>&1
    [ $? -eq 0 ] && echo >&2 "User '${username}' already exists." && return 0

    echo >&2 "Adding ${username} user account ..."

    local nologin="$(which nologin 2>/dev/null || command -v nologin 2>/dev/null || echo '/bin/false')"

    # Linux
    if check_cmd useradd
    then
        run useradd -r -g "${username}" -c "${username}" -s "${nologin}" -d / "${username}" && return 0
    fi

    # FreeBSD
    if check_cmd pw
    then
        run pw useradd "${username}" -d / -g "${username}" -s "${nologin}" && return 0
    fi

    # BusyBox
    if check_cmd adduser
    then
        run adduser -D -G "${username}" "${username}" && return 0
    fi

    echo >&2 "Failed to add ${username} user account !"

    return 1
}

portable_add_group() {
    local groupname="${1}"

    getent group "${groupname}" > /dev/null 2>&1
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

    getent group "${groupname}" > /dev/null 2>&1
    [ $? -ne 0 ] && echo >&2 "Group '${groupname}' does not exist." && return 1

    # find the user is already in the group
    local users=$(getent group "${groupname}" | cut -d ':' -f 4)
    if [[ ",${users}," =~ ,${username}, ]]
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

    # if the directory /etc/systemd/system does not exit, it is not systemd
    [ ! -d /etc/systemd/system ] && return 1

    # if there is no systemctl command, it is not systemd
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
        [ ! -z "${myns}" && "${myns}" = "${ns}" ] && return 0
    done

    # else, it is not systemd
    return 1
}
