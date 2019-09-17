#!/usr/bin/env bash
#
# Build Judy from source, you need to run this script as root.
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)
set -e
JUDY_VER="1.0.5"
JUDY_DIR="/opt/judy-${JUDY_VER}"

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup)
if [ -n "$CWD" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/package_management/$(basename "$0") from top level directory of netdata git repository"
    echo "Build Judy package from source code failed"
    exit 1
fi

echo "Fetching judy source tarball"
wget -O /opt/judy.tar.gz http://downloads.sourceforge.net/project/judy/judy/Judy-${JUDY_VER}/Judy-${JUDY_VER}.tar.gz

echo "Entering /opt directory and extracting tarball"
cd /opt && tar -xf judy.tar.gz && rm judy.tar.gz

echo "Entering ${JUDY_DIR}"
cd "${JUDY_DIR}"

echo "Running configure"
CFLAGS="-O2 -s" CXXFLAGS="-O2 -s" ./configure

echo "Compiling and installing"
make && make install

echo "Done, enjoy Judy!"
