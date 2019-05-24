#!/usr/bin/env bash
#
# This script is evaluating netdata installation with the source from make dist
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis <paul@netdata.cloud)
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

echo "Initiating dist archive contents validation"
DIST_FILE_FROM_GIT="netdata-$(git describe).tar.gz"
DIST_FILE_FROM_FILE="netdata-$(tr -d '\n' < packaging/version).tar.bgz"
if [ -f ${DIST_FILE_FROM_GIT} ]; then
	DIST_FILE="${DIST_FILE_FROM_GIT}"
elif [ -f ${DIST_FILE_FROM_FILE} ]; then
	DIST_FILE="${DIST_FILE_FROM_FILE}"
else
	echo "I could not find netdata distfile. Nor ${DIST_FILE_FROM_GIT} or ${DIST_FILE_FROM_FILE} exist"
	exit 1
fi

echo "Opening dist archive ${DIST_FILE}"
tar -xovf "${DIST_FILE}"
NETDATA_DIST_FOLDER=$(echo ${DIST_FILE} | cut -d. -f1,2,3)
if [ ! -d ${NETDATA_DIST_FOLDER} ]; then
	echo "I could not locate folder ${NETDATA_DIST_FOLDER}, something went wrong failing the test"
	exit 1
fi

echo "Entering ${NETDATA_DIST_FOLDER} and starting docker compilation"
cd ${NETDATA_DIST_FOLDER}
docker run -it -v "${PWD}:/code:rw" -w /code "netdata/os-test:centos7" /bin/bash -c "./netdata-installer.sh --dont-wait --install /tmp && echo \"Validating netdata instance is running\" && wget -O'-' 'http://127.0.0.1:19999/api/v1/info' | grep version"

echo "Installation completed with no errors! Removing temporary folders"

# TODO: Travis give me a permission denied on some files here, i made it a soft error until i figure out what is wrong
cd -
rm -rf ${NETDATA_DIST_FOLDER} || echo "I could not remove temporary directory, make sure you delete ${NETDATA_DIST_FOLDER} by yourself if this wasn't run over ephemeral storage"
