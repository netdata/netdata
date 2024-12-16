#!/bin/bash

# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/installer/functions.sh
. "$(dirname "$0")"/../installer/functions.sh || exit 1

BUILDARCH="${1}"

set -e

platform="$("$(dirname "${0}")/uname2platform.sh" "${BUILDARCH}")"

if [ -z "${platform}" ]; then
    exit 1
fi

if command -v docker > /dev/null 2>&1; then
    docker="docker"
elif command -v podman > /dev/null 2>&1; then
    docker="podman"
else
    echo "Could not find a usable OCI runtime, need either Docker or Podman."
    exit 1
fi

DOCKER_IMAGE_NAME="netdata/static-builder:v1"

if [ "${BUILDARCH}" != "$(uname -m)" ] && [ -z "${SKIP_EMULATION}" ]; then
    if [ "$(uname -m)" = "x86_64" ]; then
        ${docker} run --rm --privileged multiarch/qemu-user-static --reset -p yes || exit 1
    else
        echo "Automatic cross-architecture builds are only supported on x86_64 hosts."
        exit 1
    fi
fi

case "${BUILDARCH}" in
    x86_64) QEMU_CPU="Nehalem-v2" ;; # x86-64-v2 equivalent
    armv6l) # Raspberry Pi 1 equivalent
        QEMU_CPU="arm1176"
        CFLAGS="${CFLAGS} -march=armv6zk -mtune=arm1176jzf-s"
        CXXFLAGS="${CXXFLAGS} -march=armv6zk -mtune=arm1176jzf-s"
        ;;
    armv7l) QEMU_CPU="cortex-a7" ;; # Baseline ARMv7 CPU
    aarch64) QEMU_CPU="cortex-a53" ;; # Baseline ARMv8 CPU
    ppc64le) QEMU_CPU="power8nvl" ;; # Baseline POWER8+ CPU
esac

if ${docker} inspect "${DOCKER_IMAGE_NAME}" > /dev/null 2>&1; then
    if ${docker} image inspect "${DOCKER_IMAGE_NAME}" | grep -q 'Variant'; then
        img_platform="$(${docker} image inspect "${DOCKER_IMAGE_NAME}" --format '{{.Os}}/{{.Architecture}}/{{.Variant}}')"
    else
        img_platform="$(${docker} image inspect "${DOCKER_IMAGE_NAME}" --format '{{.Os}}/{{.Architecture}}')"
    fi

    if [ "${img_platform}" != "${platform}" ]; then
        ${docker} image rm "${DOCKER_IMAGE_NAME}" || exit 1
    fi
fi

if ! ${docker} inspect "${DOCKER_IMAGE_NAME}" > /dev/null 2>&1; then
    ${docker} pull --platform "${platform}" "${DOCKER_IMAGE_NAME}"
fi

# Run the build script inside the container
if [ -t 1 ]; then
  run ${docker} run --rm -e BUILDARCH="${BUILDARCH}" -a stdin -a stdout -a stderr -i -t -v "$(pwd)":/netdata:rw \
    --platform "${platform}" ${EXTRA_INSTALL_FLAGS:+-e EXTRA_INSTALL_FLAGS="${EXTRA_INSTALL_FLAGS}"} \
    ${QEMU_CPU:+-e QEMU_CPU="${QEMU_CPU}"} \
    ${CFLAGS:+-e CFLAGS="${CFLAGS}"} ${CXXFLAGS:+-e CXXFLAGS="${CXXFLAGS}"} \
    "${DOCKER_IMAGE_NAME}" /bin/sh /netdata/packaging/makeself/build.sh "${@}"
else
  run ${docker} run --rm -e BUILDARCH="${BUILDARCH}" -v "$(pwd)":/netdata:rw \
    -e GITHUB_ACTIONS="${GITHUB_ACTIONS}" --platform "${platform}" \
    ${EXTRA_INSTALL_FLAGS:+-e EXTRA_INSTALL_FLAGS="${EXTRA_INSTALL_FLAGS}"} \
    ${QEMU_CPU:+-e QEMU_CPU="${QEMU_CPU}"} \
    ${CFLAGS:+-e CFLAGS="${CFLAGS}"} ${CXXFLAGS:+-e CXXFLAGS="${CXXFLAGS}"} \
    "${DOCKER_IMAGE_NAME}" /bin/sh /netdata/packaging/makeself/build.sh "${@}"
fi
