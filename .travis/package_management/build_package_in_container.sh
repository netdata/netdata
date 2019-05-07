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

.travis/package_management/trigger_lxc_rpm_build.py "${BUILDER_NAME}.${BUILD_DISTRO}${BUILD_RELEASE}.${BUILD_ARCH}"

echo "Build process completed!"
