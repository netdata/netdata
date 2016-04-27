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
	NAME="$(cat "${CONFIG}" | grep "^${CGROUP} " | sed "s/[[:space:]]\+/ /g" | cut -d ' ' -f 2)"
	if [ -z "${NAME}" ]
		then
		echo >&2 "${0}: cannot find cgroup '${CGROUP}' in '${CONFIG}'."
	fi
else
	echo >&2 "${0}: configuration file '${CONFIG}' is not available."
fi

if [ -z "${NAME}" -a "${CGROUP:0:7}" = "docker/" ]
	then
	NAME="$(docker ps --filter=id="${CGROUP:7:64}" --format="{{.Names}}")"
	[ -z "${NAME}" ] && NAME="${CGROUP:0:19}"
	[ ${#NAME} -gt 20 ] && NAME="${NAME:0:20}"
fi

if [ -z "${NAME}" ]
	then
	if [ ${#CGROUP} -gt 20 ]
		then
		NAME="${CGROUP:0:20}"
	else
		NAME="${CGROUP}"
	fi
fi

echo >&2 "${0}: cgroup '${CGROUP}' is named as '${NAME}'"
echo "${NAME}"
