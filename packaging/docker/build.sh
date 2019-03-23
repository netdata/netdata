#!/bin/bash
# Cross-arch docker build helper script
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pawel Krupa (paulfantom)
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)

set -e

VERSION="$1"
REPOSITORY="${REPOSITORY:-netdata}"
declare -A ARCH_MAP
ARCH_MAP=(["i386"]="386" ["amd64"]="amd64" ["armhf"]="arm" ["aarch64"]="arm64")
DEVEL_ARCHS=(amd64)
ARCHS="${!ARCH_MAP[@]}"

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
echo "Architectures : ${ARCHS}"

docker run --rm --privileged multiarch/qemu-user-static:register --reset

# Build images using multi-arch Dockerfile.
for ARCH in "${ARCHS[@]}"; do
     TAG="${REPOSITORY}:${VERSION}-${ARCH}"
     echo "Building tag ${TAG}.."
     eval docker build \
          --build-arg ARCH="${ARCH}" \
          --tag "${TAG}" \
          --file packaging/docker/Dockerfile ./
     echo "..Done!"
done

echo "Docker build process completed!"
