#!/usr/bin/env bash

# netdata
# real-time performance and health monitoring, done right!
# (C) 2017 Costa Tsaousis <costa@tsaousis.gr>
# GPL v3+
#
# This script is a helper to allow netdata collect tc data.
# tc output parsing has been implemented in C, inside netdata
# This script allows setting names to dimensions.

export PATH="${PATH}:/sbin:/usr/sbin:/usr/local/sbin"
export LC_ALL=C


# -----------------------------------------------------------------------------
# logging functions

PROGRAM_FILE="$0"
PROGRAM_NAME="$(basename $0)"
PROGRAM_NAME="${PROGRAM_NAME/.plugin}"

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
# find /var/run/fireqos

# the default
fireqos_run_dir="/var/run/fireqos"

function realdir {
    local r="$1"
    local t=$(readlink "$r")

    while [ "$t" ]
        do
        r=$(cd $(dirname "$r") && cd $(dirname "$t") && pwd -P)/$(basename "$t")
        t=$(readlink "$r")
    done

    dirname "$r"
}

if [ ! -d "${fireqos_run_dir}" ]
    then

    # the fireqos executable - we will use it to find its config
    fireqos="$(which fireqos 2>/dev/null || command -v fireqos 2>/dev/null)"

    if [ ! -z "${fireqos}" ]
        then

        fireqos_exec_dir="$(realdir ${fireqos})"

        if [ ! -z "${fireqos_exec_dir}" -a "${fireqos_exec_dir}" != "." -a -f "${fireqos_exec_dir}/install.config" ]
            then

            LOCALSTATEDIR=
            source "${fireqos_exec_dir}/install.config"

            if [ -d "${LOCALSTATEDIR}/run/fireqos" ]
                then
                fireqos_run_dir="${LOCALSTATEDIR}/run/fireqos"
            else
                warning "FireQoS is installed as '${fireqos}', its installation config at '${fireqos_exec_dir}/install.config' specifies local state data at '${LOCALSTATEDIR}/run/fireqos', but this directory is not found or is not readable (check the permissions of its parents)."
            fi
        else
            warning "Although FireQoS is installed on this system as '${fireqos}', I cannot find/read its installation configuration at '${fireqos_exec_dir}/install.config'."
        fi
    else
        warning "FireQoS is not installed on this system. Use FireQoS to apply traffic QoS and expose the class names to netdata. Check https://github.com/firehol/netdata/wiki/You-should-install-QoS-on-all-your-servers"
    fi
fi

# -----------------------------------------------------------------------------

[ -z "${NETDATA_PLUGINS_DIR}" ] && NETDATA_PLUGINS_DIR="$(dirname "${0}")"
[ -z "${NETDATA_CONFIG_DIR}" ] && NETDATA_CONFIG_DIR="$(dirname "${0}")/../../../../etc/netdata"

plugins_dir="${NETDATA_PLUGINS_DIR}"
config_dir="${NETDATA_CONFIG_DIR}"
tc="$(which tc 2>/dev/null || command -v tc 2>/dev/null)"


# -----------------------------------------------------------------------------
# user configuration

# time in seconds to refresh QoS class/qdisc names
qos_get_class_names_every=120

# time in seconds to exit - netdata will restart the script
qos_exit_every=3600

# what to use? classes or qdiscs?
tc_show="qdisc" # can also be "class"


# -----------------------------------------------------------------------------
# check if we have a valid number for interval

t=${1}
update_every=$((t))
[ $((update_every)) -lt 1 ] && update_every=${NETDATA_UPDATE_EVERY}
[ $((update_every)) -lt 1 ] && update_every=1


# -----------------------------------------------------------------------------
# allow the user to override our defaults

if [ -f "${config_dir}/tc-qos-helper.conf" ]
    then
    source "${config_dir}/tc-qos-helper.conf"
fi

