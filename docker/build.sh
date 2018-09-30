#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Author  : Pawel Krupa (paulfantom)
# Cross-arch docker build helper script
# Needs docker in version >18.02 due to usage of manifests

set -e

REPOSITORY="${REPOSITORY:-netdata}"

VERSION=$(git tag --points-at)
if [ "${VERSION}" == "" ]; then
    VERSION="latest"
fi

declare -A ARCH_MAP
ARCH_MAP=( ["i386"]="386" ["amd64"]="amd64" ["armhf"]="arm" ["aarch64"]="arm64")

docker run --rm --privileged multiarch/qemu-user-static:register --reset

if [ -f Dockerfile ]; then
    cd ../ || exit 1
fi

# Build images using multi-arch Dockerfile.
for ARCH in i386 armhf aarch64 amd64; do
     docker build --build-arg ARCH="${ARCH}-v3.8" --tag "${REPOSITORY}:${VERSION}-${ARCH}" --file docker/Dockerfile ./ &
done
wait

# Create temporary docker CLI config with experimental features enabled (manifests v2 need it)
mkdir -p /tmp/docker
echo '{"experimental":"enabled"}' > /tmp/docker/config.json

# Login to docker hub to allow for futher operations
if [ -z ${DOCKER_USERNAME+x} ] || [ -z ${DOCKER_PASSWORD+x} ]; then
    echo "No docker hub username or password specified. Exiting without pushing images to registry"
    exit 1
fi
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

# Push netdata images to firehol organization
# TODO: Remove it after we decide to deprecate firehol/netdata docker repo
if [ "$REPOSITORY" != "netdata" ]; then
    echo "$OLD_DOCKER_PASSWORD" | docker login -u "$OLD_DOCKER_USERNAME" --password-stdin   
    for ARCH in amd64 i386 armhf aarch64; do
        docker tag "${REPOSITORY}:${VERSION}-${ARCH}" "firehol/netdata:${ARCH}"
        docker push "firehol/netdata:${ARCH}"
    done
    docker tag "${REPOSITORY}:${VERSION}" "firehol/netdata:${VERSION}"
    docker push "firehol/netdata:latest"
fi
