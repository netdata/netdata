#!/bin/bash

export PATH="${PATH}:/sbin:/usr/sbin:/usr/local/sbin"

NETDATA_CONFIG_DIR="${NETDATA_CONFIG_DIR-/etc/netdata}"
CONFIG="${NETDATA_CONFIG_DIR}/cgroups-names.conf"
CGROUP="${1}"
NAME=

if [ -z "${CGROUP}" ]
	then
	echo >&2 "${0}: called without a cgroup name. Nothing to do."
	exit 1
fi

if [ -f "${CONFIG}" ]
	then
	NAME="$(cat "${CONFIG}" | grep "^${CGROUP}" | cut -d ' ' -f 2)"
	if [ -z "${NAME}" ]
		then
		echo >&2 "${0}: cannot find cgroup '${CGROUP}' in '${CONFIG}'."
	fi
else
	echo >&2 "${0}: configuration file '${CONFIG}' is not available."
fi

if [ -z "${NAME}" ]
	then
	if [ ${#CGROUP} -gt 12 ]
		then
		NAME="${CGROUP:0:12}"
	else
		NAME="${CGROUP}"
	fi
fi

echo >&2 "${0}: cgroup '${CGROUP}' is named as '${NAME}'"
echo "${NAME}"
