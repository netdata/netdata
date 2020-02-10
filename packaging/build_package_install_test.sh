#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later

set -e

# If we are not in netdata git repo, at the top level directory, FAIL
TOP_LEVEL=$(basename "$(git rev-parse --show-toplevel)")
CWD=$(git rev-parse --show-cdup || echo "")
if [ -n "${CWD}" ] || [ ! "${TOP_LEVEL}" = "netdata" ]; then
  echo "Run as ./packaging/$(basename "$0") from top level directory of netdata git repository"
  exit 1
fi

if [ $# -lt 2 ] || [ $# -gt 3 ]; then
  echo "Usage: ./packaging/$(basename "$0") <distro> <distro_version> [<netdata_version>]"
  exit 1
fi

if ! command -v docker > /dev/null; then
  echo "Docker CLI not found. You need Docker to run this!"
  exit 2
fi

DISTRO="$1"
DISTRO_VERSION="$2"
# TODO: Auto compute this?
VERSION="${3:-1.19.0}"

TAG="netdata/netdata:${DISTRO}_${DISTRO_VERSION}"

docker build \
  -f ./packaging/Dockerfile.packager \
  --build-arg DISTRO="$DISTRO" \
  --build-arg DISTRO_VERSION="$DISTRO_VERSION" \
  --build-arg VERSION="$VERSION" \
  -t "$TAG" . |
  tee build.log
