#!/bin/bash
# Docker build wrapper, for testing manually the docker build process
# TODO: This script should consume build.sh after setting up required parameters
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Chris Akritidis (chris@netdata.cloud)
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)

printhelp() {
	echo "Usage: packaging/docker/build-test.sh -r <REPOSITORY> -v <VERSION> -u <DOCKER_USERNAME> -p <DOCKER_PASSWORD> [-s]
	-s skip build, just push the image
Builds an amd64 image and pushes it to the docker hub repository REPOSITORY"
}

set -e

if [ ! -f .gitignore ]; then
	echo "Run as ./packaging/docker/$(basename "$0") from top level directory of git repository"
	exit 1
fi

DOBUILD=1
while getopts :r:v:u:p:s option
do
	case "$option" in
	r)
		REPOSITORY=$OPTARG
	 	;;
	v)
		VERSION=$OPTARG
		;;
	u)
		DOCKER_USERNAME=$OPTARG
		;;
	p) 
		DOCKER_PASSWORD=$OPTARG
		;;
	s)
		DOBUILD=0
		;;
	*)
		printhelp
		exit 1
		;;
	esac
done

if [ -n "${REPOSITORY}" ] && [ -n "${VERSION}" ] && [ -n "${DOCKER_USERNAME}" ] && [ -n "${DOCKER_PASSWORD}" ] ; then
	if [ $DOBUILD -eq 1 ] ; then
		echo "Building ${VERSION} of ${REPOSITORY} container"
		docker run --rm --privileged multiarch/qemu-user-static:register --reset

		# Build images using multi-arch Dockerfile.
		eval docker build --build-arg ARCH="amd64" --tag "${REPOSITORY}:${VERSION}" --file packaging/docker/Dockerfile ./

		# Create temporary docker CLI config with experimental features enabled (manifests v2 need it)
		mkdir -p /tmp/docker
		#echo '{"experimental":"enabled"}' > /tmp/docker/config.json
	fi

	# Login to docker hub to allow futher operations
	echo "Logging into docker"
	echo "$DOCKER_PASSWORD" | docker --config /tmp/docker login -u "$DOCKER_USERNAME" --password-stdin

	echo "Pushing ${REPOSITORY}:${VERSION}"
	docker --config /tmp/docker push "${REPOSITORY}:${VERSION}"
else
	echo "Missing parameter. REPOSITORY=${REPOSITORY} VERSION=${VERSION} DOCKER_USERNAME=${DOCKER_USERNAME} DOCKER_PASSWORD=${DOCKER_PASSWORD}"
	printhelp
	exit 1
fi
