#!/usr/bin/env sh

# -------------------------------------------------------------------------------------------------
# detect the kernel

KERNEL_NAME="$(uname -s)"
KERNEL_VERSION="$(uname -r)"
ARCHITECTURE="$(uname -m)"

# -------------------------------------------------------------------------------------------------
# detect the operating system

OS_DETECTION="unknown"
NAME="unknown"
VERSION="unknown"
VERSION_ID="unknown"
ID="unknown"
ID_LIKE="unknown"

if [ "${KERNEL_NAME}" = "Darwin" ]; then
	# Mac OS
	OIFS="$IFS"
	IFS=$'\n'
	set $(sw_vers) > /dev/null
	NAME=$(echo $1 | tr "\n\t" '  ' | sed -e 's/ProductName:[ ]*//' -e 's/[ ]*$//')
	VERSION=$(echo $2 | tr "\n\t" '  ' | sed -e 's/ProductVersion:[ ]*//' -e 's/[ ]*$//')
	ID="mac"
	ID_LIKE="mac"
	OS_DETECTION="sw_vers"
	IFS="$OIFS"
else
	if [ -f "/etc/os-release" ]; then
		OS_DETECTION="/etc/os-release"
		eval "$(grep -E "^(NAME|ID|ID_LIKE|VERSION|VERSION_ID)=" </etc/os-release)"
	fi

	if [ "${NAME}" = "unknown" ] || [ "${VERSION}" = "unknown" ] || [ "${ID}" = "unknown" ]; then
		if [ -f "/etc/lsb-release" ]; then
			if [ "${OS_DETECTION}" = "unknown" ]; then OS_DETECTION="/etc/lsb-release"; else OS_DETECTION="Mixed"; fi
			DISTRIB_ID="unknown"
			DISTRIB_RELEASE="unknown"
			DISTRIB_CODENAME="unknown"
			eval "$(grep -E "^(DISTRIB_ID|DISTRIB_RELEASE|DISTRIB_CODENAME)=" </etc/lsb-release)"
			if [ "${NAME}" = "unknown" ]; then NAME="${DISTRIB_ID}"; fi
			if [ "${VERSION}" = "unknown" ]; then VERSION="${DISTRIB_RELEASE}"; fi
			if [ "${ID}" = "unknown" ]; then ID="${DISTRIB_CODENAME}"; fi
		fi
		if [ -n "$(command -v lsb_release 2>/dev/null)" ]; then
			if [ "${OS_DETECTION}" = "unknown" ]; then OS_DETECTION="lsb_release"; else OS_DETECTION="Mixed"; fi
			if [ "${NAME}" = "unknown" ]; then NAME="$(lsb_release -is 2>/dev/null)"; fi
			if [ "${VERSION}" = "unknown" ]; then VERSION="$(lsb_release -rs 2>/dev/null)"; fi
			if [ "${ID}" = "unknown" ]; then ID="$(lsb_release -cs 2>/dev/null)"; fi
		fi
	fi
fi

# -------------------------------------------------------------------------------------------------
# detect the virtualization

VIRTUALIZATION="unknown"
VIRT_DETECTION="none"
CONTAINER="unknown"
CONT_DETECTION="none"

if [ -n "$(command -v systemd-detect-virt 2>/dev/null)" ]; then
	VIRTUALIZATION="$(systemd-detect-virt -v)"
	VIRT_DETECTION="systemd-detect-virt"
	CONTAINER="$(systemd-detect-virt -c)"
	CONT_DETECTION="systemd-detect-virt"
else
	if grep -q "^flags.*hypervisor" /proc/cpuinfo 2>/dev/null; then
		VIRTUALIZATION="hypervisor"
		VIRT_DETECTION="/proc/cpuinfo"
	fi
fi

# -------------------------------------------------------------------------------------------------
# detect containers with heuristics

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

echo "NETDATA_SYSTEM_OS_NAME=\"${NAME}\""
echo "NETDATA_SYSTEM_OS_ID=${ID}"
echo "NETDATA_SYSTEM_OS_ID_LIKE=${ID_LIKE}"
echo "NETDATA_SYSTEM_OS_VERSION=${VERSION}"
echo "NETDATA_SYSTEM_OS_VERSION_ID=${VERSION_ID}"
echo "NETDATA_SYSTEM_OS_DETECTION=${OS_DETECTION}"
echo "NETDATA_SYSTEM_KERNEL_NAME=${KERNEL_NAME}"
echo "NETDATA_SYSTEM_KERNEL_VERSION=${KERNEL_VERSION}"
echo "NETDATA_SYSTEM_ARCHITECTURE=${ARCHITECTURE}"
echo "NETDATA_SYSTEM_VIRTUALIZATION=${VIRTUALIZATION}"
echo "NETDATA_SYSTEM_VIRT_DETECTION=${VIRT_DETECTION}"
echo "NETDATA_SYSTEM_CONTAINER=${CONTAINER}"
echo "NETDATA_SYSTEM_CONTAINER_DETECTION=${CONT_DETECTION}"

