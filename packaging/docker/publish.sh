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
MANIFEST_LIST="${REPOSITORY}:${VERSION}"
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
echo "Manifest list : ${MANIFEST_LIST}"

# Create temporary docker CLI config with experimental features enabled (manifests v2 need it)
mkdir -p "${WORKDIR}"
echo '{"experimental":"enabled"}' > "${WORKDIR}"/config.json

# Login to docker hub to allow futher operations
echo "$DOCKER_PASSWORD" | docker --config "${WORKDIR}" login -u "$DOCKER_USERNAME" --password-stdin

# Push images to registry
for ARCH in ${ARCHS[@]}; do
    TAG="${MANIFEST_LIST}-${ARCH}"
    echo "Publushing image ${TAG}.."
    docker --config "${WORKDIR}" push "${TAG}" &
done

echo "Waiting for images publishing to complete"
wait

# Recreate docker manifest list
docker --config "${WORKDIR}" manifest create --amend "${MANIFEST_LIST}" \
                                                     "${MANIFEST_LIST}-i386" \
                                                     "${MANIFEST_LIST}-armhf" \
                                                     "${MANIFEST_LIST}-aarch64" \
                                                     "${MANIFEST_LIST}-amd64"

# Annotate manifest with CPU architecture information
for ARCH in ${ARCHS[@]}; do
     TAG="${MANIFEST_LIST}-${ARCH}"
     docker --config "${WORKDIR}" manifest annotate "${MANIFEST_LIST}" "${TAG}" --os linux --arch "${ARCH_MAP[$ARCH]}"
done

# Push manifest to docker hub
docker --config "${WORKDIR}" manifest push -p "${MANIFEST_LIST}"

# Show current manifest (debugging purpose only)
docker --config "${WORKDIR}" manifest inspect "${MANIFEST_LIST}"

echo "Docker publishing process completed!"
