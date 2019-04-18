#!/bin/bash
#
# Draft release generator.
# This utility is responsible for submitting a draft release to github repo
# It is agnostic of other processes, when executed it will draft a release,
# based on the most recent reachable tag.
#
# Requirements:
#   - GITHUB_TOKEN variable set with GitHub token. Access level: repo.public_repo
#   - artifacts directory in place
#     - The directory is created by create_artifacts.sh mechanism
#     - The artifacts need to be created with the same tag, obviously
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author: Pavlos Emm. Katsoulakis <paul@netdata.cloud>

set -e

if [ ! -f .gitignore ]; then
	echo "Run as ./travis/$(basename "$0") from top level directory of git repository"
	exit 1
fi

echo "Pulling latest code"
git pull

if [[ $(git describe) =~ -rc* ]]; then
	echo "This is a release candidate tag, we do not generate a release draft"
	exit 0
fi

# Load the tag, if any
GIT_TAG=$(git describe)

if [ ! "${TRAVIS_REPO_SLUG}" == "netdata/netdata" ]; then
	echo "Beta mode on ${TRAVIS_REPO_SLUG}, i was about to run for release (${GIT_TAG}), but i am emulating, so bye"
	exit 0
fi;

echo "---- CREATING RELEASE DRAFT WITH ASSETS -----"
# Download hub
HUB_VERSION=${HUB_VERSION:-"2.5.1"}
wget "https://github.com/github/hub/releases/download/v${HUB_VERSION}/hub-linux-amd64-${HUB_VERSION}.tgz" -O "/tmp/hub-linux-amd64-${HUB_VERSION}.tgz"
tar -C /tmp -xvf "/tmp/hub-linux-amd64-${HUB_VERSION}.tgz"
export PATH=$PATH:"/tmp/hub-linux-amd64-${HUB_VERSION}/bin"

# Create a release draft
if [ -z ${GIT_TAG+x} ]; then
	echo "Variable GIT_TAG is not set. Something went terribly wrong! Exiting."
	exit 1
fi
if [ "${GIT_TAG}" != "$(git tag --points-at)" ]; then
	echo "ERROR! Current commit is not tagged. Stopping release creation."
	exit 1
fi
until hub release create --draft \
		-a "artifacts/netdata-${GIT_TAG}.tar.gz" \
		-a "artifacts/netdata-${GIT_TAG}.gz.run" \
		-a "artifacts/sha256sums.txt" \
		-m "${GIT_TAG}" "${GIT_TAG}"; do
	sleep 5
done
