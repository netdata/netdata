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
declare -A ARCH_MAP
ARCH_MAP=(["i386"]="386" ["amd64"]="amd64" ["armhf"]="arm" ["aarch64"]="arm64")
DEVEL_ARCHS=(amd64)
[ "${ARCHS}" ] || ARCHS="${!ARCH_MAP[@]}" # Use default ARCHS unless ARCHS are externally provided

if [ -z ${REPOSITORY} ]; then
	REPOSITORY="${TRAVIS_REPO_SLUG}"
	if [ -z ${REPOSITORY} ]; then
		echo "REPOSITORY not set, build cannot proceed"
		exit 1
	else
		echo "REPOSITORY was not detected, attempted to use TRAVIS_REPO_SLUG setting: ${TRAVIS_REPO_SLUG}"
	fi
fi

# When development mode is set, build on DEVEL_ARCHS
if [ ! -z ${DEVEL+x} ]; then
    declare -a ARCHS=(${DEVEL_ARCHS[@]})
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

echo "Docker image build in progress.."
echo "Version       : ${VERSION}"
echo "Repository    : ${REPOSITORY}"
echo "Architectures : ${ARCHS[*]}"

docker run --rm --privileged multiarch/qemu-user-static:register --reset

# Build images using multi-arch Dockerfile.
for ARCH in ${ARCHS[@]}; do
     TAG="${REPOSITORY}:${VERSION}-${ARCH}"
     echo "Building tag ${TAG}.."
     eval docker build --no-cache \
          --build-arg ARCH="${ARCH}" \
          --tag "${TAG}" \
          --file packaging/docker/Dockerfile ./
     echo "..Done!"
done

echo "Docker build process completed!"
