#!/usr/bin/env bash
#
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pawel Krupa (paulfantom)
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)

set -e

if [ "${BASH_VERSINFO[0]}" -lt "4" ]; then
	echo "This mechanism currently can only run on BASH version 4 and above"
	exit 1
fi

VERSION="$1"

if [ -z "${ARCH}" ]; then
  echo "ARCH not set, build cannot proceed"
  exit 1
fi

if [ "${RELEASE_CHANNEL}" != "nightly" ] && [ "${RELEASE_CHANNEL}" != "stable" ]; then
  echo "RELEASE_CHANNEL must be set to either 'nightly' or 'stable' - build cannot proceed"
  exit 1
fi

if [ -z ${REPOSITORY} ]; then
	REPOSITORY="${TRAVIS_REPO_SLUG}"
	if [ -z ${REPOSITORY} ]; then
		echo "REPOSITORY not set, build cannot proceed"
		exit 1
	else
		echo "REPOSITORY was not detected, attempted to use TRAVIS_REPO_SLUG setting: ${TRAVIS_REPO_SLUG}"
	fi
fi

# Ensure there is a version, the most appropriate one
if [ "${VERSION}" == "" ]; then
    VERSION=$(git tag --points-at)
    if [ "${VERSION}" == "" ]; then
        VERSION="latest"
    fi
fi

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup)
if [ ! -z $CWD ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as ./packaging/docker/$(basename "$0") from top level directory of netdata git repository"
    echo "Docker build process aborted"
    exit 1
fi

case "${ARCH}" in
  amd64) DOCKER_PLATFORM="linux/amd64" ;;
  i386) DOCKER_PLATFORM="linux/i386" ;;
  armhf) DOCKER_PLATFORM="linux/arm/v7" ;;
  aarch64) DOCKER_PLATFORM="linux/arm64" ;;
esac

echo "Docker image build in progress.."
echo "Version     : ${VERSION}"
echo "Repository  : ${REPOSITORY}"
echo "Architecture: ${ARCH}"

docker run --rm --privileged multiarch/qemu-user-static --reset -p yes

# Build images using multi-arch Dockerfile.
TAG="${REPOSITORY,,}:${VERSION}-${ARCH}"
echo "Building tag ${TAG}.."
docker build --no-cache                                  \
	--build-arg ARCH="${ARCH}"                       \
	--build-arg RELEASE_CHANNEL="${RELEASE_CHANNEL}" \
	--platform "${DOCKER_PLATFORM}"                  \
	--tag "${TAG}"                                   \
	--file packaging/docker/Dockerfile .
echo "..Done!"

echo "Docker build process completed!"
