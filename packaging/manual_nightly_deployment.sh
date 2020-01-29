#!/usr/bin/env bash
#
# This tool allows netdata team to manually deploy nightlies
# It emulates the nightly operations required for a new version to be published for our users
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis <paul@netdata.cloud>
#
set -e

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup || echo "")
if [ -n "${CWD}" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/$(basename "$0") from top level directory of netdata git repository"
    echo "Changelog generation process aborted"
    exit 1
fi

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
	echo "Run as ./$(basename "$0") [docker|gcs]|all]  from the top level directory of netdata GIT repository"
	exit 1
fi

GSUTIL_BINARY=$(command -v gsutil 2> /dev/null)
if [ -z "${GSUTIL_BINARY}" ]; then
	echo "No gsutil utility available, you need gsutil deployed to manually deploy to GCS"
	exit 1
fi;

# Function declarations
publish_docker() {

	# Ensure REPOSITORY present
	if [ -z "${REPOSITORY}" ]; then
		echo "Please provide the repository to deploy the containers:"
		read -r REPOSITORY
		export REPOSITORY
	else
		echo "Docker publishing to ${REPOSITORY}"
	fi

	# Ensure DOCKER_USERNAME present
	if [ -z "${DOCKER_USERNAME}" ]; then
		echo "For repository ${REPOSITORY}, Please provide the docker USERNAME to use:"
		read -r DOCKER_USERNAME
		export DOCKER_USERNAME
	else
		echo "Using docker username ${DOCKER_USERNAME}"
	fi

	# Ensure DOCKER_PASS present
	if [ -z "${DOCKER_PASS}" ]; then
		echo "Username ${DOCKER_USERNAME} received, now give me the password:"
		read -r -s DOCKER_PASS
		export DOCKER_PASS
	else
		echo "Docker password has already been set to env, using that"
	fi

	echo "Building Docker images.."
	RELEASE_CHANNEL=nightly packaging/docker/build.sh

	echo "Publishing Docker images.."
	packaging/docker/publish.sh
}

publish_nightly_binaries() {
	echo "Publishing nightly binaries to GCS"

	echo "Please select the bucket to sync, from the ones available to you:"
	bucket_list=$(${GSUTIL_BINARY} list | tr '\n' ' ')
	declare -A buckets
	idx=0
	for abucket in ${bucket_list}; do
		echo "${idx}. ${abucket}"
		buckets["${idx}"]=${abucket}
		((idx=idx+1))
	done
	read -p"Selection>" -r -n 1 selected_bucket

	echo "Ok!"
	echo "Syncing artifacts directory contents with GCS bucket: ${buckets[${selected_bucket}]}"
	if [ -d artifacts ]; then
		${GSUTIL_BINARY} -m rsync -r artifacts "${buckets["${selected_bucket}"]}"
		echo "GCS Sync complete!"
	else
		echo "Directory artifacts does not exist, nothing to do on GCS"
	fi
}

prepare_and_publish_gcs() {
	# Prepare the artifacts directory
	echo "Preparing artifacts directory contents"
	.travis/create_artifacts.sh

	# Publish it to GCS
	publish_nightly_binaries

	# Clean up
	echo "Cleaning up repository"
	make clean || echo "Nothing to clean"
	make distclean || echo "Nothing to distclean"
	rm -rf artifacts
}

# Mandatory variable declarations
export TRAVIS_REPO_SLUG="netdata/netdata"

echo "Manual nightly deployment procedure started"
case "$1" in
	"docker")
		publish_docker
		;;
	"gcs")
		prepare_and_publish_gcs
		;;
	"all")
		publish_docker
		prepare_and_publish_gcs
		;;
	*)
		echo "ERROR: Invalid request parameter $1. Valid values are: docker, gcs, all"
		;;
esac
echo "Manual nightly deployment completed!"
