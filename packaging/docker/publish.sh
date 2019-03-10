#!/bin/bash
# Cross-arch docker publish helper script
# Needs docker in version >18.02 due to usage of manifests
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pawel Krupa (paulfantom)
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)

set -e

WORKDIR="/tmp/docker"
VERSION="$1"
REPOSITORY="${REPOSITORY:-netdata}"
declare -A ARCH_MAP
ARCH_MAP=( ["i386"]="386" ["amd64"]="amd64" ["armhf"]="arm" ["aarch64"]="arm64")
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

# There is no reason to continue if we cannot log in to docker hub
if [ -z ${DOCKER_USERNAME+x} ] || [ -z ${DOCKER_PASSWORD+x} ]; then
    echo "No docker hub username or password found, aborting without publishing"
    exit 1
fi

# TODO: Need a more stable way to find where to run from.
if [ ! -f .gitignore ]; then
    echo "Run as ./packaging/docker/$(basename "$0") from top level directory of git repository"
    echo "Docker build process aborted"
    exit 1
fi

echo "Docker image publishing in progress.."
echo "Version       : ${VERSION}"
echo "Repository    : ${REPOSITORY}"
echo "Architectures : ${ARCHS}"

# Create temporary docker CLI config with experimental features enabled (manifests v2 need it)
mkdir -p "${WORKDIR}"
echo '{"experimental":"enabled"}' > "${WORKDIR}"/config.json

# Login to docker hub to allow futher operations
echo "$DOCKER_PASSWORD" | docker --config "${WORKDIR}" login -u "$DOCKER_USERNAME" --password-stdin

# Push images to registry
for ARCH in ${ARCHS[@]}; do
    docker --config "${WORKDIR}" push "${REPOSITORY}:${VERSION}-${ARCH}" &
done
wait

# Recreate docker manifest
docker --config "${WORKDIR}" manifest create --amend \
                       "${REPOSITORY}:${VERSION}" \
                       "${REPOSITORY}:${VERSION}-i386" \
                       "${REPOSITORY}:${VERSION}-armhf" \
                       "${REPOSITORY}:${VERSION}-aarch64" \
                       "${REPOSITORY}:${VERSION}-amd64"

# Annotate manifest with CPU architecture information
for ARCH in ${ARCHS[@]}; do
     docker --config "${WORKDIR}" manifest annotate "${REPOSITORY}:${VERSION}" "${REPOSITORY}:${VERSION}-${ARCH}" --os linux --arch "${ARCH_MAP[$ARCH]}"
done

# Push manifest to docker hub
docker --config "${WORKDIR}" manifest push -p "${REPOSITORY}:${VERSION}"

# Show current manifest (debugging purpose only)
docker --config "${WORKDIR}" manifest inspect "${REPOSITORY}:${VERSION}"

echo "Docker publishing process completed!"
