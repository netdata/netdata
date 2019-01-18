#!/bin/bash

BAD_THING_HAPPENED=0

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
	BAD_THING_HAPPENED=1
fi

echo "--- BUILD & PUBLISH DOCKER IMAGES ---"
export REPOSITORY="netdata/netdata"
packaging/docker/build.sh || BAD_THING_HAPPENED=1

echo "--- BUILD ARTIFACTS ---"
.travis/create_artifacts.sh || BAD_THING_HAPPENED=1

exit "${BAD_THING_HAPPENED}"
