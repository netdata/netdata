#!/bin/bash
#
# This is the nightlies orchastration script
# It runs the following activities in order:
# 1) Generate changelog
# 2) Build docker images
# 3) Publish docker images
# 4) Generate the rest of the artifacts (Source code .tar.gz file and makeself binary generation)
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
if [ -n "$CWD" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/$(basename "$0") from top level directory of netdata git repository"
    echo "Changelog generation process aborted"
    exit 1
fi

LAST_TAG=$(git describe --abbrev=0 --tags)
COMMITS_SINCE_RELEASE=$(git rev-list "$LAST_TAG"..HEAD --count)
PREVIOUS_NIGHTLY_COUNT="$(rev <packaging/version | cut -d- -f 2 | rev)"

# If no commits since release, just stop
if [ "$COMMITS_SINCE_RELEASE" == "${PREVIOUS_NIGHTLY_COUNT}" ]; then
	echo "No changes since last nighthly release"
	exit 0
fi

echo "--- Running Changelog generation ---"
.travis/generate_changelog.sh "$LAST_TAG" "$COMMITS_SINCE_RELEASE" || echo "Changelog generation has failed, this is a soft error, process continues"

echo "--- Build && publish docker images ---"
# Do not fail artifacts creation if docker fails. We will be restructuring this on a follow up PR
packaging/docker/build.sh && packaging/docker/publish.sh || echo "Failed to build and publish docker images"

echo "--- Build artifacts ---"
.travis/create_artifacts.sh

exit "${FAIL}"
