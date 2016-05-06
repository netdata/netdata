#!/bin/bash

export PATH="${PATH}:/sbin:/usr/sbin:/usr/local/sbin"
export LC_ALL=C

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
#else
#	echo >&2 "${0}: configuration file '${CONFIG}' is not available."
fi

function get_name_classic {
	DOCKERID=$1
	echo >&2 "Running command: docker ps --filter=id=\"${DOCKERID}\" --format=\"{{.Names}}\""
	NAME="$( docker ps --filter=id="${DOCKERID}" --format="{{.Names}}" )"
}

function get_name_api {
	DOCKERID=$1
	if [ ! -S "/var/run/docker.sock" ]
		then
		echo >&2 "Can't find /var/run/docker.sock"
		return
	fi
	echo >&2 "Running API command: /containers/${DOCKERID}/json"
	JSON=$(echo -e "GET /containers/${DOCKERID}/json HTTP/1.0\r\n" | nc -U /var/run/docker.sock | egrep '^{.*')
	NAME=$(echo $JSON | jq -r .Name,.Config.Hostname | grep -v null | head -n1 | sed 's|^/||')
}

if [ -z "${NAME}" ]
	then
	if [[ "${CGROUP}" =~ ^.*docker[-/\.][a-fA-F0-9]+[-\.]?.*$ ]]
		then
		DOCKERID="$( echo "${CGROUP}" | sed "s|^.*docker[-/]\([a-fA-F0-9]\+\)[-\.]\?.*$|\1|" )"

		if [ ! -z "${DOCKERID}" -a \( ${#DOCKERID} -eq 64 -o ${#DOCKERID} -eq 12 \) ]
			then
			if hash docker 2>/dev/null
				then
				get_name_classic $DOCKERID
			else
				get_name_api $DOCKERID
			fi
			if [ -z "${NAME}" ]
				then
				echo >&2 "Cannot find the name of docker container '${DOCKERID}'"
				NAME="${DOCKERID:0:12}"
			else
				echo >&2 "Docker container '${DOCKERID}' is named '${NAME}'"
			fi
		fi
	fi

	[ -z "${NAME}" ] && NAME="${CGROUP}"
	[ ${#NAME} -gt 50 ] && NAME="${NAME:0:50}"
fi

echo >&2 "${0}: cgroup '${CGROUP}' is called '${NAME}'"
echo "${NAME}"
