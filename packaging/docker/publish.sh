#!/usr/bin/env bash
#
# Cross-arch docker publish helper script
# Needs docker in version >18.02 due to usage of manifests
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)

set -e

if [ "${BASH_VERSINFO[0]}" -lt "4" ]; then
	echo "This mechanism currently can only run on BASH version 4 and above"
	exit 1
fi

WORKDIR="$(mktemp -d)" # Temporary folder, removed after script is done
VERSION="$1"
declare -A ARCH_MAP
ARCH_MAP=(["i386"]="386" ["amd64"]="amd64" ["armhf"]="arm" ["aarch64"]="arm64")
DEVEL_ARCHS=(amd64)
ARCHS="${!ARCH_MAP[@]}"
DOCKER_CMD="docker --config ${WORKDIR}"
GIT_MAIL=${GIT_MAIL:-"bot@netdata.cloud"}
GIT_USER=${GIT_USER:-"netdatabot"}

if [ -z ${REPOSITORY} ]; then
	REPOSITORY="${TRAVIS_REPO_SLUG}"
	if [ -z ${REPOSITORY} ]; then
		echo "REPOSITORY not set, publish cannot proceed"
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
MANIFEST_LIST="${REPOSITORY}:${VERSION}"

# There is no reason to continue if we cannot log in to docker hub
if [ -z ${DOCKER_USERNAME+x} ] || [ -z ${DOCKER_PWD+x} ]; then
    echo "No docker hub username or password found, aborting without publishing"
    exit 1
fi

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup)
if [ ! -z $CWD ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as ./packaging/docker/$(basename "$0") from top level directory of netdata git repository"
    echo "Docker build process aborted"
    exit 1
fi

echo "Docker image publishing in progress.."
echo "Version       : ${VERSION}"
echo "Repository    : ${REPOSITORY}"
echo "Architectures : ${ARCHS[*]}"
echo "Manifest list : ${MANIFEST_LIST}"

# Create temporary docker CLI config with experimental features enabled (manifests v2 need it)
echo '{"experimental":"enabled"}' > "${WORKDIR}"/config.json

# Login to docker hub to allow futher operations
echo "$DOCKER_PWD" | $DOCKER_CMD login -u "$DOCKER_USERNAME" --password-stdin

# Push images to registry
for ARCH in ${ARCHS[@]}; do
    TAG="${MANIFEST_LIST}-${ARCH}"
    echo "Publishing image ${TAG}.."
    $DOCKER_CMD push "${TAG}" &
    echo "Image ${TAG} published succesfully!"
done

echo "Waiting for images publishing to complete"
wait

# Recreate docker manifest list
echo "Creating manifest list.."
$DOCKER_CMD manifest create --amend "${MANIFEST_LIST}" \
                                    "${MANIFEST_LIST}-i386" \
                                    "${MANIFEST_LIST}-armhf" \
                                    "${MANIFEST_LIST}-aarch64" \
                                    "${MANIFEST_LIST}-amd64"

# Annotate manifest with CPU architecture information

echo "Executing manifest annotate.."
for ARCH in ${ARCHS[@]}; do
     TAG="${MANIFEST_LIST}-${ARCH}"
     echo "Annotating manifest for $ARCH, with TAG: ${TAG} (Manifest list: ${MANIFEST_LIST})"
     $DOCKER_CMD manifest annotate "${MANIFEST_LIST}" "${TAG}" --os linux --arch "${ARCH_MAP[$ARCH]}"
done

# Push manifest to docker hub
echo "Pushing manifest list to docker.."
$DOCKER_CMD manifest push -p "${MANIFEST_LIST}"

# Show current manifest (debugging purpose only)
echo "Evaluating manifest list entry"
$DOCKER_CMD manifest inspect "${MANIFEST_LIST}"

# Cleanup
rm -r "${WORKDIR}"

echo "Docker publishing process completed!"
