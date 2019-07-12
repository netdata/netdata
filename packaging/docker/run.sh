#!/usr/bin/env bash
#
# Entry point script for netdata
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis <paul@netdata.cloud>
set -e

echo "Netdata entrypoint script starting"
if [ ${RESCRAMBLE+x} ]; then
	echo "Reinstalling all packages to get the latest Polymorphic Linux scramble"
	apk upgrade --update-cache --available
fi

exec /usr/sbin/netdata -u "${DOCKER_USR}" -D -s /host -p "${NETDATA_PORT}" "$@"

echo "Netdata entrypoint script, completed!"
