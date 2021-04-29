#!/usr/bin/env bash
# shellcheck disable=SC1117

# cgroup-network-helper.sh
# detect container and virtual machine interfaces
#
# (C) 2017 Costa Tsaousis
# SPDX-License-Identifier: GPL-3.0-or-later
#
# This script is called as root (by cgroup-network), with either a pid, or a cgroup path.
# It tries to find all the network interfaces that belong to the same cgroup.
#
# It supports several method for this detection:
#
# 1. cgroup-network (the binary father of this script) detects veth network interfaces,
#    by examining iflink and ifindex IDs and switching namespaces
#    (it also detects the interface name as it is used by the container).
#
# 2. this script, uses /proc/PID/fdinfo to find tun/tap network interfaces.
#
# 3. this script, calls virsh to find libvirt network interfaces.
#

# -----------------------------------------------------------------------------

# the system path is cleared by cgroup-network
# shellcheck source=/dev/null
[ -f /etc/profile ] && source /etc/profile

export LC_ALL=C

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

debug=${NETDATA_CGROUP_NETWORK_HELPER_DEBUG=0}
debug() {
    [ "${debug}" = "1" ] && log DEBUG "${@}"
}

# -----------------------------------------------------------------------------
# check for BASH v4+ (required for associative arrays)

[ $(( BASH_VERSINFO[0] )) -lt 4 ] && \
    fatal "BASH version 4 or later is required (this is ${BASH_VERSION})."

# -----------------------------------------------------------------------------
# parse the arguments

pid=
cgroup=
while [ ! -z "${1}" ]
do
    case "${1}" in
        --cgroup) cgroup="${2}"; shift 1;;
        --pid|-p) pid="${2}"; shift 1;;
        --debug|debug) debug=1;;
        *) fatal "Cannot understand argument '${1}'";;
    esac

    shift
done

if [ -z "${pid}" ] && [ -z "${cgroup}" ]
then
    fatal "Either --pid or --cgroup is required"
fi

# -----------------------------------------------------------------------------

set_source() {
    [ ${debug} -eq 1 ] && echo "SRC ${*}"
}


# -----------------------------------------------------------------------------
# veth interfaces via cgroup

# cgroup-network can detect veth interfaces by itself (written in C).
# If you seek for a shell version of what it does, check this:
# https://github.com/netdata/netdata/issues/474#issuecomment-317866709


# -----------------------------------------------------------------------------
# tun/tap interfaces via /proc/PID/fdinfo

# find any tun/tap devices linked to a pid
proc_pid_fdinfo_iff() {
    local p="${1}" # the pid

    debug "Searching for tun/tap interfaces for pid ${p}..."
    set_source "fdinfo"
    grep "^iff:.*" "${NETDATA_HOST_PREFIX}/proc/${p}/fdinfo"/* 2>/dev/null | cut -f 2
}

find_tun_tap_interfaces_for_cgroup() {
    local c="${1}" # the cgroup path
    [ -d "${c}/emulator" ] && c="${c}/emulator" # check for 'emulator' subdirectory
    c="${c}/cgroup.procs" # make full path 

    # for each pid of the cgroup
    # find any tun/tap devices linked to the pid
    if [ -f "${c}" ]
    then
        local p
        for p in $(< "${c}" )
        do
            proc_pid_fdinfo_iff "${p}"
        done
    else
        debug "Cannot find file '${c}', not searching for tun/tap interfaces."
    fi
}


# -----------------------------------------------------------------------------
# virsh domain network interfaces

virsh_cgroup_to_domain_name() {
    local c="${1}" # the cgroup path

    debug "extracting a possible virsh domain from cgroup ${c}..."

    # extract for the cgroup path
    sed -n -e "s|.*/machine-qemu\\\\x2d[0-9]\+\\\\x2d\(.*\)\.scope$|\1|p" \
           -e "s|.*/machine/\(.*\)\.libvirt-qemu$|\1|p" \
           <<EOF
${c}
EOF
}

