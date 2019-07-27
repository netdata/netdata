#!/bin/bash
#
# This is the nightly changelog generation script
# It is responsible for two major activities:
# 1) Update packaging/version with the current nightly version
# 2) Generate the changelog for the mentioned version
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pawel Krupa (paulfantom)
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)
set -e

FAIL=0

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup || echo "")
if [ -n "${CWD}" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/$(basename "$0") from top level directory of netdata git repository"
    echo "Changelog generation process aborted"
    exit 1
fi

LAST_TAG=$(git describe --abbrev=0 --tags)
COMMITS_SINCE_RELEASE=$(git rev-list "${LAST_TAG}"..HEAD --count)
PREVIOUS_NIGHTLY_COUNT="$(rev <packaging/version | cut -d- -f 2 | rev)"

# If no commits since release, just stop
if [ "${COMMITS_SINCE_RELEASE}" == "${PREVIOUS_NIGHTLY_COUNT}" ]; then
	echo "No changes since last nighthly release"
	exit 0
fi

if [ ! "${TRAVIS_REPO_SLUG}" == "netdata/netdata" ]; then
	echo "Beta mode -- nothing to do for ${TRAVIS_REPO_SLUG} on the nightlies script, bye"
	exit 0
fi

echo "--- Running Changelog generation ---"
NIGHTLIES_CHANGELOG_FAILED=0
.travis/generate_changelog_for_nightlies.sh "${LAST_TAG}" "${COMMITS_SINCE_RELEASE}" || NIGHTLIES_CHANGELOG_FAILED=1

if [ ${NIGHTLIES_CHANGELOG_FAILED} -eq 1 ]; then
	echo "Changelog generation has failed, this is a soft error, process continues"
	post_message "TRAVIS_MESSAGE" "Changelog generation job for nightlies failed, possibly due to github issues" "${NOTIF_CHANNEL}"
fi

exit "${FAIL}"
