#!/usr/bin/env sh
#
# Run me with:
#
# bash <(curl -Ss https://my-netdata.io/kickstart.sh)
# 
# or (to install all netdata dependencies):
#
# bash <(curl -Ss https://my-netdata.io/kickstart.sh) all
#
# Other options:
#  --src-dir PATH    keep netdata.git at PATH/netdata.git
#  --dont-wait       do not prompt for user input
#  --non-interactive do not prompt for user input
#  --no-updates      do not install script for daily updates
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

[ -z "${UID}" ] && UID="$(id -u)"

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
# collect system information

fatal() {
    printf >&2 "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ABORTED ${TPUT_RESET} ${*} \n\n"
    exit 1
}

export PATH="${PATH}:/usr/local/bin:/usr/local/sbin"

curl="$(which_cmd curl)"
wget="$(which_cmd wget)"
bash="$(which_cmd bash)"

if [ -z "${BASH_VERSION}" ]
then
    # we don't run under bash
    if [ ! -z "${bash}" -a -x "${bash}" ]
    then
        BASH_MAJOR_VERSION=$(${bash} -c 'echo "${BASH_VERSINFO[0]}"')
    fi
else
    # we run under bash
    BASH_MAJOR_VERSION="${BASH_VERSINFO[0]}"
fi

HAS_BASH4=1
if [ -z "${BASH_MAJOR_VERSION}" ]
then
    echo >&2 "No BASH is available on this system"
    HAS_BASH4=0
elif [ $((BASH_MAJOR_VERSION)) -lt 4 ]
then
    echo >&2 "No BASH v4+ is available on this system (installed bash is v${BASH_MAJOR_VERSION}"
    HAS_BASH4=0
fi

SYSTEM="$(uname -s)"
OS="$(uname -o)"
MACHINE="$(uname -m)"

cat <<EOF
System            : ${SYSTEM}
Operating System  : ${OS}
Machine           : ${MACHINE}
BASH major version: ${BASH_MAJOR_VERSION}
EOF

sudo=""
[ "${UID}" -ne "0" ] && sudo="sudo"


# ---------------------------------------------------------------------------------------------------------------------
# install required system packages

INTERACTIVE=1
PACKAGES_INSTALLER_OPTIONS="netdata"
NETDATA_INSTALLER_OPTIONS=""
NETDATA_UPDATES="-u"
SOURCE_DST="/usr/src"
while [ ! -z "${1}" ]
do
    if [ "${1}" = "all" ]
    then
        PACKAGES_INSTALLER_OPTIONS="netdata-all"
        shift 1
    elif [ "${1}" = "--dont-wait" -o "${1}" = "--non-interactive" ]
    then
        INTERACTIVE=0
        shift 1
    elif [ "${1}" = "--src-dir" ]
    then
        SOURCE_DST="${2}"
        # echo >&2 "netdata source will be installed at ${SOURCE_DST}/netdata.git"
        shift 2
    elif [ "${1}" = "--no-updates" ]
    then
        # echo >&2 "netdata will not auto-update"
        NETDATA_UPDATES=
        shift 1
    else
        break
    fi
done

if [ "${INTERACTIVE}" = "0" ]
then
    PACKAGES_INSTALLER_OPTIONS="--dont-wait --non-interactive ${PACKAGES_INSTALLER_OPTIONS}"
    NETDATA_INSTALLER_OPTIONS="--dont-wait"
fi

# echo "PACKAGES_INSTALLER_OPTIONS=${PACKAGES_INSTALLER_OPTIONS}"
# echo "NETDATA_INSTALLER_OPTIONS=${NETDATA_INSTALLER_OPTIONS} ${*}"

