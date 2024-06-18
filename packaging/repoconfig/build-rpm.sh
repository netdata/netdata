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
    yum install -y rpm-build make || exit 1
    curl --fail -sSL --connect-timeout 20 --retry 3 --output "cmake-linux-$(uname -m).sh" "https://github.com/Kitware/CMake/releases/download/v3.27.6/cmake-3.27.6-linux-$(uname -m).sh" && \
    if [ "$(uname -m)" = "x86_64" ]; then \
        echo '8c449dabb2b2563ec4e6d5e0fb0ae09e729680efab71527b59015131cea4a042  cmake-linux-x86_64.sh' | sha256sum -c - ; \
    elif [ "$(uname -m)" = "aarch64" ]; then \
        echo 'a83e01ed1cdf44c2e33e0726513b9a35a8c09e3b5a126fd720b3c8a9d5552368  cmake-linux-aarch64.sh' | sha256sum -c - ; \
    else \
        echo "ARCH NOT SUPPORTED BY CMAKE" ; \
        exit 1 ; \
    fi && \
    chmod +x "./cmake-linux-$(uname -m).sh" && \
    mkdir -p /cmake && \
    "./cmake-linux-$(uname -m).sh" --skip-license --prefix=/cmake
    PATH="/cmake/bin:${PATH}"
elif command -v zypper > /dev/null ; then
    zypper update -y || exit 1
    zypper install -y rpm-build cmake make || exit 1
fi
echo "::endgroup::"

echo "::group::Building Packages"
cmake -S "${SRC_DIR}" -B "${BUILD_DIR}"
cmake --build "${BUILD_DIR}"

cd "${BUILD_DIR}"
cpack -G RPM
echo "::endgroup::"

[ -d "${SRC_DIR}/artifacts" ] || mkdir -p "${SRC_DIR}/artifacts"

find "${BUILD_DIR}/packages/" -type f -name '*.rpm' -exec cp '{}' "${SRC_DIR}/artifacts" \; || exit 1

chown -R --reference="${SRC_DIR}" "${SRC_DIR}/artifacts"
