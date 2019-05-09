#!/usr/bin/env bash
#
# TBD
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)
set -e

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup)
if [ ! -z $CWD ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/package_management/$(basename "$0") from top level directory of netdata git repository"
    echo "Docker build process aborted"
    exit 1
fi

# Check for presence of mandatory environment variables
if [ -z "${BUILD_STRING}" ]; then
	echo "No Distribution was defined. Make sure BUILD_STRING is set on the environment before running this script"
	exit 1
fi

if [ -z "${BUILD_ARCH}" ]; then
	echo "No container arch was defined. Make sure BUILD_ARCH is set on the environment before running this script"
	exit 1
fi

if [ -z "${BUILDER_NAME}" ]; then
	echo "No builder account and container name defined. Make sure BUILDER_NAME is set on the environment before running this script"
	exit 1
fi

if [ -z "${BUILD_DISTRO}" ]; then
	echo "No build distro information defined. Make sure BUILD_DISTRO is set on the environment before running this script"
	exit 1
fi

if [ -z "${BUILD_RELEASE}" ]; then
	echo "No build release information defined. Make sure BUILD_RELEASE is set on the environment before running this script"
	exit 1
fi

if [ -z "${PACKAGE_TYPE}" ]; then
	echo "No build release information defined. Make sure PACKAGE_TYPE is set on the environment before running this script"
	exit 1
fi

.travis/package_management/${PACKAGE_TYPE}/trigger_lxc_rpm_build.py "${CONTAINER_NAME}"

echo "Build process completed!"
