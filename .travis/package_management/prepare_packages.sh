#!/usr/bin/env bash
#
# Utility that gathers generated packages,
# puts them together in a local folder for deploy facility to pick up
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)
#shellcheck disable=SC2068
set -e

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup)
if [ -n "$CWD" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/package_management/$(basename "$0") from top level directory of netdata git repository"
    echo "Package preparation aborted"
    exit 1
fi

export LXC_ROOT="/var/lib/lxc"

# Go through the containers created for packaging and pick up all generated packages
CREATED_CONTAINERS=$(ls -A "${LXC_ROOT}")
for d in ${CREATED_CONTAINERS[@]}; do
	echo "Picking up packaging contents from ${d}"

	# Pick up any RPMS from builder
	RPM_BUILD_PATH="${LXC_ROOT}/${d}/rootfs/home/${BUILDER_NAME}/rpmbuild"
	echo "Checking folder ${RPM_BUILD_PATH} for RPMS and SRPMS"

	if [ -d "${RPM_BUILD_PATH}/RPMS" ]; then
		echo "Copying any RPMS in '${RPM_BUILD_PATH}', copying over the following:"
		ls -ltrR "${RPM_BUILD_PATH}/RPMS"
		[[ -d "${RPM_BUILD_PATH}/RPMS/x86_64" ]] && cp -r "${RPM_BUILD_PATH}"/RPMS/x86_64/* "${PACKAGES_DIRECTORY}"
		[[ -d "${RPM_BUILD_PATH}/RPMS/i386" ]] && cp -r "${RPM_BUILD_PATH}"/RPMS/i386/* "${PACKAGES_DIRECTORY}"
		[[ -d "${RPM_BUILD_PATH}/RPMS/i686" ]] && cp -r "${RPM_BUILD_PATH}"/RPMS/i686/* "${PACKAGES_DIRECTORY}"
	fi

	if [ -d "${RPM_BUILD_PATH}/SRPMS" ]; then
		echo "Copying any SRPMS in '${RPM_BUILD_PATH}', copying over the following:"
		ls -ltrR "${RPM_BUILD_PATH}/SRPMS"
		[[ -d "${RPM_BUILD_PATH}/SRPMS/x86_64" ]] && cp -r "${RPM_BUILD_PATH}"/SRPMS/x86_64/* "${PACKAGES_DIRECTORY}"
		[[ -d "${RPM_BUILD_PATH}/SRPMS/i386" ]] && cp -r "${RPM_BUILD_PATH}"/SRPMS/i386/* "${PACKAGES_DIRECTORY}"
		[[ -d "${RPM_BUILD_PATH}/SRPMS/i686" ]] && cp -r "${RPM_BUILD_PATH}"/SRPMS/i686/* "${PACKAGES_DIRECTORY}"
	fi

	# Pick up any DEBs from builder
	DEB_BUILD_PATH="${d}/home/${BUILDER_NAME}/build-area"
	echo "Checking folder ${DEB_BUILD_PATH} for DEB packages"
	#TODO: During debian clean up we 'll fill this up

done

chmod -R 777 "${PACKAGES_DIRECTORY}"
echo "Packaging contents ready to ship!"
