#!/usr/bin/env bash
#
# Script to handle package cloud retention policy
# Our open source subscription is limited,
# so we use this sript to control the number of packages maintained historically
#
# Dependencies:
# - PACKAGE_CLOUD_RETENTION_DAYS
#   This is to indicate for how many days back we want to maintain the various RPM and DEB packages on package cloud
#
# Copyright : SPDX-License-Identifier: GPL-3.0-or-later
#
# Author    : Pavlos Emm. Katsoulakis <paul@netdata.cloud>
#
set -e

delete_files_for_version() {
	local v="$1"

	# Delete the selected filenames in version
	FILES_IN_VERSION=$(jq --sort-keys --arg v "${v}" '.[] | select ( .version | contains($v))' "${PKG_LIST_FILE}" | grep filename | cut -d':' -f 2)

	# Iterate through the files and delete them
	for pkg in ${FILES_IN_VERSION/\\n/}; do
		pkg=${pkg/,/}
		pkg=${pkg/\"/}
		pkg=${pkg/\"/}
		echo "Attempting yank on ${pkg}.."
		.travis/package_management/package_cloud_wrapper.sh yank "${PACKAGING_USER}/${DEPLOY_REPO}" "${pkg}" || echo "Nothing to yank or error on ${pkg}"
	done
}

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup)
if [ -n "$CWD" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/package_management/$(basename "$0") from top level directory of netdata git repository"
    echo "Old packages yanking cancelled"
    exit 1
fi

if [ -z "${PACKAGING_USER}" ]; then
	echo "No PACKAGING_USER variable found"
	exit 1
fi

if [ -z "${DEPLOY_REPO}" ]; then
	echo "No DEPLOY_REPO variable found"
	exit 1
fi

if [ -z ${PKG_CLOUD_TOKEN} ]; then
	echo "No PKG_CLOUD_TOKEN variable found"
	exit 1
fi

if [ -z ${PACKAGE_CLOUD_RETENTION_DAYS} ]; then
	echo "No PACKAGE_CLOUD_RETENTION_DAYS variable found"
	exit 1
fi

TMP_DIR="$(mktemp -d /tmp/netdata-old-package-yanking-XXXXXX)"
PKG_LIST_FILE="${TMP_DIR}/complete_package_list.json"
DATE_EPOCH="1970-01-01"
DATE_UNTIL_TO_DELETE=$(date --date="${PACKAGE_CLOUD_RETENTION_DAYS} day ago" +%Y-%m-%d)


echo "Created temp directory: ${TMP_DIR}"
echo "We will be purging contents up until ${DATE_UNTIL_TO_DELETE}"

echo "Calling package could to retrieve all available packages on ${PACKAGING_USER}/${DEPLOY_REPO}"
curl -sS "https://${PKG_CLOUD_TOKEN}:@packagecloud.io/api/v1/repos/${PACKAGING_USER}/${DEPLOY_REPO}/packages.json" > "${PKG_LIST_FILE}"

# Get versions within the desired duration
#
VERSIONS_TO_PURGE=$(jq --arg s "${DATE_EPOCH}" --arg e "${DATE_UNTIL_TO_DELETE}" '
[($s, $e) | strptime("%Y-%m-%d")[0:3]] as $r
  | map(select(
        (.created_at[:19] | strptime("%Y-%m-%dT%H:%M:%S")[0:3]) as $d
          | $d >= $r[0] and $d <= $r[1]
))' "${PKG_LIST_FILE}" | grep '"version":' | sort -u | sed -e 's/ //g' | cut -d':' -f2)

echo "We will be deleting the following versions: ${VERSIONS_TO_PURGE}"
for v in ${VERSIONS_TO_PURGE/\n//}; do
	v=${v/\"/}
	v=${v/\"/}
	v=${v/,/}
	echo "Remove all files for version $v"
	delete_files_for_version "${v}"
done

# Done, clean up
[ -d "${TMP_DIR}" ] && rm -rf "${TMP_DIR}"
