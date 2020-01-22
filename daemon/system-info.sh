#!/usr/bin/env sh

# -------------------------------------------------------------------------------------------------
# detect the kernel

KERNEL_NAME="$(uname -s)"
KERNEL_VERSION="$(uname -r)"
ARCHITECTURE="$(uname -m)"

# -------------------------------------------------------------------------------------------------
# detect the virtualization

if [ -z "${VIRTUALIZATION}" ]; then
    VIRTUALIZATION="unknown"
    VIRT_DETECTION="none"

    if [ -n "$(command -v systemd-detect-virt 2>/dev/null)" ]; then
            VIRTUALIZATION="$(systemd-detect-virt -v)"
            VIRT_DETECTION="systemd-detect-virt"
            CONTAINER="$(systemd-detect-virt -c)"
            CONT_DETECTION="systemd-detect-virt"
    else
            if grep -q "^flags.*hypervisor" /proc/cpuinfo 2>/dev/null; then
                    VIRTUALIZATION="hypervisor"
                    VIRT_DETECTION="/proc/cpuinfo"
            elif [ -n "$(command -v dmidecode)" ]; then
                    # Virtualization detection from https://unix.stackexchange.com/questions/89714/easy-way-to-determine-virtualization-technology
                    # This only works as root
                    if dmidecode -s system-product-name 2>/dev/null | grep -q "VMware\|Virtual\|KVM\|Bochs"; then
                            VIRTUALIZATION="$(dmidecode -s system-product-name)"
                            VIRT_DETECTION="dmidecode"
                    fi
            fi
    fi
else
    # Passed from outside - probably in docker run
    VIRT_DETECTION="provided"
fi

# -------------------------------------------------------------------------------------------------
# detect containers with heuristics

CONTAINER="unknown"
CONT_DETECTION="none"

if [ "${CONTAINER}" = "unknown" ]; then
        if [ -f /proc/1/sched ] ; then
                IFS='(, ' read -r process _ </proc/1/sched
                if [ "${process}" = "netdata" ]; then
                        CONTAINER="container"
                        CONT_DETECTION="process"
                fi
        fi
        # ubuntu and debian supply /bin/running-in-container
        # https://www.apt-browse.org/browse/ubuntu/trusty/main/i386/upstart/1.12.1-0ubuntu4/file/bin/running-in-container
        if /bin/running-in-container >/dev/null 2>&1; then
                CONTAINER="container"
                CONT_DETECTION="/bin/running-in-container"
        fi

        # lxc sets environment variable 'container'
        #shellcheck disable=SC2154
        if [ -n "${container}" ]; then
                CONTAINER="lxc"
                CONT_DETECTION="containerenv"
        fi

        # docker creates /.dockerenv
        # http://stackoverflow.com/a/25518345
        if [ -f "/.dockerenv" ]; then
                CONTAINER="docker"
                CONT_DETECTION="dockerenv"
        fi

fi

# -------------------------------------------------------------------------------------------------
# detect the operating system

# Initially assume all OS detection values are for a container, these are moved later if we are bare-metal

CONTAINER_OS_DETECTION="unknown"
CONTAINER_NAME="unknown"
CONTAINER_VERSION="unknown"
CONTAINER_VERSION_ID="unknown"
CONTAINER_ID="unknown"
CONTAINER_ID_LIKE="unknown"

if [ "${KERNEL_NAME}" = "Darwin" ]; then
        CONTAINER_ID=$(sw_vers -productName)
        CONTAINER_ID_LIKE="mac"
        CONTAINER_NAME="mac"
        CONTAINER_VERSION=$(sw_vers -productVersion)
        CONTAINER_OS_DETECTION="sw_vers"
elif [ "${KERNEL_NAME}" = "FreeBSD" ]; then
        CONTAINER_ID="FreeBSD"
        CONTAINER_ID_LIKE="FreeBSD"
        CONTAINER_NAME="FreeBSD"
        CONTAINER_OS_DETECTION="uname"
        CONTAINER_VERSION=$(uname -r)
        KERNEL_VERSION=$(uname -K)
