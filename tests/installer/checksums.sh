#!/bin/sh

#
# Mechanism to validate kickstart files integrity status
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pawel Krupa (pawel@netdata.cloud)
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)
set -e

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel 2>/dev/null || echo "")")
CWD="$(git rev-parse --show-cdup 2>/dev/null || echo "")"
if [ -n "$CWD" ] || [ "${TOP_LEVEL}" != "netdata" ]; then
	echo "Run as ./tests/installer/$(basename "$0") from top level directory of netdata git repository"
	echo "Kickstart validation process aborted"
	exit 1
fi

README_DOC="packaging/installer/README.md"

for file in kickstart.sh kickstart-static64.sh; do
	README_MD5=$(grep "$file" $README_DOC | grep md5sum | cut -d '"' -f2)
	KICKSTART_URL="https://my-netdata.io/$file"
	KICKSTART="packaging/installer/$file"
	KICKSTART_MD5="$(md5sum "${KICKSTART}" | cut -d' ' -f1)"
	CALCULATED_MD5="$(curl -Ss ${KICKSTART_URL} | md5sum | cut -d ' ' -f 1)"

	# Conditionally run the website validation
	if [ -z "${LOCAL_ONLY}" ]; then
		echo "Validating ${KICKSTART_URL} against local file ${KICKSTART} with MD5 ${KICKSTART_MD5}.."
		if [ "$KICKSTART_MD5" = "$CALCULATED_MD5" ]; then
			echo "${KICKSTART_URL} looks fine"
		else
			echo "${KICKSTART_URL} md5sum does not match local file, it needs to be updated"
			exit 2
		fi
	fi

	echo "Validating documentation for $file"
	if [ "$KICKSTART_MD5" != "$README_MD5" ]; then
		echo "Invalid checksum for $file in $README_DOC."
		echo "checksum in docs: $README_MD5"
		echo "current checksum: $KICKSTART_MD5"
		exit 2
	else
		echo "$file MD5Sum is well documented"
	fi

done
echo "No problems found, exiting succesfully!"
