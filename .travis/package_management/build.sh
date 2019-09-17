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
# pre/post options are after 1.18.8, is simpler to just check help for their existence than parsing version
if dpkg-buildpackage --help | grep "\-\-post\-clean" 2> /dev/null > /dev/null; then
	dpkg-buildpackage --post-clean --pre-clean --build=binary -us -uc
else
	dpkg-buildpackage -b -us -uc
fi

echo "DEB build script completed!"
