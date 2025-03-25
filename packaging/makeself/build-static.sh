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

case "${BUILDARCH}" in
    x86_64) # x86-64-v2 equivalent
        QEMU_ARCH="x86_64"
        QEMU_CPU="Nehalem-v2"
        TUNING_FLAGS="-march=x86-64"
        GOAMD64="v1"
        ;;
    armv6l) # Raspberry Pi 1 equivalent
        QEMU_ARCH="arm"
        QEMU_CPU="arm1176"
        TUNING_FLAGS="-march=armv6zk -mtune=arm1176jzf-s"
        GOARM="6"
        ;;
    armv7l) # Baseline ARMv7 CPU
        QEMU_ARCH="arm"
        QEMU_CPU="cortex-a7"
        TUNING_FLAGS="-march=armv7-a"
        GOARM="7"
        ;;
    aarch64) # Baseline ARMv8 CPU
        QEMU_ARCH="aarch64"
        QEMU_CPU="cortex-a53"
        TUNING_FLAGS="-march=armv8-a"
        GOARM64="v8.0"
        ;;
    ppc64le) # Baseline POWER8+ CPU
        QEMU_ARCH="ppc64le"
        QEMU_CPU="power8nvl"
        TUNING_FLAGS="-mcpu=power8 -mtune=power9"
        GOPPC64="power8"
        ;;
esac

[ -f "/proc/sys/fs/binfmt_misc/qemu-${QEMU_ARCH}" ] && SKIP_EMULATION=1

if [ "${BUILDARCH}" != "$(uname -m)" ] && [ -z "${SKIP_EMULATION}" ]; then
    ${docker} run --rm --privileged tonistiigi/binfmt:master --install "${QEMU_ARCH}" || exit 1
fi

DOCKER_IMAGE_NAME="netdata/static-builder:v1"

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
    ${DEBUG_BUILD_INFRA:+-e DEBUG_BUILD_INFRA=1} \
    ${QEMU_CPU:+-e QEMU_CPU="${QEMU_CPU}"} \
    -e TUNING_FLAGS="${TUNING_FLAGS}" \
    ${GOAMD64:+-e GOAMD64="${GOAMD64}"} ${GOARM:+-e GOARM="${GOARM}"} \
    ${GOARM64:+-e GOARM64="${GOARM64}"} ${GOPPC64:+-e GOPPC64="${GOPPC64}"} \
    "${DOCKER_IMAGE_NAME}" /bin/sh /netdata/packaging/makeself/build.sh "${@}"
else
  run ${docker} run --rm -e BUILDARCH="${BUILDARCH}" -v "$(pwd)":/netdata:rw \
    -e GITHUB_ACTIONS="${GITHUB_ACTIONS}" --platform "${platform}" \
    ${EXTRA_INSTALL_FLAGS:+-e EXTRA_INSTALL_FLAGS="${EXTRA_INSTALL_FLAGS}"} \
    ${QEMU_CPU:+-e QEMU_CPU="${QEMU_CPU}"} \
    -e TUNING_FLAGS="${TUNING_FLAGS}" \
    ${GOAMD64:+-e GOAMD64="${GOAMD64}"} ${GOARM:+-e GOARM="${GOARM}"} \
    ${GOARM64:+-e GOARM64="${GOARM64}"} ${GOPPC64:+-e GOPPC64="${GOPPC64}"} \
    "${DOCKER_IMAGE_NAME}" /bin/sh /netdata/packaging/makeself/build.sh "${@}"
fi
