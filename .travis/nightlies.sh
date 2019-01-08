#!/bin/bash

set -e

if [ ! -f .gitignore ]; then
	echo "Run as ./travis/$(basename "$0") from top level directory of git repository"
	exit 1
fi

export GIT_MAIL="pawel+bot@netdata.cloud"
export GIT_USER="netdatabot"
echo "--- Initialize git configuration ---"
git config user.email "${GIT_MAIL}"
git config user.name "${GIT_USER}"

echo "--- UPDATE VERSION FILE ---"
LAST_TAG=$(git describe --abbrev=0 --tags)
NO_COMMITS=$(git rev-list "$LAST_TAG"..HEAD --count)
echo "$LAST_TAG-$((NO_COMMITS + 1))-nightly" >packaging/version
git add packaging/version

echo "---- GENERATE CHANGELOG -----"
.travis/generate_changelog.sh
git add CHANGELOG.md

echo "---- UPLOAD FILE CHANGES ----"
git commit -m '[ci skip] create nightly packages and update changelog'
git push "https://${GITHUB_TOKEN}:@$(git config --get remote.origin.url | sed -e 's/^https:\/\///')"

echo "---- BUILD & PUBLISH DOCKER IMAGES ----"
export REPOSITORY="netdata/netdata"
packaging/docker/build.sh

echo "---- BUILD ARTIFACTS ----"
.travis/create_artifacts.sh
