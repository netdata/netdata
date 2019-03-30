#!/bin/bash
#
# Changelog generation scriptlet.
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
ORG=$(echo "$TRAVIS_REPO_SLUG" | cut -d '/' -f1)
PROJECT=$(echo "$TRAVIS_REPO_SLUG" | cut -d '/' -f 2)
GIT_MAIL=${GIT_MAIL:-"bot@netdata.cloud"}
GIT_USER=${GIT_USER:-"netdatabot"}
PUSH_URL=$(git config --get remote.origin.url | sed -e 's/^https:\/\///')
FAIL=0
if [ -z ${GIT_TAG+x} ]; then
	OPTS=""
else
	OPTS="--future-release ${GIT_TAG}"
fi

echo "We got $COMMITS_SINCE_RELEASE changes since $LAST_TAG, re-generating changelog"
git config user.email "${GIT_MAIL}"
git config user.name "${GIT_USER}"
git checkout master
git pull

echo "Running project markmandel for github changelog generation"
#docker run -it --rm -v "$(pwd)":/usr/local/src/your-app ferrarimarco/github-changelog-generator:1.14.3 \
docker run -it -v "$(pwd)":/project markmandel/github-changelog-generator:latest \
	--user "${ORG}" \
	--project "${PROJECT}" \
	--token "${GITHUB_TOKEN}" \
	--since-tag "v1.10.0" \
	--unreleased-label "**Next release**" \
	--exclude-labels "stale,duplicate,question,invalid,wontfix,discussion,no changelog" \
	--no-compare-link ${OPTS}

echo "Changelog created! Adding packaging/version(${NEW_VERSION}) and CHANGELOG.md to the repository"
echo "${NEW_VERSION}" > packaging/version
git add packaging/version && echo "1) Added packaging/version to repository" || FAIL=1
git add CHANGELOG.md && echo "2) Added changelog file to repository" || FAIL=1
git commit -m '[ci skip] create nightly packages and update changelog' && echo "3) Committed changes to repository" || FAIL=1
git push "https://${GITHUB_TOKEN}:@${PUSH_URL}" && echo "4) Pushed changes to remote ${PUSH_URL}" || FAIL=1

# In case of a failure, wrap it up and bail out cleanly
if [ $FAIL -eq 1 ]; then
	git clean -xfd
	echo "Changelog generation failed during github UPDATE!"
	exit 1
fi

echo "Changelog generation completed successfully!"
