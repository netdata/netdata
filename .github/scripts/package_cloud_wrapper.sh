#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# This is a tool to help removal of packages from packagecloud.io
# It utilizes the package_cloud utility provided from packagecloud.io
#
# Depends on:
# 1) package cloud gem (detects absence and installs it)
#
# Requires:
# 1) PKG_CLOUD_TOKEN variable exported
# 2) To properly install package_cloud when not found, it requires: ruby gcc gcc-c++ ruby-devel
#
#shellcheck disable=SC2068,SC2145

set -e
PKG_CLOUD_CONFIG="$HOME/.package_cloud_configuration.cfg"

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup)
if [ -n "$CWD" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .github/scripts/$(basename "$0") from top level directory of netdata git repository"
    echo "Docker build process aborted"
    exit 1
fi

# Install dependency if not there
if ! command -v package_cloud > /dev/null 2>&1; then
	echo "No package cloud gem found, installing"
	sudo gem install -V package_cloud || (echo "Package cloud installation failed. you might want to check if required dependencies are there (ruby gcc gcc-c++ ruby-devel)" && exit 1)
else
	echo "Found package_cloud gem, continuing"
fi

# Check for required token and prepare config
if [ -z "${PKG_CLOUD_TOKEN}" ]; then
	echo "Please set PKG_CLOUD_TOKEN to be able to use ${0}"
	exit 1
fi
echo "{\"url\":\"https://packagecloud.io\",\"token\":\"${PKG_CLOUD_TOKEN}\"}" > "${PKG_CLOUD_CONFIG}"

echo "Executing package_cloud with config ${PKG_CLOUD_CONFIG} and parameters $@"
package_cloud $@ --config="${PKG_CLOUD_CONFIG}"

rm -rf "${PKG_CLOUD_CONFIG}"
echo "Done!"
