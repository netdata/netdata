#!/usr/bin/env bash
#
# Changelog generation scriptlet, for nightlies
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pawel Krupa (paulfantom)
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)
set -e

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup || echo "")
if [ -n "$CWD" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/$(basename "$0") from top level directory of netdata git repository"
    echo "Changelog generation process aborted"
    exit 1
fi

LAST_TAG="$1"
COMMITS_SINCE_RELEASE="$2"
NEW_VERSION="${LAST_TAG}-$((COMMITS_SINCE_RELEASE + 1))-nightly"
GIT_MAIL=${GIT_MAIL:-"bot@netdata.cloud"}
GIT_USER=${GIT_USER:-"netdatabot"}
PUSH_URL=$(git config --get remote.origin.url | sed -e 's/^https:\/\///')
FAIL=0

if [ ! "${TRAVIS_REPO_SLUG}" == "netdata/netdata" ]; then
	echo "Beta mode on ${TRAVIS_REPO_SLUG}, nothing else to do here"
	exit 0
fi

echo "Running changelog creation mechanism"
.travis/create_changelog.sh

echo "Changelog created! Adding packaging/version(${NEW_VERSION}) and CHANGELOG.md to the repository"
echo "${NEW_VERSION}" > packaging/version
git add packaging/version && echo "1) Added packaging/version to repository" || FAIL=1
git add CHANGELOG.md && echo "2) Added changelog file to repository" || FAIL=1
git commit -m '[ci skip] create nightly packages and update changelog' --author "${GIT_USER} <${GIT_MAIL}>" && echo "3) Committed changes to repository" || FAIL=1
git push "https://${GITHUB_TOKEN}:@${PUSH_URL}" && echo "4) Pushed changes to remote ${PUSH_URL}" || FAIL=1

# In case of a failure, wrap it up and bail out cleanly
if [ $FAIL -eq 1 ]; then
	git clean -xfd
	echo "Changelog generation failed during github UPDATE!"
	exit 1
fi

echo "Changelog generation completed successfully!"