else
        if [ -f "/etc/os-release" ]; then
                eval "$(grep -E "^(NAME|ID|ID_LIKE|VERSION|VERSION_ID)=" </etc/os-release | sed 's/^/CONTAINER_/')"
                CONTAINER_OS_DETECTION="/etc/os-release"
        fi

        if [ "${NAME}" = "unknown" ] || [ "${VERSION}" = "unknown" ] || [ "${ID}" = "unknown" ]; then
                if [ -f "/etc/lsb-release" ]; then
                        if [ "${OS_DETECTION}" = "unknown" ]; then
                                CONTAINER_OS_DETECTION="/etc/lsb-release"
                        else
                                CONTAINER_OS_DETECTION="Mixed"
                        fi
                        DISTRIB_ID="unknown"
                        DISTRIB_RELEASE="unknown"
                        DISTRIB_CODENAME="unknown"
                        eval "$(grep -E "^(DISTRIB_ID|DISTRIB_RELEASE|DISTRIB_CODENAME)=" </etc/lsb-release)"
                        if [ "${NAME}" = "unknown" ]; then CONTAINER_NAME="${DISTRIB_ID}"; fi
                        if [ "${VERSION}" = "unknown" ]; then CONTAINER_VERSION="${DISTRIB_RELEASE}"; fi
                        if [ "${ID}" = "unknown" ]; then CONTAINER_ID="${DISTRIB_CODENAME}"; fi
                fi
                if [ -n "$(command -v lsb_release 2>/dev/null)" ]; then
                        if [ "${OS_DETECTION}" = "unknown" ]; then 
                                CONTAINER_OS_DETECTION="lsb_release"
                        else 
                                CONTAINER_OS_DETECTION="Mixed"
                        fi
                        if [ "${NAME}" = "unknown" ]; then CONTAINER_NAME="$(lsb_release -is 2>/dev/null)"; fi
                        if [ "${VERSION}" = "unknown" ]; then CONTAINER_VERSION="$(lsb_release -rs 2>/dev/null)"; fi
                        if [ "${ID}" = "unknown" ]; then CONTAINER_ID="$(lsb_release -cs 2>/dev/null)"; fi
                fi
        fi
fi

# If Netdata is not running in a container then use the local detection as the host
HOST_OS_DETECTION="unknown"
HOST_NAME="unknown"
HOST_VERSION="unknown"
HOST_VERSION_ID="unknown"
HOST_ID="unknown"
HOST_ID_LIKE="unknown"
if [ "${CONTAINER}" = "unknown" ]; then
        for v in NAME ID ID_LIKE VERSION VERSION_ID OS_DETECTION; do
                eval "HOST_$v=\$CONTAINER_$v; CONTAINER_$v=none"
        done
else
# Otherwise try and use a user-supplied bind-mount into the container to resolve the host details
        if [ -e "/host/etc/os-release" ]; then
                OS_DETECTION="/etc/os-release"
                eval "$(grep -E "^(NAME|ID|ID_LIKE|VERSION|VERSION_ID)=" </host/etc/os-release | sed 's/^/HOST_/')"
                HOST_OS_DETECTION="/host/etc/os-release"
        fi
        if [ "${HOST_NAME}" = "unknown" ] || [ "${HOST_VERSION}" = "unknown" ] || [ "${HOST_ID}" = "unknown" ]; then
                if [ -f "/host/etc/lsb-release" ]; then
                        if [ "${HOST_OS_DETECTION}" = "unknown" ]; then
                                HOST_OS_DETECTION="/etc/lsb-release"
                        else
                                HOST_OS_DETECTION="Mixed"
                        fi
                        DISTRIB_ID="unknown"
                        DISTRIB_RELEASE="unknown"
                        DISTRIB_CODENAME="unknown"
                        eval "$(grep -E "^(DISTRIB_ID|DISTRIB_RELEASE|DISTRIB_CODENAME)=" </etc/lsb-release)"
                        if [ "${HOST_NAME}" = "unknown" ]; then HOST_NAME="${DISTRIB_ID}"; fi
                        if [ "${HOST_VERSION}" = "unknown" ]; then HOST_VERSION="${DISTRIB_RELEASE}"; fi
                        if [ "${HOST_ID}" = "unknown" ]; then HOST_ID="${DISTRIB_CODENAME}"; fi
                fi
        fi
fi


echo "NETDATA_CONTAINER_OS_NAME=${CONTAINER_NAME}"
echo "NETDATA_CONTAINER_OS_ID=${CONTAINER_ID}"
echo "NETDATA_CONTAINER_OS_ID_LIKE=${CONTAINER_ID_LIKE}"
echo "NETDATA_CONTAINER_OS_VERSION=${CONTAINER_VERSION}"
echo "NETDATA_CONTAINER_OS_VERSION_ID=${CONTAINER_VERSION_ID}"
echo "NETDATA_CONTAINER_OS_DETECTION=${CONTAINER_OS_DETECTION}"
echo "NETDATA_HOST_OS_NAME=${HOST_NAME}"
echo "NETDATA_HOST_OS_ID=${HOST_ID}"
echo "NETDATA_HOST_OS_ID_LIKE=${HOST_ID_LIKE}"
echo "NETDATA_HOST_OS_VERSION=${HOST_VERSION}"
echo "NETDATA_HOST_OS_VERSION_ID=${HOST_VERSION_ID}"
echo "NETDATA_HOST_OS_DETECTION=${HOST_OS_DETECTION}"
echo "NETDATA_SYSTEM_KERNEL_NAME=${KERNEL_NAME}"
echo "NETDATA_SYSTEM_KERNEL_VERSION=${KERNEL_VERSION}"
echo "NETDATA_SYSTEM_ARCHITECTURE=${ARCHITECTURE}"
echo "NETDATA_SYSTEM_VIRTUALIZATION=${VIRTUALIZATION}"
echo "NETDATA_SYSTEM_VIRT_DETECTION=${VIRT_DETECTION}"
echo "NETDATA_SYSTEM_CONTAINER=${CONTAINER}"
echo "NETDATA_SYSTEM_CONTAINER_DETECTION=${CONT_DETECTION}"

