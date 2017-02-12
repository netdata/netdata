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
    # Is stderr on the terminal? If not, then fail
    test -t 2 || return 1

    if [ ! -z "$TPUT_CMD" ]
    then
        if [ $[$($TPUT_CMD colors 2>/dev/null)] -ge 8 ]
        then
            # Enable colors
            COLOR_RESET="\e[0m"
            COLOR_BLACK="\e[30m"
            COLOR_RED="\e[31m"
            COLOR_GREEN="\e[32m"
            COLOR_YELLOW="\e[33m"
            COLOR_BLUE="\e[34m"
            COLOR_PURPLE="\e[35m"
            COLOR_CYAN="\e[36m"
            COLOR_WHITE="\e[37m"
            COLOR_BGBLACK="\e[40m"
            COLOR_BGRED="\e[41m"
            COLOR_BGGREEN="\e[42m"
            COLOR_BGYELLOW="\e[43m"
            COLOR_BGBLUE="\e[44m"
            COLOR_BGPURPLE="\e[45m"
            COLOR_BGCYAN="\e[46m"
            COLOR_BGWHITE="\e[47m"
            COLOR_BOLD="\e[1m"
            COLOR_DIM="\e[2m"
            COLOR_UNDERLINED="\e[4m"
            COLOR_BLINK="\e[5m"
            COLOR_INVERTED="\e[7m"
        fi
    fi

    return 0
}
TPUT_CMD="$(which_cmd tput)"
setup_terminal

progress() {
    printf >&2 " --- ${COLOR_DIM}${COLOR_BOLD}${*}${COLOR_RESET} --- \n"
}

# -----------------------------------------------------------------------------

netdata_banner() {
    local   l1="  ^"                                                                            \
            l2="  |.-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-.   .-"  \
            l3="  |   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'   '-'  "  \
            l4="  +----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+--->" \
            sp="                                                                              " \
            netdata="netdata" start end msg="${*}" chartcolor="${COLOR_DIM}"

    [ ${#msg} -lt ${#netdata} ] && msg="${msg}${sp:0:$(( ${#netdata} - ${#msg}))}"
    [ ${#msg} -gt $(( ${#l2} - 20 )) ] && msg="${msg:0:$(( ${#l2} - 23 ))}..."

    start="$(( ${#l2} / 2 - 4 ))"
    [ $(( start + ${#msg} + 4 )) -gt ${#l2} ] && start=$((${#l2} - ${#msg} - 4))
    end=$(( ${start} + ${#msg} + 4 ))

    echo >&2
    echo >&2 -e "${chartcolor}${l1}${COLOR_RESET}"
    echo >&2 -e "${chartcolor}${l2:0:start}${sp:0:2}${COLOR_RESET}${COLOR_BOLD}${COLOR_GREEN}${netdata}${COLOR_RESET}${chartcolor}${sp:0:$((end - start - 2 - ${#netdata}))}${l2:end:$((${#l2} - end))}${COLOR_RESET}"
    echo >&2 -e "${chartcolor}${l3:0:start}${sp:0:2}${COLOR_RESET}${COLOR_BOLD}${COLOR_CYAN}${msg}${COLOR_RESET}${chartcolor}${sp:0:2}${l3:end:$((${#l2} - end))}${COLOR_RESET}"
    echo >&2 -e "${chartcolor}${l4}${COLOR_RESET}"
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

run_logfile="/dev/null"
run() {
    local user="${USER}" dir="$(basename "${PWD}")" info info_console

    if [ "${UID}" = "0" ]
        then
        info="[root ${dir}]# "
        info_console="[${COLOR_DIM}${dir}${COLOR_RESET}]# "
    else
        info="[${user} ${dir}]$ "
        info_console="[${COLOR_DIM}${dir}${COLOR_RESET}]$ "
    fi

    printf >> "${run_logfile}" "${info}"
    printf >> "${run_logfile}" "%q " "${@}"
    printf >> "${run_logfile}" " ... "

    printf >&2 "${info_console}${COLOR_BOLD}${COLOR_YELLOW}"
    printf >&2 "%q " "${@}"
    printf >&2 "${COLOR_RESET}\n"

    "${@}"

    local ret=$?
    if [ ${ret} -ne 0 ]
        then
        printf >&2 "${COLOR_BGRED}${COLOR_WHITE}${COLOR_BOLD} FAILED ${COLOR_RESET}\n\n"
        printf >> "${run_logfile}" "FAILED with exit code ${ret}\n"
    else
        printf >&2 "${COLOR_BGGREEN}${COLOR_WHITE}${COLOR_BOLD} OK ${COLOR_RESET}\n\n"
        printf >> "${run_logfile}" "OK\n"
    fi

    return ${ret}
}

portable_add_user() {
    local username="${1}"

    getent passwd "${username}" > /dev/null 2>&1
    [ $? -eq 0 ] && return 0

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
    [ $? -eq 0 ] && return 0

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
    [ $? -ne 0 ] && return 1

    # find the user is already in the group
    local users=$(getent group "${groupname}" | cut -d ':' -f 4)
    if [[ ",${users}," =~ ,${username}, ]]
        then
        # username is already there
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
    local pid=$( cat /proc/1/sched | head -n 1 | { IFS='(),#:' read name pid th threads; echo $pid; } )
    local p=$(( pid + 0 ))
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
