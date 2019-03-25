#!/bin/bash
#
# This is the nightlies orchastration script
# It runs the following activities in order:
# 1) Generate changelog
# 2) Build docker images
# 3) Publish docker images
# 4) Generate the rest of the artifacts (Source code .tar.gz file and makeself binary generation)
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pawel Krupa (paulfantom)
# Author  : Pavlos Emm. Katsoulakis (paul@netdata.cloud)
set -e

FAIL=0

# If we are not in netdata git repo, at the top level directory, fail
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup)
if [ ! -z $CWD ] || [ ! "${TOP_LEVEL}" == "netdata" ]; then
    echo "Run as .travis/$(basename "$0") from top level directory of netdata git repository"
    echo "Changelog generation process aborted"
    exit 1
fi

echo "--- Running Changelog generation ---"
.travis/generate_changelog.sh || echo "Changelog generation has failed, this is a soft error, process continues"

echo "--- Build docker images ---"
packaging/docker/build.sh
echo "--- Publish docker images ---"
packaging/docker/publish.sh

echo "--- Build artifacts ---"
.travis/create_artifacts.sh

exit "${FAIL}"
