#!/bin/sh

set -e

SRC_DIR="$(CDPATH='' cd -- "$(dirname -- "${0}")" && pwd -P)"
BUILD_DIR=/build
. /etc/os-release

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

# Check the version APT will compare and the filename the installer will request.
for pkg in "${BUILD_DIR}"/packages/*.deb; do
  version="$(dpkg-deb -f "${pkg}" Version)"
  case "${version}" in
    *+"${ID}${VERSION_ID}") ;;
    *) printf 'Unexpected package version %s in %s\n' "${version}" "${pkg}" >&2; exit 1 ;;
  esac
  name="$(dpkg-deb -f "${pkg}" Package)"
  test "${pkg##*/}" = "${name}_${version}_all.deb"
  mv "${pkg}" "${SRC_DIR}/artifacts/"
done

# Correct ownership of the artifacts.
# Without this, the artifacts directory and it's contents end up owned
# by root instead of the local user on Linux boxes
chown -R --reference="${SRC_DIR}" "${SRC_DIR}/artifacts"
