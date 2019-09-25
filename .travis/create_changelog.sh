#!/usr/bin/env bash
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)
set -e

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup || echo "")
if [ -n "$CWD" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/$(basename "$0") from top level directory of netdata git repository"
    echo "Changelog creation aborted"
    exit 1
fi

ORGANIZATION=$(echo "$TRAVIS_REPO_SLUG" | awk -F '/' '{print $1}')
PROJECT=$(echo "$TRAVIS_REPO_SLUG" | awk -F '/' '{print $2}')
GIT_MAIL=${GIT_MAIL:-"bot@netdata.cloud"}
GIT_USER=${GIT_USER:-"netdatabot"}

if [ -z ${GIT_TAG+x} ]; then
	OPTS=""
else
	OPTS="--future-release ${GIT_TAG}"
fi

if [ ! "${TRAVIS_REPO_SLUG}" == "netdata/netdata" ]; then
	echo "Beta mode on ${TRAVIS_REPO_SLUG}, nothing else to do here"
	exit 0
fi

echo "--- Creating changelog ---"
git checkout master
git pull

docker run -it -v "$(pwd)":/project markmandel/github-changelog-generator:latest \
	--user "${ORGANIZATION}" \
	--project "${PROJECT}" \
	--token "${GITHUB_TOKEN}" \
	--since-tag "v1.10.0" \
	--unreleased-label "**Next release**" \
	--no-issues \
	--exclude-labels "stale,duplicate,question,invalid,wontfix,discussion,no changelog" \
	--max-issues 500 \
	--bug-labels IGNOREBUGS ${OPTS}
