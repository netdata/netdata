#!/usr/bin/env bash
#
# This is a credential checker script, to help get early input on docker credentials status
# If these are wrong, then build/publish has no point running
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)

set -e

if [ "${BASH_VERSINFO[0]}" -lt "4" ]; then
	echo "This mechanism currently can only run on BASH version 4 and above"
	exit 1
fi

DOCKER_CMD="docker "

# There is no reason to continue if we cannot log in to docker hub
if [ -z ${DOCKER_USERNAME+x} ] || [ -z ${DOCKER_PWD+x} ]; then
    echo "No docker hub username or password found, aborting without publishing"
    exit 1
fi

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup)
if [ -n "$CWD" ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as ./packaging/docker/$(basename "$0") from top level directory of netdata git repository"
    echo "Docker build process aborted"
    exit 1
fi

# Login to docker hub to allow futher operations
echo "Attempting to login to docker"
echo "$DOCKER_PWD" | $DOCKER_CMD login -u "$DOCKER_USERNAME" --password-stdin

echo "Docker login successful!"
$DOCKER_CMD logout

echo "Docker login validation completed"
