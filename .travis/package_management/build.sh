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

echo "Linking debian -> contrib/debian"
ln -sf contrib/debian debian

echo "Executing dpkg-buildpackage"
if dpkg-buildpackage --version 2> /dev/null | grep -q "1.18"; then
	dpkg-buildpackage --post-clean --pre-clean --build=binary
else
	dpkg-buildpackage -b
fi

echo "DEB build script completed!"
