#!/usr/bin/env bash

# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/installer/functions.sh
. "$(dirname "$0")"/../installer/functions.sh || exit 1

BUILDARCH="${1}"

set -e

case ${BUILDARCH} in
  x86_64) platform=linux/amd64 ;;
  armv7l) platform=linux/arm/v7 ;;
  aarch64) platform=linux/arm64/v8 ;;
  ppc64le) platform=linux/ppc64le ;;
  *)
    echo "Unknown target architecture '${BUILDARCH}'."
    exit 1
    ;;
esac

DOCKER_IMAGE_NAME="netdata/static-builder"

if [ "${BUILDARCH}" != "$(uname -m)" ] && [ "$(uname -m)" = 'x86_64' ] && [ -z "${SKIP_EMULATION}" ]; then
    docker run --rm --privileged multiarch/qemu-user-static --reset -p yes || exit 1
fi

if [ "${USE_EXISTING_DOCKER_IMAGE}" != 'yes' ] && docker inspect "${DOCKER_IMAGE_NAME}" > /dev/null 2>&1; then
    docker image rm "${DOCKER_IMAGE_NAME}" || exit 1
fi

if ! docker inspect "${DOCKER_IMAGE_NAME}" > /dev/null 2>&1; then
    docker pull --platform "${platform}" "${DOCKER_IMAGE_NAME}"
fi

# Run the build script inside the container
if [ -t 1 ]; then
  run docker run -e BUILDARCH="${BUILDARCH}" -a stdin -a stdout -a stderr -i -t -v "$(pwd)":/netdata:rw \
    "${DOCKER_IMAGE_NAME}" \
    /bin/sh /netdata/packaging/makeself/build.sh "${@}"
else
  run docker run -e BUILDARCH="${BUILDARCH}" -v "$(pwd)":/netdata:rw \
    -e GITHUB_ACTIONS="${GITHUB_ACTIONS}" "${DOCKER_IMAGE_NAME}" \
    /bin/sh /netdata/packaging/makeself/build.sh "${@}"
fi
