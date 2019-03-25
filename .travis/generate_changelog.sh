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
CWD=$(git rev-parse --show-cdup)
if [ ! -z $CWD ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/$(basename "$0") from top level directory of netdata git repository"
    echo "Changelog generation process aborted"
    exit 1
fi

ORG=$(echo "$TRAVIS_REPO_SLUG" | cut -d '/' -f1)
PROJECT=$(echo "$TRAVIS_REPO_SLUG" | cut -d '/' -f 2)
GIT_MAIL=${GIT_MAIL:-"bot@netdata.cloud"}
GIT_USER=${GIT_USER:-"netdatabot"}
LAST_TAG=$(git describe --abbrev=0 --tags)
COMMITS_SINCE_RELEASE=$(git rev-list "$LAST_TAG"..HEAD --count)
PUSH_URL=$(git config --get remote.origin.url | sed -e 's/^https:\/\///')
FAIL=0
if [ -z ${GIT_TAG+x} ]; then
	OPTS=""
else
	OPTS="--future-release ${GIT_TAG}"
fi

# If no commits since release, just stop
if [ "$COMMITS_SINCE_RELEASE" == "$(rev <packaging/version | cut -d- -f 2 | rev)" ]; then
	echo "No changes since last release"
	exit 0
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

echo "Changelog created! Adding packaging/version and CHANGELOG.md to the repository"
echo "$LAST_TAG-$((COMMITS_SINCE_RELEASE + 1))-nightly" > packaging/version
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
