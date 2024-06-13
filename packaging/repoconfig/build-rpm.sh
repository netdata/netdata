#!/bin/sh

set -e

SRC_DIR="$(CDPATH='' cd -- "$(dirname -- "${0}")" && pwd -P)"
BUILD_DIR=/build

echo "::group::Installing Build Dependencies"
if command -v dnf > /dev/null ; then
    dnf distro-sync -y --nodocs || exit 1
    dnf install -y --nodocs --setopt=install_weak_deps=False rpm-build cmake make || exit 1
elif command -v yum > /dev/null ; then
    yum distro-sync -y || exit 1
    yum install -y rpm-build cmake make || exit 1
elif command -v zypper > /dev/null ; then
    zypper update -y || exit 1
    zypper install -y rpm-build cmake make || exit 1
fi
echo "::endgroup::"

echo "::group::Building Packages"
if [ "$(rpm --eval '%{centos}' | cut -f 1 -d '.')" -lt 8 ]; then
    mkdir -p "${BUILD_DIR}"
    cp -rv "${SRC_DIR}" "${BUILD_DIR}"
    cd "${BUILD_DIR}"
    cmake .
    cmake --build .
else
    cmake -S "${SRC_DIR}" -B "${BUILD_DIR}"
    cmake --build "${BUILD_DIR}"
fi

cd "${BUILD_DIR}"
cpack -G RPM
echo "::endgroup::"

[ -d "${SRC_DIR}/artifacts" ] || mkdir -p "${SRC_DIR}/artifacts"

find "${BUILD_DIR}/packages/" -type f -name '*.rpm' -exec cp '{}' "${SRC_DIR}/artifacts" \; || exit 1

chown -R --reference="${SRC_DIR}" "${SRC_DIR}/artifacts"
