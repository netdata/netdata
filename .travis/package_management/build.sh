#!/usr/bin/env bash

UNPACKAGED_NETDATA_PATH="$1"
LATEST_RELEASE_VERSION="$2"

if [ -z "${LATEST_RELEASE_VERSION}" ]; then
	echo "Parameter 'LATEST_RELEASE_VERSION' not defined"
	exit 1
fi

if [ -z "${UNPACKAGED_NETDATA_PATH}" ]; then
	echo "Parameter 'UNPACKAGED_NETDATA_PATH' not defined"
	exit 1
fi

echo "Running changelog generation mechanism since ${LATEST_RELEASE_VERSION}"

echo "Entering ${UNPACKAGED_NETDATA_PATH}"
cd "${UNPACKAGED_NETDATA_PATH}"

echo "Executing dpkg-buildpackage"
dpkg-buildpackage --host-arch amd64 --target-arch amd64 --post-clean --pre-clean --build=binary

echo "DEB build script completed!"
