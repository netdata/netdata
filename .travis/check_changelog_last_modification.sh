#!/usr/bin/env bash
#
# This scriptplet validates nightlies age and notifies is if it gets too old
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)

set -e

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup || echo "")
if [ -n "${CWD}" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/$(basename "$0") from top level directory of netdata git repository"
    echo "Changelog age checker exited abnormally"
    exit 1
fi

source tests/installer/slack.sh || echo "I could not load slack library"

LAST_MODIFICATION="$(git log -1 --pretty="format:%at" CHANGELOG.md)"
CURRENT_TIME="$(date +"%s")"
TWO_DAYS_IN_SECONDS=172800

DIFF=$((CURRENT_TIME - LAST_MODIFICATION))

echo "Checking CHANGELOG.md last modification time on GIT.."
echo "CHANGELOG.md timestamp: ${LAST_MODIFICATION}"
echo "Current timestamp: ${CURRENT_TIME}"
echo "Diff: ${DIFF}"

if [ ${DIFF} -gt ${TWO_DAYS_IN_SECONDS} ]; then
	echo "CHANGELOG.md is more than two days old!"
	post_message "TRAVIS_MESSAGE" "Hi <!here>, CHANGELOG.md was found more than two days old (Diff: ${DIFF} seconds)" "${NOTIF_CHANNEL}"
else
	echo "CHANGELOG.md is less than two days old, fine"
fi
