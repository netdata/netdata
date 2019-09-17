#!/usr/bin/env bash
#
# Build libuv from source, you need to run this script as root.
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis <paul@netdata.cloud>
set -e
LIBUV_VERSION="v1.32.0"
# Their folder is libuv-1.32.0 while the tarball version is v1.32.0, so fix that until they fix it...
LIBUV_DIR="/opt/libuv-${LIBUV_VERSION/v/}"

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup)
if [ -n "$CWD" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/package_management/$(basename "$0") from top level directory of netdata git repository"
    echo "Build libuv package from source code failed"
    exit 1
fi

echo "Fetching libuv from github"
wget -O /opt/libuv.tar.gz "https://github.com/libuv/libuv/archive/${LIBUV_VERSION}.tar.gz"

echo "Entering /opt and extracting source"
cd /opt && tar -xf libuv.tar.gz && rm libuv.tar.gz

echo "Entering ${LIBUV_DIR}"
cd "${LIBUV_DIR}"

echo "Compiling and installing"
sh autogen.sh
./configure
make && make check && make install

echo "Done, enjoy libuv!"
