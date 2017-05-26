#!/usr/bin/env bash
#
# Run me with:
#
# bash <(curl -Ss https://my-netdata.io/kickstart.sh)
# 
# or (to install all netdata dependencies):
#
# bash <(curl -Ss https://my-netdata.io/kickstart.sh) all
#
#
# This script will:
#
# 1. install all netdata compilation dependencies
#    using the package manager of the system
#
# 2. download netdata source code in /usr/src/netdata.git
#
# 3. install netdata

umask 022


# ---------------------------------------------------------------------------------------------------------------------
# library functions copied from installer/functions.sh

which_cmd() {
    which "${1}" 2>/dev/null || \
        command -v "${1}" 2>/dev/null
}

check_cmd() {
    which_cmd "${1}" >/dev/null 2>&1 && return 0
    return 1
}

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

run_ok() {
    printf >&2 "${TPUT_BGGREEN}${TPUT_WHITE}${TPUT_BOLD} OK ${TPUT_RESET} ${*} \n\n"
}

run_failed() {
    printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} FAILED ${TPUT_RESET} ${*} \n\n"
}

run_logfile="/dev/null"
run() {
    local user="${USER:-}" dir="${PWD}" info info_console

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

# ---------------------------------------------------------------------------------------------------------------------

curl="$(which_cmd curl)"
wget="$(which_cmd wget)"
bash="$(which_cmd bash)"

if [ -z "${BASH_VERSION}" -o -z "${bash}" ]
then
    run_failed "This script needs BASH version 4+."
    echo >&2 "You can still install netdata on any 64bit machine."
    echo >&2 "Check this: https://github.com/firehol/binary-packages"
    exit 1
fi

if [ "${BASH_VERSINFO[0]}" -lt "4" ]
then
    run_failed "This script needs BASH version 4+, but you have BASH version ${BASH_VERSION}"
    echo >&2 "You can still install netdata on any 64bit machine."
    echo >&2 "Check this: https://github.com/firehol/binary-packages"
    exit 1
fi

# ---------------------------------------------------------------------------------------------------------------------
# this is where the action starts

set -e

tmp="$(mktemp /tmp/netdata-kickstart-XXXXXX)"

progress "Downloading script to detect required packages..."
if [ ! -z "${curl}" ]
then
	run ${curl} 'https://raw.githubusercontent.com/firehol/netdata-demo-site/master/install-required-packages.sh' >"${tmp}"
elif [ ! -z "${wget}" ]
then
	run "${wget}" -O - 'https://raw.githubusercontent.com/firehol/netdata-demo-site/master/install-required-packages.sh' >"${tmp}"
else
	run_failed "I need curl or wget to proceed, but neither is available on this system."
	rm "${tmp}"
	exit 1
fi

KICKSTART_OPTIONS="netdata"
if [ "${1}" = "all" ]
then
    KICKSTART_OPTIONS="netdata-all"
    shift 1
fi

ask=0
if [ -s "${tmp}" ]
then
	progress "Running downloaded script to detect required packages..."
	run "${bash}" "${tmp}" ${KICKSTART_OPTIONS} || ask=1
    rm "${tmp}"
else
	run_failed "Downloaded script is empty..."
	rm "${tmp}"
	exit 1
fi

if [ "${ask}" = "1" ]
then
    echo >&2 "It failed to install all the required packages, but I can try to install netdata."
	read -p "Press ENTER to continue to netdata installation > "
	progress "OK, let me give it a try..."
else
    progress "OK, downloading netdata..."
fi

SOURCE_DST="/usr/src"
sudo=""
[ "${UID}" -ne "0" ] && sudo="sudo"

[ ! -d "${SOURCE_DST}" ] && run ${sudo} mkdir -p "${SOURCE_DST}"

if [ ! -d "${SOURCE_DST}/netdata.git" ]
then
    progress "Downloading netdata source code..."
	run ${sudo} git clone https://github.com/firehol/netdata.git "${SOURCE_DST}/netdata.git"
	cd "${SOURCE_DST}/netdata.git"
else
    progress "Updating netdata source code..."
	cd "${SOURCE_DST}/netdata.git"
	run ${sudo} git fetch --all
	run ${sudo} git reset --hard origin/master
fi

progress "Running netdata installer..."

#if [ -f netdata-updater.sh ]
#then
#	run ${sudo} ./netdata-updater.sh || run ${sudo} ./netdata-installer.sh -u "${@}"
#else
	run ${sudo} ./netdata-installer.sh -u "${@}"
#fi

