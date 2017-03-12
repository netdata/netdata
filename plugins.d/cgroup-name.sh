#!/usr/bin/env bash

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
# GPL v3+
#
# Script to find a better name for cgroups
#

export PATH="${PATH}:/sbin:/usr/sbin:/usr/local/sbin"
export LC_ALL=C

# -----------------------------------------------------------------------------

PROGRAM_NAME="$(basename "${0}")"

logdate() {
    date "+%Y-%m-%d %H:%M:%S"
}

log() {
    local status="${1}"
    shift

    echo >&2 "$(logdate): ${PROGRAM_NAME}: ${status}: ${*}"

}

warning() {
    log WARNING "${@}"
}

error() {
    log ERROR "${@}"
}

info() {
    log INFO "${@}"
}

fatal() {
    log FATAL "${@}"
    exit 1
}

debug=0
debug() {
    [ $debug -eq 1 ] && log DEBUG "${@}"
}

# -----------------------------------------------------------------------------

NETDATA_CONFIG_DIR="${NETDATA_CONFIG_DIR-/etc/netdata}"
CONFIG="${NETDATA_CONFIG_DIR}/cgroups-names.conf"
CGROUP="${1}"
NAME=

# -----------------------------------------------------------------------------

if [ -z "${CGROUP}" ]
    then
    fatal "called without a cgroup name. Nothing to do."
fi

if [ -f "${CONFIG}" ]
    then
    NAME="$(grep "^${CGROUP} " "${CONFIG}" | sed "s/[[:space:]]\+/ /g" | cut -d ' ' -f 2)"
    if [ -z "${NAME}" ]
        then
        info "cannot find cgroup '${CGROUP}' in '${CONFIG}'."
    fi
#else
#   info "configuration file '${CONFIG}' is not available."
fi

function get_name_classic {
    local DOCKERID="$1"
    info "Running command: docker ps --filter=id=\"${DOCKERID}\" --format=\"{{.Names}}\""
    NAME="$( docker ps --filter=id="${DOCKERID}" --format="{{.Names}}" )"
    return 0
}

function get_name_api {
    local DOCKERID="$1"
    if [ ! -S "/var/run/docker.sock" ]
        then
        warning "Can't find /var/run/docker.sock"
        return 1
    fi
    info "Running API command: /containers/${DOCKERID}/json"
    JSON=$(echo -e "GET /containers/${DOCKERID}/json HTTP/1.0\r\n" | nc -U /var/run/docker.sock | egrep '^{.*')
    NAME=$(echo $JSON | jq -r .Name,.Config.Hostname | grep -v null | head -n1 | sed 's|^/||')
    return 0
}

if [ -z "${NAME}" ]
    then
    if [[ "${CGROUP}" =~ ^.*docker[-_/\.][a-fA-F0-9]+[-_\.]?.*$ ]]
        then
        # docker containers

        DOCKERID="$( echo "${CGROUP}" | sed "s|^.*docker[-_/]\([a-fA-F0-9]\+\)[-_\.]\?.*$|\1|" )"
        # echo "DOCKERID=${DOCKERID}"

        if [ ! -z "${DOCKERID}" -a \( ${#DOCKERID} -eq 64 -o ${#DOCKERID} -eq 12 \) ]
            then
            if hash docker 2>/dev/null
                then
                get_name_classic $DOCKERID
            else
                get_name_api $DOCKERID || get_name_classic $DOCKERID
            fi
            if [ -z "${NAME}" ]
                then
                warning "cannot find the name of docker container '${DOCKERID}'"
                NAME="${DOCKERID:0:12}"
            else
                info "docker container '${DOCKERID}' is named '${NAME}'"
            fi
        fi
    elif [[ "${CGROUP}" =~ machine.slice_machine.*-qemu ]]
        then
        # libvirtd / qemu virtual machines

        NAME="$(echo ${CGROUP} | sed 's/machine.slice_machine.*-qemu//; s/\/x2d//; s/\/x2d/\-/g; s/\.scope//g')"
    fi

    [ -z "${NAME}" ] && NAME="${CGROUP}"
    [ ${#NAME} -gt 100 ] && NAME="${NAME:0:100}"
fi

info "cgroup '${CGROUP}' is called '${NAME}'"
echo "${NAME}"
