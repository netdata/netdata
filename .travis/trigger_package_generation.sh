#!/usr/bin/env bash
#
# Trigger .RPM and .DEB package generation processes
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author: Pavlos Emm. Katsoulakis <paul@netdata.cloud>
set -e
WAIT_TIME=15
BUILD_NIGHTLY="$1"

commit_change() {
	local ARCH="$1"
	local PKG="$2"

	echo "---- Committing ${ARCH} .${PKG} package generation ----"
	git commit --allow-empty -m "[Package ${ARCH} ${PKG}]${BUILD_NIGHTLY} Package build process trigger"
}

push_change() {
	local GIT_MAIL="bot@netdata.cloud"
	local GIT_USER="netdatabot"

	echo "---- Push changes to repository ----"
	git push "https://${GITHUB_TOKEN}:@$(git config --get remote.origin.url | sed -e 's/^https:\/\///')"
}

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup || echo "")
if [ -n "${CWD}" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/$(basename "$0") from top level directory of netdata git repository"
    echo "Changelog generation process aborted"
    exit 1
fi

echo "--- Initialize git configuration ---"
git checkout master
git fetch --all
git pull

commit_change "amd64" "DEB"
push_change

echo "---- Waiting for ${WAIT_TIME} seconds before triggering next process ----"
sleep "${WAIT_TIME}"

commit_channge "amd64" "RPM"
push_change

echo "---- Done! ----"
