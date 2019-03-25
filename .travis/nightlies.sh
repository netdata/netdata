#!/bin/bash
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

if [ ! -f .gitignore ]; then
	echo "Run as ./travis/$(basename "$0") from top level directory of git repository"
	exit 1
fi

export GIT_MAIL="bot@netdata.cloud"
export GIT_USER="netdatabot"
echo "--- Initialize git configuration ---"
git config user.email "${GIT_MAIL}"
git config user.name "${GIT_USER}"

echo "--- UPDATE VERSION FILE ---"
LAST_TAG=$(git describe --abbrev=0 --tags)
NO_COMMITS=$(git rev-list "$LAST_TAG"..HEAD --count)
if [ "$NO_COMMITS" == "$(rev <packaging/version | cut -d- -f 2 | rev)" ]; then
	echo "Nothing changed since last nightly build"
	exit 0
fi
echo "$LAST_TAG-$((NO_COMMITS + 1))-nightly" >packaging/version
git add packaging/version || exit 1

echo "--- GENERATE CHANGELOG ---"
if .travis/generate_changelog.sh; then
	git add CHANGELOG.md

	echo "--- UPLOAD FILE CHANGES ---"
	git commit -m '[ci skip] create nightly packages and update changelog'
	git push "https://${GITHUB_TOKEN}:@$(git config --get remote.origin.url | sed -e 's/^https:\/\///')"
else
	git clean -xfd
	echo "Changelog generation has failed, will proceed anyway"
fi

echo "--- BUILD & PUBLISH DOCKER IMAGES ---"
packaging/docker/build.sh || FAIL=1
packaging/docker/publish.sh || FAIL=1

echo "--- BUILD ARTIFACTS ---"
.travis/create_artifacts.sh || FAIL=1

exit "${FAIL}"
