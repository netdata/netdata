#!/usr/bin/env sh
# SPDX-License-Identifier: GPL-3.0+
# shellcheck disable=SC1117,SC2016,SC2034,SC2039,SC2059,SC2086,SC2119,SC2120,SC2129,SC2162,SC2166,SC2181

umask 022

# make sure UID is set
[ -z "${UID}" ] && export UID="$(id -u)"

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
setup_terminal || echo >/dev/null

progress() {
    echo >&2 " --- ${TPUT_DIM}${TPUT_BOLD}${*}${TPUT_RESET} --- "
}

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


# ---------------------------------------------------------------------------------------------------------------------

fatal() {
    printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ABORTED ${TPUT_RESET} ${*} \n\n"
    exit 1
}

# ---------------------------------------------------------------------------------------------------------------------

if [ "$(uname -m)" != "x86_64" ]
	then
	fatal "Static binary versions of netdata are available only for 64bit Intel/AMD CPUs (x86_64), but yours is: $(uname -m)."
fi

if [ "$(uname -s)" != "Linux" ]
	then
	fatal "Static binary versions of netdata are available only for Linux, but this system is $(uname -s)"
fi

curl="$(which_cmd curl)"
wget="$(which_cmd wget)"

# ---------------------------------------------------------------------------------------------------------------------

progress "Checking the latest version of static build..."

BASE='https://raw.githubusercontent.com/firehol/binary-packages/master'

LATEST=
if [ ! -z "${curl}" -a -x "${curl}" ]
then
    LATEST="$(run ${curl} "${BASE}/netdata-latest.gz.run")"
elif [ ! -z "${wget}" -a -x "${wget}" ]
then
    LATEST="$(run ${wget} -O - "${BASE}/netdata-latest.gz.run")"
else
    fatal "curl or wget are needed for this script to work."
fi

if [ -z "${LATEST}" ]
	then
	fatal "Cannot find the latest static binary version of netdata."
fi

# ---------------------------------------------------------------------------------------------------------------------

progress "Downloading static netdata binary: ${LATEST}"

ret=1
if [ ! -z "${curl}" -a -x "${curl}" ]
then
    run ${curl} "${BASE}/${LATEST}" >"/tmp/${LATEST}"
    ret=$?
elif [ ! -z "${wget}" -a -x "${wget}" ]
then
    run ${wget} -O "/tmp/${LATEST}" "${BASE}/${LATEST}"
    ret=$?
else
    fatal "curl or wget are needed for this script to work."
fi

if [ ${ret} -ne 0 -o ! -s "/tmp/${LATEST}" ]
	then
	fatal "Failed to download the latest static binary version of netdata."
fi

# ---------------------------------------------------------------------------------------------------------------------

opts=
inner_opts=
while [ ! -z "${1}" ]
do
    if [ "${1}" = "--dont-wait" -o "${1}" = "--non-interactive" -o "${1}" = "--accept" ]
    then
        opts="${opts} --accept"
    elif [ "${1}" = "--dont-start-it" ]
    then
        inner_opts="${inner_opts} ${1}"
    else
        echo >&2 "Unknown option '${1}'"
        exit 1
    fi
    shift
done
[ ! -z "${inner_opts}" ] && inner_opts="-- ${inner_opts}"

# ---------------------------------------------------------------------------------------------------------------------

progress "Installing netdata"

sudo=
[ "${UID}" != "0" ] && sudo="sudo"
run ${sudo} sh "/tmp/${LATEST}" ${opts} ${inner_opts}

if [ $? -eq 0 ]
	then
	rm "/tmp/${LATEST}"
else
	echo >&2 "NOTE: did not remove: /tmp/${LATEST}"
fi
