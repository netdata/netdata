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

DOCKER_CONTAINER_NAME="netdata-package-${BUILDARCH}-static-alpine315"

if [ "${BUILDARCH}" != "$(uname -m)" ] && [ "$(uname -m)" = 'x86_64' ] && [ -z "${SKIP_EMULATION}" ]; then
    docker run --rm --privileged multiarch/qemu-user-static --reset -p yes || exit 1
fi

if ! docker inspect "${DOCKER_CONTAINER_NAME}" > /dev/null 2>&1; then
  # To run interactively:
  #   docker run -it netdata-package-x86_64-static /bin/sh
  # (add -v host-dir:guest-dir:rw arguments to mount volumes)
  #
  # To remove images in order to re-create:
  #   docker rm -v $(sudo docker ps -a -q -f status=exited)
  #   docker rmi netdata-package-x86_64-static
  #
  # This command maps the current directory to
  #   /usr/src/netdata.git
  # inside the container and runs the script install-alpine-packages.sh
  # (also inside the container)
  #
  if docker inspect alpine:3.15 > /dev/null 2>&1; then
    run docker image remove alpine:3.15
    run docker pull --platform=${platform}  alpine:3.15
  fi

  run docker run --platform=${platform} -v "$(pwd)":/usr/src/netdata.git:rw alpine:3.15 \
    /bin/sh /usr/src/netdata.git/packaging/makeself/install-alpine-packages.sh

  # save the changes made permanently
  id=$(docker ps -l -q)
  run docker commit "${id}" "${DOCKER_CONTAINER_NAME}"
fi

# Run the build script inside the container
if [ -t 1 ]; then
  run docker run -e BUILDARCH="${BUILDARCH}" -a stdin -a stdout -a stderr -i -t -v "$(pwd)":/usr/src/netdata.git:rw \
    "${DOCKER_CONTAINER_NAME}" \
    /bin/sh /usr/src/netdata.git/packaging/makeself/build.sh "${@}"
else
  run docker run -e BUILDARCH="${BUILDARCH}" -v "$(pwd)":/usr/src/netdata.git:rw \
    -e GITHUB_ACTIONS="${GITHUB_ACTIONS}" "${DOCKER_CONTAINER_NAME}" \
    /bin/sh /usr/src/netdata.git/packaging/makeself/build.sh "${@}"
fi

if [ "${USER}" ]; then
  sudo chown -R "${USER}" .
fi
