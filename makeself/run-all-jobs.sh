#!/usr/bin/env bash

LC_ALL=C
umask 002

# be nice
renice 19 $$ >/dev/null 2>/dev/null

# prepare the environment for the jobs
export NETDATA_INSTALL_PATH="${1-/opt/netdata}"
export NETDATA_MAKESELF_PATH="$(pwd)"
export NETDATA_SOURCE_PATH="${NETDATA_MAKESELF_PATH}/.."

PROCESSORS=$(cat /proc/cpuinfo 2>/dev/null | grep ^processor | wc -l)
[ -z "${PROCESSORS}" -o $(( PROCESSORS )) -lt 1 ] && PROCESSORS=1
export PROCESSORS

# make sure ${NULL} is empty
export NULL=

. $(dirname "${0}")/functions.sh "${@}" || exit 1

for x in $(dirname "${0}")/*.install.sh
do
	progress "running ${x}"
	"${x}" "${NETDATA_INSTALL_PATH}"
done

