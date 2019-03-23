#!/bin/bash
#
# Mechanism to validate kickstart files integrity status
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)

KICKSTART_URL="https://my-netdata.io/kickstart.sh"
KICKSTART_MD5="e051d1baed4c69653b5d3e4588696466"

KICKSTART_STATIC64_URL="https://my-netdata.io/kickstart-static64.sh"
KICKSTART_STATIC64_MD5="8e6df9b6f6cc7de0d73f6e5e51a3c8c2"

echo "Starting integrity validation of kickstart files"
set -e

echo "Checking ${KICKSTART_URL} against ${KICKSTART_MD5}.."

CALCULATED_MD5="$(curl -Ss ${KICKSTART_URL} | md5sum | cut -d ' ' -f 1)"
if [ "$KICKSTART_MD5" == "$CALCULATED_MD5" ]; then
	echo "${KICKSTART_URL} looks fine"
else
	echo "${KICKSTART_URL} has changed, please update it"
	exit 1
fi

echo "Checking ${KICKSTART_STATIC64_URL} against ${KICKSTART_STATIC64_MD5}.."

CALCULATED_STATIC64_MD5="$(curl -Ss ${KICKSTART_STATIC64_URL} | md5sum | cut -d ' ' -f 1)"
if [ "${KICKSTART_STATIC64_MD5}" == "${CALCULATED_STATIC64_MD5}" ]; then
	echo "${KICKSTART_STATIC64_URL} looks fine"
else
	echo "${KICKSTART_STATIC64_URL} has changed, please update it"
	exit 2
fi

echo "Kickstart validation completed successfully!"
