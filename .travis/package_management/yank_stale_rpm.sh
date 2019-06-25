#!/usr/bin/env bash
#
# This script is responsible for the removal of stale RPM/DEB files.
# It runs on the pre-deploy step and takes care of the removal of the files
# prior to the upload of the freshly built ones
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)
#shellcheck disable=SC2010,SC2068
set -e

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup)
if [ -n "$CWD" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/package_management/$(basename "$0") from top level directory of netdata git repository"
    echo "Package yanking cancelled"
    exit 1
fi

PACKAGES_DIR="$1"
DISTRO="$2"
PACKAGES_LIST="$(ls -AR "${PACKAGES_DIR}" | grep '\.rpm')"

if [ ! -d "${PACKAGES_DIR}" ] || [ -z "${PACKAGES_LIST}" ]; then
	echo "Folder ${PACKAGES_DIR} does not seem to be a valid directory or is empty. No packages to check for yanking"
	exit 1
fi

for pkg in ${PACKAGES_LIST[@]}; do
	echo "Attempting yank on ${pkg}.."
	.travis/package_management/package_cloud_wrapper.sh yank "${PACKAGING_USER}/${DEPLOY_REPO}/${DISTRO}" "${pkg}" || echo "Nothing to yank or error on ${pkg}"
done

