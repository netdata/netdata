#!/usr/bin/env sh

# Valid actions:
# - KICKSTART   - installed using the kickstart.sh script
# - KICKSTART64 - installed using the kickstart-static64.sh script
# - MAKESELF    - installed by running a makeself package
# - INSTALL     - installed using netdata-installer.sh
# - UPDATE      - updated using netdata-updater.sh
# - FATAL       - netdata exited due to a fatal condition
# - START       - netdata started

ACTION="${1}"
ACTION_RESULT="${2}"
ACTION_DATA="${3}"

# -------------------------------------------------------------------------------------------------
# detect the operating system

OS_DETECTION=
NAME=
VERSION=
VERSION_ID=
ID=
ID_LIKE=

if [ -f "/etc/os-release" ]; then
    OS_DETECTION="/etc/os-release"
    eval "$(grep -E "^(NAME|ID|ID_LIKE|VERSION|VERSION_ID)=" </etc/os-release)"
elif [ -f "/etc/lsb-release" ]; then
    OS_DETECTION="/etc/lsb-release"
    DISTRIB_ID=
    DISTRIB_RELEASE=
    DISTRIB_CODENAME=
    eval "$(grep -E "^(DISTRIB_ID|DISTRIB_RELEASE|DISTRIB_CODENAME)=" </etc/lsb-release)"
    NAME="${DISTRIB_ID}"
    VERSION="${DISTRIB_RELEASE}"
    ID="${DISTRIB_CODENAME}"
elif [ ! -z "$(command -v lsb_release 2>/dev/null)" ]; then
    OS_DETECTION="lsb_release"
    NAME="$(lsb_release -i 2>/dev/null | sed "s/[[:space:]]*:[[:space:]]*/:/g" | cut -d ':' -f 2)"
    VERSION_ID="$(lsb_release -f 2>/dev/null | sed "s/[[:space:]]*:[[:space:]]*/:/g" | cut -d ':' -f 2)"
    VERSION="$(lsb_release -c 2>/dev/null | sed "s/[[:space:]]*:[[:space:]]*/:/g" | cut -d ':' -f 2)"
fi

# -------------------------------------------------------------------------------------------------
# detect the kernel

KERNEL_NAME="$(uname -s)"
KERNEL_VERSION="$(uname -r)"
ARCHITECTURE="$(uname -m)"

# -------------------------------------------------------------------------------------------------
# detect the virtualization

VIRTUALIZATION="unknown"
VIRT_DETECTION="none"

if [ ! -z "$(command -v systemd-detect-virt 2>/dev/null)" ]
then
    VIRTUALIZATION="$(systemd-detect-virt)"
    VIRT_DETECTION="systemd-detect-virt"
else
    # /proc/1/sched exposes the host's pid of our init !
    # http://stackoverflow.com/a/37016302
    pid=$( head -n 1 </proc/1/sched 2>/dev/null | { IFS='(),#:' read name pid th threads; echo $pid; } )
    if [ ! -z "${pid}" ]; then
        pid=$(( pid + 0 ))
        [ ${pid} -ne 1 ] &&
            VIRTUALIZATION="container" &&
            VIRT_DETECTION="pid"
    fi

    # ubuntu and debian supply /bin/running-in-container
    # https://www.apt-browse.org/browse/ubuntu/trusty/main/i386/upstart/1.12.1-0ubuntu4/file/bin/running-in-container
    [ -x "/bin/running-in-container" ] &&
        "/bin/running-in-container" >/dev/null 2>&1 &&
        VIRTUALIZATION="container" &&
        VIRT_DETECTION="/bin/running-in-container"

    # lxc sets environment variable 'container'
    [ ! -z "${container}" ] &&
        VIRTUALIZATION="lxc" &&
        VIRT_DETECTION="containerenv"

    # docker creates /.dockerenv
    # http://stackoverflow.com/a/25518345
    [ -f "/.dockerenv" ] &&
        VIRTUALIZATION="docker" &&
        VIRT_DETECTION="dockerenv"
fi

# -------------------------------------------------------------------------------------------------
# check netdata version

if [ -z "${NETDATA_VERSION}" ]; then
    NETDATA_VERSION="$(netdata -V 2>&1 | cut -d ' ' -f 2)"
fi

# -------------------------------------------------------------------------------------------------
# check netdata unique id

if [ -z "${NETDATA_REGISTRY_UNIQUE_ID}" ]; then
    for dir in "$(netdata -W get global 'lib directory' '/var/lib/netdata' 2>/dev/null)" "/var/lib/netdata" "/opt/netdata/var/lib/netdata"; do
        if [ -f "${dir}/registry/netdata.public.unique.id" ]; then
            NETDATA_REGISTRY_UNIQUE_ID="$(cat "${dir}/registry/netdata.public.unique.id")"
            break
        fi
    done
fi

# -------------------------------------------------------------------------------------------------
# check opt-out

OPTOUT=
if [ -z "${NETDATA_CONFIG_DIR}" ]; then
    for dir in "$(netdata -W get global 'config directory' '/etc/netdata' 2>/dev/null)" "/etc/netdata" "/opt/netdata/etc/netdata"; do
        if [ -f "${dir}/.opt-out-from-anonymous-statistics" ]; then
            OPTOUT="$(cat "${dir}/.opt-out-from-anonymous-statistics")"
            break
        fi
    done
else
    OPTOUT="$(cat "${NETDATA_CONFIG_DIR}/.opt-out-from-anonymous-statistics")"
fi

if [ "${OPTOUT}" = "all" ] || ( [ "${OPTOUT}" = "usage" ] && [ "${ACTION}" = "START" ] ); then
    # the user does not want us to send statistics
    exit 0
fi

# -------------------------------------------------------------------------------------------------
# send the anonymous statistics to netdata

payload="$(cat <<EOF
{
    "v": 1,
    "agent": {
        "id": "${NETDATA_REGISTRY_UNIQUE_ID}",
        "version": "${NETDATA_VERSION}"
    },
    "os": {
        "name": "${NAME}",
        "id": "${ID}",
        "id_like": "${ID_LIKE}",
        "version": "${VERSION}",
        "version_id": "${VERSION_ID}",
        "detection": "${OS_DETECTION}"
    },
    "kernel": {
        "name": "${KERNEL_NAME}",
        "version": "${KERNEL_VERSION}",
        "arch": "${ARCHITECTURE}"
    },
    "virtualization": {
        "name": "${VIRTUALIZATION}",
        "detection": "${VIRT_DETECTION}"
    },
    "action": {
        "name": "${ACTION}",
        "result": "${ACTION_RESULT}",
        "data": "$(echo "${ACTION_DATA}" | tr '"' "'")"
    }
}
EOF
)"

# curl >/dev/null 2>&1 -X POST -Ss --max-time 5 --data-urlencode "payload=${payload}" "https://registry.my-netdata.io/log/anonymous-statistics"
echo "${payload}"