case "${tc_show}" in
    qdisc|class)
        ;;

    *)
        error "tc_show variable can be either 'qdisc' or 'class' but is set to '${tc_show}'. Assuming it is 'qdisc'."
        tc_show="qdisc"
        ;;
esac


# -----------------------------------------------------------------------------
# default sleep function

LOOPSLEEPMS_LASTWORK=0
loopsleepms() {
    sleep $1
}

# if found and included, this file overwrites loopsleepms()
# with a high resolution timer function for precise looping.
. "${plugins_dir}/loopsleepms.sh.inc"


# -----------------------------------------------------------------------------
# final checks we can run

if [ -z "${tc}" -o ! -x "${tc}" ]
    then
    fatal "cannot find command 'tc' in this system."
fi

tc_devices=
fix_names=

# -----------------------------------------------------------------------------

setclassname() {
    if [ "${tc_show}" = "qdisc" ]
        then
        echo "SETCLASSNAME $4 $2"
    else
        echo "SETCLASSNAME $3 $2"
    fi
}

show_tc_cls() {
    [ "${tc_show}" = "qdisc" ] && return 1

    local x="${1}"

    if [ -f /etc/iproute2/tc_cls ]
    then
        local classid name rest
        while read classid name rest
        do
            [ -z "${classid}" -o -z "${name}" -o "${classid}" = "#" -o "${name}" = "#" -o "${classid:0:1}" = "#" -o "${name:0:1}" = "#" ] && continue
            setclassname "" "${name}" "${classid}"
        done </etc/iproute2/tc_cls
        return 0
    fi
    return 1
}

show_fireqos_names() {
    local x="${1}" name n interface_dev interface_classes interface_classes_monitor

    if [ -f "${fireqos_run_dir}/ifaces/${x}" ]
    then
        name="$(<"${fireqos_run_dir}/ifaces/${x}")"
        echo "SETDEVICENAME ${name}"

        interface_dev=
        interface_classes=
        interface_classes_monitor=
        source "${fireqos_run_dir}/${name}.conf"
        for n in ${interface_classes_monitor}
        do
            setclassname ${n//|/ }
        done
        [ ! -z "${interface_dev}" ] && echo "SETDEVICEGROUP ${interface_dev}"

        return 0
    fi

    return 1
}

show_tc() {
    local x="${1}"

    echo "BEGIN ${x}"

    # netdata can parse the output of tc
    ${tc} -s ${tc_show} show dev ${x}

    # check FireQOS names for classes
    if [ ! -z "${fix_names}" ]
    then
        show_fireqos_names "${x}" || show_tc_cls "${x}"
    fi

    echo "END ${x}"
}

find_tc_devices() {
    local count=0 devs= dev rest l

    # find all the devices in the system
    # without forking
    while IFS=":| " read dev rest
    do
        count=$((count + 1))
        [ ${count} -le 2 ] && continue
        devs="${devs} ${dev}"
    done </proc/net/dev

    # from all the devices find the ones
    # that have QoS defined
    # unfortunately, one fork per device cannot be avoided
    tc_devices=
    for dev in ${devs}
    do
        l="$(${tc} class show dev ${dev} 2>/dev/null)"
        [ ! -z "${l}" ] && tc_devices="${tc_devices} ${dev}"
    done
}

# update devices and class names
# once every 2 minutes
names_every=$((qos_get_class_names_every / update_every))

# exit this script every hour
# it will be restarted automatically
exit_after=$((qos_exit_every / update_every))

c=0
gc=0
while [ 1 ]
do
    fix_names=
    c=$((c + 1))
    gc=$((gc + 1))

    if [ ${c} -le 1 -o ${c} -ge ${names_every} ]
    then
        c=1
        fix_names="YES"
        find_tc_devices
    fi

    for d in ${tc_devices}
    do
        show_tc ${d}
    done

    echo "WORKTIME ${LOOPSLEEPMS_LASTWORK}"

    loopsleepms ${update_every}

    [ ${gc} -gt ${exit_after} ] && exit 0
done