if [ "${OS}" = "GNU/Linux" -o "${SYSTEM}" = "Linux" ]
then
    if [ "${HAS_BASH4}" = "1" ]
    then
        tmp="$(mktemp /tmp/netdata-kickstart-XXXXXX)"
        url="https://raw.githubusercontent.com/firehol/netdata-demo-site/master/install-required-packages.sh"

        progress "Downloading script to detect required packages..."
        if [ ! -z "${curl}" ]
        then
            run ${curl} "${url}" >"${tmp}" || fatal "Cannot download ${url}"
        elif [ ! -z "${wget}" ]
        then
            run "${wget}" -O - "${url}" >"${tmp}" || fatal "Cannot download ${url}"
        else
            rm "${tmp}"
            fatal "I need curl or wget to proceed, but neither is available on this system."
        fi

        ask=0
        if [ -s "${tmp}" ]
        then
            progress "Running downloaded script to detect required packages..."
            run ${sudo} "${bash}" "${tmp}" ${PACKAGES_INSTALLER_OPTIONS} || ask=1
            rm "${tmp}"
        else
            rm "${tmp}"
            fatal "Downloaded script is empty..."
        fi

        if [ "${ask}" = "1" ]
        then
            echo >&2 "It failed to install all the required packages, but I can try to install netdata."
            read -p "Press ENTER to continue to netdata installation > "
            progress "OK, let's give it a try..."
        fi
    else
        echo >&2 "WARNING"
        echo >&2 "Cannot detect the packages to be installed in this system, without BASH v4+."
        echo >&2 "We can only attempt to install netdata..."
        echo >&2
    fi
else
    echo >&2 "WARNING"
    echo >&2 "Cannot detect the packages to be installed on a ${SYSTEM} - ${OS} system."
    echo >&2 "We can only attempt to install netdata..."
    echo >&2
fi


# ---------------------------------------------------------------------------------------------------------------------
# download netdata source

# this has to checked after we have installed the required packages
git="$(which_cmd git)"

NETDATA_SOURCE_DIR=
if [ ! -z "${git}" -a -x "${git}" ]
then
    [ ! -d "${SOURCE_DST}" ] && run ${sudo} mkdir -p "${SOURCE_DST}"

    if [ ! -d "${SOURCE_DST}/netdata.git" ]
    then
        progress "Downloading netdata source code..."
        run ${sudo} ${git} clone https://github.com/firehol/netdata.git "${SOURCE_DST}/netdata.git" || fatal "Cannot download netdata source"
        cd "${SOURCE_DST}/netdata.git" || fatal "Cannot cd to netdata source tree"
    else
        progress "Updating netdata source code..."
        cd "${SOURCE_DST}/netdata.git" || fatal "Cannot cd to netdata source tree"
        run ${sudo} ${git} fetch --all || fatal "Cannot fetch netdata source updates"
        run ${sudo} ${git} reset --hard origin/master || fatal "Cannot update netdata source tree"
    fi
    NETDATA_SOURCE_DIR="${SOURCE_DST}/netdata.git"
else
    fatal "Cannot find the command 'git' to download the netdata source code."
fi


# ---------------------------------------------------------------------------------------------------------------------
# install netdata from source

if [ ! -z "${NETDATA_SOURCE_DIR}" -a -d "${NETDATA_SOURCE_DIR}" ]
then
    cd "${NETDATA_SOURCE_DIR}" || fatal "Cannot cd to netdata source tree"

    install=0
    if [ -x netdata-updater.sh ]
    then
        # attempt to run the updater, to respect any compilation settings already in place
        progress "Re-installing netdata..."
        run ${sudo} ./netdata-updater.sh -f || install=1
    else
        install=1
    fi

    if [ "${install}" = "1" ]
    then
        if [ -x netdata-installer.sh ]
        then
            progress "Installing netdata..."
            run ${sudo} ./netdata-installer.sh ${NETDATA_UPDATES} ${NETDATA_INSTALLER_OPTIONS} "${@}" || \
                fatal "netdata-installer.sh exited with error"
        else
            fatal "Cannot install netdata from source (the source directory does not include netdata-installer.sh)."
        fi
    fi
else
    fatal "Cannot install netdata from source, on this system (cannot download the source code)."
fi
