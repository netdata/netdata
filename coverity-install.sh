#!/usr/bin/env bash
# Coverity installation script
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author: Pavlos Emm. Katsoulakis (paul@netdata.cloud)

token="${COVERITY_SCAN_TOKEN}"
([ -z "${token}" ] && [ -f .coverity-token ]) && token="$(<.coverity-token)"
if [ -z "${token}" ]; then
	echo >&2 "Save the coverity token to .coverity-token or export it as COVERITY_SCAN_TOKEN."
	exit 1
fi

covbuild="$(which cov-build 2>/dev/null || command -v cov-build 2>/dev/null)"
([ -z "${covbuild}" ] && [ -f .coverity-build ]) && covbuild="$(<.coverity-build)"
if [ ! -z "${covbuild}" ]; then
	echo >&2 "Coverity already installed, nothing to do!"
	exit 0
fi

echo >&2 "Installing coverity..."
WORKDIR="/opt/coverity-source"
mkdir -p "${WORKDIR}"

curl -SL --data "token=${token}&project=${REPOSITORY}" https://scan.coverity.com/download/linux64 > "${WORKDIR}/coverity_tool.tar.gz"
if [ -f "${WORKDIR}/coverity_tool.tar.gz" ]; then
	tar -x -C "${WORKDIR}" -f "${WORKDIR}/coverity_tool.tar.gz"
	sudo mv "${WORKDIR}/cov-analysis-linux64-2019.03" /opt/coverity
	export PATH=${PATH}:/opt/coverity/bin/
else
	echo "Failed to download coverity tool tarball!"
fi

# Validate the installation
covbuild="$(which cov-build 2>/dev/null || command -v cov-build 2>/dev/null)"
if [ -z "$covbuild" ]; then
	echo "Failed to install coverity!"
	exit 1
else
	echo >&2 "Coverity scan installed!"
fi
