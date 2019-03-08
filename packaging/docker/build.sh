#!/bin/bash

# Cross-arch docker build helper script
# Needs docker in version >18.02 due to usage of manifests
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pawel Krupa (paulfantom)

set -e

if [ ! -f .gitignore ]; then
	echo "Run as ./packaging/docker/$(basename "$0") from top level directory of git repository"
	exit 1
fi

echo "Docker build process starting.."

if [ "$1" == "" ]; then
    VERSION=$(git tag --points-at)
else
    VERSION="$1"
fi
if [ "${VERSION}" == "" ]; then
    VERSION="latest"
fi

declare -A ARCH_MAP
ARCH_MAP=( ["i386"]="386" ["amd64"]="amd64" ["armhf"]="arm" ["aarch64"]="arm64")
if [ -z ${DEVEL+x} ]; then
    declare -a ARCHITECTURES=(i386 armhf aarch64 amd64)
else
    declare -a ARCHITECTURES=(amd64)
    unset DOCKER_PASSWORD
    unset DOCKER_USERNAME
fi

REPOSITORY="${REPOSITORY:-netdata}"
echo "Building ${VERSION} of ${REPOSITORY} container"

docker run --rm --privileged multiarch/qemu-user-static:register --reset

# Build images using multi-arch Dockerfile.
for ARCH in "${ARCHITECTURES[@]}"; do
     eval docker build \
     		--build-arg ARCH="${ARCH}" \
     		--tag "${REPOSITORY}:${VERSION}-${ARCH}" \
     		--file packaging/docker/Dockerfile ./
done

echo "Docker build process completed!"
