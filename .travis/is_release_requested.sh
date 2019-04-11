#!/bin/bash
#
# This script is designed to retrieve current commit message
# and identify whether we are looking at a release request my the committer.
#
# Accepted keywords that would indicate a release are:
# [netdata release candidate]
# [netdata patch release]
# [netdata minor release]
# [netdata major release]
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)
set -e

IS_RELEASE=1
IS_NOT_RELEASE=0

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup || echo "")
if [ -n "${CWD}" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/$(basename "$0") from top level directory of netdata git repository"
    echo "Changelog generation process aborted"
    exit 1
fi

echo "Retrieving last commit message from the repo"
LAST_COMMIT_MESSAGE=$(git log -1 --pretty=%B)

echo "Travis knows TRAVIS_COMMIT_MESSAGE: ${TRAVIS_COMMIT_MESSAGE}"
echo "GIT last commit message: ${LAST_COMMIT_MESSAGE}"

case "${LAST_COMMIT_MESSAGE}" in
*"[netdata patch release]"*|*"[netdata minor release]"*|*"[netdata major release]"*|*"[netdata release candidate]"*|*"[netdata release candidate]"*)
echo "this is indeed a relase request, notifying travis"
;;
*)
echo "No special keyword detected, this is not a release request"
exit $IS_NOT_RELEASE
;;
esac

exit $IS_RELEASE
