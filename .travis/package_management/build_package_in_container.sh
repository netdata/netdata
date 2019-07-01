#!/usr/bin/env bash
#
# Entry point for package build process
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)
#shellcheck disable=SC1091
set -e

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup)
if [ -n "$CWD" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/package_management/$(basename "$0") from top level directory of netdata git repository"
    echo "Docker build process aborted"
    exit 1
fi

source .travis/package_management/functions.sh || (echo "Failed to load packaging library" && exit 1)

# Check for presence of mandatory environment variables
if [ -z "${BUILD_STRING}" ]; then
	echo "No Distribution was defined. Make sure BUILD_STRING is set on the environment before running this script"
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

# Detect architecture and load extra variables needed
detect_arch_from_commit

case "${BUILD_ARCH}" in
"all")
	echo "* * * Building all architectures, amd64 and i386 * * *"
	echo "Building for amd64.."
	export CONTAINER_NAME="${BUILDER_NAME}-${BUILD_DISTRO}${BUILD_RELEASE}-amd64"
	export LXC_CONTAINER_ROOT="/var/lib/lxc/${CONTAINER_NAME}/rootfs"
	.travis/package_management/trigger_${PACKAGE_TYPE}_lxc_build.py "${CONTAINER_NAME}"

	echo "Building for arm64.."
	export CONTAINER_NAME="${BUILDER_NAME}-${BUILD_DISTRO}${BUILD_RELEASE}-arm64"
	export LXC_CONTAINER_ROOT="/var/lib/lxc/${CONTAINER_NAME}/rootfs"
	.travis/package_management/trigger_${PACKAGE_TYPE}_lxc_build.py "${CONTAINER_NAME}"

	echo "Building for i386.."
	export CONTAINER_NAME="${BUILDER_NAME}-${BUILD_DISTRO}${BUILD_RELEASE}-i386"
	export LXC_CONTAINER_ROOT="/var/lib/lxc/${CONTAINER_NAME}/rootfs"
	.travis/package_management/trigger_${PACKAGE_TYPE}_lxc_build.py "${CONTAINER_NAME}"

	;;
"amd64"|"arm64"|"i386")
	echo "Building for ${BUILD_ARCH}.."
	export CONTAINER_NAME="${BUILDER_NAME}-${BUILD_DISTRO}${BUILD_RELEASE}-${BUILD_ARCH}"
	export LXC_CONTAINER_ROOT="/var/lib/lxc/${CONTAINER_NAME}/rootfs"
	.travis/package_management/trigger_${PACKAGE_TYPE}_lxc_build.py "${CONTAINER_NAME}"
	;;
*)
	echo "Unknown build architecture '${BUILD_ARCH}', nothing to do for build"
	exit 1
	;;
esac

echo "Build process completed!"