virsh_find_all_interfaces_for_cgroup() {
    local c="${1}" # the cgroup path

    # the virsh command
    local virsh
    # shellcheck disable=SC2230
    virsh="$(which virsh 2>/dev/null || command -v virsh 2>/dev/null)"

    if [ ! -z "${virsh}" ]
    then
        local d
        d="$(virsh_cgroup_to_domain_name "${c}")"

        if [ ! -z "${d}" ]
        then
            debug "running: virsh domiflist ${d}; to find the network interfaces"

            # match only 'network' interfaces from virsh output

            set_source "virsh"
            "${virsh}" -r domiflist "${d}" |\
                sed -n \
                    -e "s|^\([^[:space:]]\+\)[[:space:]]\+network[[:space:]]\+\([^[:space:]]\+\)[[:space:]]\+[^[:space:]]\+[[:space:]]\+[^[:space:]]\+$|\1 \1_\2|p" \
                    -e "s|^\([^[:space:]]\+\)[[:space:]]\+bridge[[:space:]]\+\([^[:space:]]\+\)[[:space:]]\+[^[:space:]]\+[[:space:]]\+[^[:space:]]\+$|\1 \1_\2|p"
        else
            debug "no virsh domain extracted from cgroup ${c}"
        fi
    else
        debug "virsh command is not available"
    fi
}

# -----------------------------------------------------------------------------
# netnsid detected interfaces

netnsid_find_all_interfaces_for_pid() {
    local pid="${1}"
    [ -z "${pid}" ] && return 1

    local nsid=$(lsns -t net -p ${pid} -o NETNSID -nr)
    [ -z "${nsid}" -o "${nsid}" = "unassigned" ] && return 1

    set_source "netnsid"
    ip link show |\
        grep -B 1 -E " link-netnsid ${nsid}($| )" |\
        sed -n -e "s|^[[:space:]]*[0-9]\+:[[:space:]]\+\([A-Za-z0-9_]\+\)\(@[A-Za-z0-9_]\+\)*:[[:space:]].*$|\1|p"
}

netnsid_find_all_interfaces_for_cgroup() {
    local c="${1}" # the cgroup path

    # for each pid of the cgroup
    # find any tun/tap devices linked to the pid
    if [ -f "${c}/cgroup.procs" ]
    then
        local p
        for p in $(< "${c}/cgroup.procs" )
        do
            netnsid_find_all_interfaces_for_pid "${p}"
        done
    else
        debug "Cannot find file '${c}/cgroup.procs', not searching for netnsid interfaces."
    fi
}

# -----------------------------------------------------------------------------

find_all_interfaces_of_pid_or_cgroup() {
    local p="${1}" c="${2}" # the pid and the cgroup path

    if [ ! -z "${pid}" ]
    then
        # we have been called with a pid

        proc_pid_fdinfo_iff "${p}"
        netnsid_find_all_interfaces_for_pid "${p}"

    elif [ ! -z "${c}" ]
    then
        # we have been called with a cgroup

        info "searching for network interfaces of cgroup '${c}'"

        find_tun_tap_interfaces_for_cgroup "${c}"
        virsh_find_all_interfaces_for_cgroup "${c}"
        netnsid_find_all_interfaces_for_cgroup "${c}"

    else

        error "Either a pid or a cgroup path is needed"
        return 1

    fi

    return 0
}

# -----------------------------------------------------------------------------

# an associative array to store the interfaces
# the index is the interface name as seen by the host
# the value is the interface name as seen by the guest / container
declare -A devs=()

# store all interfaces found in the associative array
# this will also give the unique devices, as seen by the host
last_src=
# shellcheck disable=SC2162
while read host_device guest_device
do
    [ -z "${host_device}" ] && continue

    [ "${host_device}" = "SRC" ] && last_src="${guest_device}" && continue

    # the default guest_device is the host_device
    [ -z "${guest_device}" ] && guest_device="${host_device}"

    # when we run in debug, show the source
    debug "Found host device '${host_device}', guest device '${guest_device}', detected via '${last_src}'"

    if [ -z "${devs[${host_device}]}" ] || [ "${devs[${host_device}]}" = "${host_device}" ]; then
        devs[${host_device}]="${guest_device}"
    fi

done < <( find_all_interfaces_of_pid_or_cgroup "${pid}" "${cgroup}" )

# print the interfaces found, in the format netdata expects them
found=0
for x in "${!devs[@]}"
do
    found=$((found + 1))
    echo "${x} ${devs[${x}]}"
done

debug "found ${found} network interfaces for pid '${pid}', cgroup '${cgroup}', run as ${USER}, ${UID}"

# let netdata know if we found any
[ ${found} -eq 0 ] && exit 1
exit 0
