#!/bin/bash
#
# Mechanism to validate kickstart files integrity status
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pawel Krupa (pawel@netdata.cloud)
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)
set -e
README_DOC="packaging/installer/README.md"

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel 2> /dev/null || echo "")")
CWD="$(git rev-parse --show-cdup 2> /dev/null || echo "")"
if [ ! -z $CWD ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/$(basename "$0") from top level directory of netdata git repository"
    echo "Kickstart validation process aborted"
    exit 1
fi

for file in kickstart.sh kickstart-static64.sh; do
	CHECKSUM_IN_README=$(grep "$file" $README_DOC | grep md5sum | cut -d '"' -f2)
	KICKSTART_URL="https://my-netdata.io/$file"
	KICKSTART_MD5="$(md5sum "packaging/installer/$file" | cut -d' ' -f1)"
	CALCULATED_MD5="$(curl -Ss ${KICKSTART_URL} | md5sum | cut -d ' ' -f 1)"

	# Conditionally run the website validation
	if [ -z "${LOCAL_ONLY}" ]; then
		echo "Validating ${KICKSTART_URL} against ${KICKSTART_MD5}.."
		if [ "$KICKSTART_MD5" == "$CALCULATED_MD5" ]; then
			echo "${KICKSTART_URL} looks fine"
		else
			echo "${KICKSTART_URL} has changed, please update it"
			exit 1
		fi
	fi

	echo "Validating documentation for $file"
	if [ "$KICKSTART_MD5" != "$CHECKSUM_IN_README" ]; then
		echo "Invalid checksum for $file in $README_DOC."
		echo "checksum in docs: $CHECKSUM_IN_README"
		echo "current checksum: $KICKSTART_MD5"
		exit 2
	else
		echo "$file MD5Sum is well documented"
	fi

done
echo "No problems found, exiting succesfully!"
