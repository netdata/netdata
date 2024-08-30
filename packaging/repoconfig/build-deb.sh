#!/bin/sh

set -e

SRC_DIR="$(CDPATH='' cd -- "$(dirname -- "${0}")" && pwd -P)"
BUILD_DIR=/build
DISTRO="$(awk -F'=' '/^ID=/ {print $2}' /etc/os-release)"
DISTRO_VERSION="$(awk -F'"' '/VERSION_ID=/ {print $2}' /etc/os-release)"

# Needed because dpkg is stupid and tries to configure things interactively if it sees a terminal.
export DEBIAN_FRONTEND=noninteractive

echo "::group::Installing Build Dependencies"
apt update
apt upgrade -y
apt install -y --no-install-recommends ca-certificates cmake ninja-build curl gnupg
echo "::endgroup::"

echo "::group::Building Packages"
cmake -S "${SRC_DIR}" -B "${BUILD_DIR}" -G Ninja
cmake --build "${BUILD_DIR}"

cd "${BUILD_DIR}"
cpack -G DEB
echo "::endgroup::"

[ -d "${SRC_DIR}/artifacts" ] || mkdir -p "${SRC_DIR}/artifacts"

# Embed distro info in package name.
# This is required to make the repo actually standards compliant wthout packagecloud's hacks.
distid="${DISTRO}${DISTRO_VERSION}"
for pkg in "${BUILD_DIR}"/packages/*.deb; do
  extension="${pkg##*.}"
  pkgname="$(basename "${pkg}" "${extension}")"
  name="$(echo "${pkgname}" | cut -f 1 -d '_')"
  version="$(echo "${pkgname}" | cut -f 2 -d '_')"
  arch="$(echo "${pkgname}" | cut -f 3 -d '_')"

  newname="${SRC_DIR}/artifacts/${name}_${version}+${distid}_${arch}${extension}"
  mv "${pkg}" "${newname}"
done

# Correct ownership of the artifacts.
# Without this, the artifacts directory and it's contents end up owned
# by root instead of the local user on Linux boxes
chown -R --reference="${SRC_DIR}" "${SRC_DIR}/artifacts"
