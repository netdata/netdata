#!/usr/bin/env bash
#
# This script generates an LXC container and starts it up
# Once the script completes successfully, a container has become available for usage
# The container image to be used and the container name to be set, are part of variables 
# that must be present for the script to work
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)
# shellcheck disable=SC1091
set -e

source .travis/package_management/functions.sh || (echo "Failed to load packaging library" && exit 1)

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup)
if [ -n "$CWD" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/package_management/$(basename "$0") from top level directory of netdata git repository"
    echo "LXC Container creation aborted"
    exit 1
fi

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

echo "Creating LXC container ${BUILDER_NAME}/${BUILD_STRING}/${BUILD_ARCH}...."

case "${BUILD_ARCH}" in
"all")
	# i386
	echo "Creating LXC Container for i386.."
	export CONTAINER_NAME="${BUILDER_NAME}-${BUILD_DISTRO}${BUILD_RELEASE}-i386"
	export LXC_CONTAINER_ROOT="/var/lib/lxc/${CONTAINER_NAME}/rootfs"
	lxc-create -n "${CONTAINER_NAME}" -t "download" -- --dist "${BUILD_DISTRO}" --release "${BUILD_RELEASE}" --arch "i386" --no-validate

	echo "Container(s) ready. Configuring container(s).."
	.travis/package_management/configure_${PACKAGE_TYPE}_lxc_environment.py "${CONTAINER_NAME}"

	# amd64
	echo "Creating LXC Container for amd64.."
	export CONTAINER_NAME="${BUILDER_NAME}-${BUILD_DISTRO}${BUILD_RELEASE}-amd64"
	export LXC_CONTAINER_ROOT="/var/lib/lxc/${CONTAINER_NAME}/rootfs"
	lxc-create -n "${CONTAINER_NAME}" -t "download" -- --dist "${BUILD_DISTRO}" --release "${BUILD_RELEASE}" --arch "amd64" --no-validate

	echo "Container(s) ready. Configuring container(s).."
	.travis/package_management/configure_${PACKAGE_TYPE}_lxc_environment.py "${CONTAINER_NAME}"

	# arm64
	echo "Creating LXC Container for arm64.."
	export CONTAINER_NAME="${BUILDER_NAME}-${BUILD_DISTRO}${BUILD_RELEASE}-arm64"
	export LXC_CONTAINER_ROOT="/var/lib/lxc/${CONTAINER_NAME}/rootfs"
	lxc-create -n "${CONTAINER_NAME}" -t "download" -- --dist "${BUILD_DISTRO}" --release "${BUILD_RELEASE}" --arch "arm64" --no-validate

	echo "Container(s) ready. Configuring container(s).."
	.travis/package_management/configure_${PACKAGE_TYPE}_lxc_environment.py "${CONTAINER_NAME}"
	;;
"i386"|"amd64"|"arm64")
	# amd64 or i386
	echo "Creating LXC Container for ${BUILD_ARCH}.."
	export CONTAINER_NAME="${BUILDER_NAME}-${BUILD_DISTRO}${BUILD_RELEASE}-${BUILD_ARCH}"
	export LXC_CONTAINER_ROOT="/var/lib/lxc/${CONTAINER_NAME}/rootfs"
	lxc-create -n "${CONTAINER_NAME}" -t "download" -- --dist "${BUILD_DISTRO}" --release "${BUILD_RELEASE}" --arch "${BUILD_ARCH}" --no-validate

	echo "Container(s) ready. Configuring container(s).."
	.travis/package_management/configure_${PACKAGE_TYPE}_lxc_environment.py "${CONTAINER_NAME}"
	;;
*)
	echo "Unknown BUILD_ARCH value '${BUILD_ARCH}' given, process failed"
	exit 1
	;;
esac

echo "..LXC creation complete!"
