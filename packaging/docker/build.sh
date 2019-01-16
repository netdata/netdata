#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Author  : Pawel Krupa (paulfantom)
# Cross-arch docker build helper script
# Needs docker in version >18.02 due to usage of manifests

set -e

if [ ! -f .gitignore ]; then
	echo "Run as ./packaging/docker/$(basename "$0") from top level directory of git repository"
	exit 1
fi

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
    BG="&"
else
    declare -a ARCHITECTURES=(amd64)
    unset DOCKER_PASSWORD
    unset DOCKER_USERNAME
    BG=""
fi

REPOSITORY="${REPOSITORY:-netdata}"
echo "Building ${VERSION} of ${REPOSITORY} container"

docker run --rm --privileged multiarch/qemu-user-static:register --reset

# Build images using multi-arch Dockerfile.
for ARCH in "${ARCHITECTURES[@]}"; do
     eval docker build \
     		--build-arg ARCH="${ARCH}-v3.8" \
     		--build-arg OUTPUT=/dev/null \
     		--tag "${REPOSITORY}:${VERSION}-${ARCH}" \
     		--file packaging/docker/Dockerfile ./ ${BG}
done
wait

# There is no reason to continue if we cannot log in to docker hub
if [ -z ${DOCKER_USERNAME+x} ] || [ -z ${DOCKER_PASSWORD+x} ]; then
    echo "No docker hub username or password specified. Exiting without pushing images to registry"
    exit 0
fi

# Create temporary docker CLI config with experimental features enabled (manifests v2 need it)
mkdir -p /tmp/docker
echo '{"experimental":"enabled"}' > /tmp/docker/config.json

# Login to docker hub to allow futher operations
echo "$DOCKER_PASSWORD" | docker --config /tmp/docker login -u "$DOCKER_USERNAME" --password-stdin

# Push images to registry
for ARCH in amd64 i386 armhf aarch64; do
    docker --config /tmp/docker push "${REPOSITORY}:${VERSION}-${ARCH}" &
done
wait

# Recreate docker manifest
docker --config /tmp/docker manifest create --amend \
                       "${REPOSITORY}:${VERSION}" \
                       "${REPOSITORY}:${VERSION}-i386" \
                       "${REPOSITORY}:${VERSION}-armhf" \
                       "${REPOSITORY}:${VERSION}-aarch64" \
                       "${REPOSITORY}:${VERSION}-amd64"

# Annotate manifest with CPU architecture information
for ARCH in i386 armhf aarch64 amd64; do
     docker --config /tmp/docker manifest annotate "${REPOSITORY}:${VERSION}" "${REPOSITORY}:${VERSION}-${ARCH}" --os linux --arch "${ARCH_MAP[$ARCH]}"
done

# Push manifest to docker hub
docker --config /tmp/docker manifest push -p "${REPOSITORY}:${VERSION}"

# Show current manifest (debugging purpose only)
docker --config /tmp/docker manifest inspect "${REPOSITORY}:${VERSION}"
