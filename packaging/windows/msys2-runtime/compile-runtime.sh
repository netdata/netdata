#!/bin/bash
#
# Build the patched MSYS2 runtime DLL (see runtime.env for why) the way MSYS2's msys2-runtime PKGBUILD does.
#
# Usage: compile-runtime.sh <output-dir>
#
# Writes <output-dir>/msys-2.0.dll (stripped, with a debug link) and <output-dir>/msys-2.0.dbg.

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)"

# shellcheck source=./runtime.env disable=SC1091
. "${here}/runtime.env"

if [ $# -ne 1 ]; then
    echo "Usage: $0 <output-dir>" >&2
    exit 1
fi

# Absolute, because the script changes directories before writing the output.
out="$(realpath -m "$1")"
work="$(mktemp -d)"
trap 'rm -rf "${work}"' EXIT

${GITHUB_ACTIONS+echo "::group::Installing MSYS2 runtime build dependencies"}
pacman -S --noconfirm --needed \
    autotools \
    cocom \
    docbook-xsl \
    gettext-devel \
    mingw-w64-cross-crt \
    mingw-w64-cross-gcc \
    mingw-w64-cross-zlib \
    xmlto
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Fetching and patching msys2-runtime"}
git init -q "${work}/src"
cd "${work}/src"
git config core.symlinks true
git remote add origin "${MSYS2_RUNTIME_REPO}"
git fetch -q --depth=1 origin "${MSYS2_RUNTIME_BASE_COMMIT}"
git checkout -q --detach FETCH_HEAD

GIT_COMMITTER_NAME="${MSYS2_RUNTIME_COMMITTER_NAME}" \
GIT_COMMITTER_EMAIL="${MSYS2_RUNTIME_COMMITTER_EMAIL}" \
GIT_COMMITTER_DATE="${MSYS2_RUNTIME_COMMITTER_DATE}" \
    git -c commit.gpgsign=false am -q "${here}"/*.patch

commit="$(git rev-parse HEAD)"
if [ "${commit}" != "${MSYS2_RUNTIME_PATCHED_COMMIT}" ]; then
    echo "Patched msys2-runtime is at ${commit}, expected ${MSYS2_RUNTIME_PATCHED_COMMIT}" >&2
    exit 1
fi
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Building msys2-runtime"}
(cd "${work}/src/winsup" && ./autogen.sh)

mkdir "${work}/build"
cd "${work}/build"

# Same flags as MSYS2's PKGBUILD; CYGPORT_RELEASE_INFO keeps git from appending "-dirty" to the version.
export CFLAGS="-O2 -pipe -ggdb -DCYGPORT_RELEASE_INFO=${MSYS2_RUNTIME_VERSION}"
export CXXFLAGS="${CFLAGS}"

"${work}/src/configure" \
    --with-msys2-runtime-commit="${MSYS2_RUNTIME_PATCHED_COMMIT}" \
    --prefix=/usr \
    --build=x86_64-pc-cygwin \
    --sysconfdir=/etc

LC_ALL=C make -j"$(nproc)"
LC_ALL=C make -j1 DESTDIR="${work}/dest" install
${GITHUB_ACTIONS+echo "::endgroup::"}

# makepkg splits the debug info out and strips; do the same so the DLL matches the official one.
dll="${work}/dest/usr/bin/msys-2.0.dll"
objcopy --only-keep-debug "${dll}" "${work}/msys-2.0.dbg"
strip --strip-unneeded "${dll}"
(cd "${work}" && objcopy --add-gnu-debuglink=msys-2.0.dbg "${dll}")

mkdir -p "${out}"
cp "${work}/msys-2.0.dbg" "${out}/msys-2.0.dbg"
cp "${dll}" "${out}/msys-2.0.dll.tmp"
mv "${out}/msys-2.0.dll.tmp" "${out}/msys-2.0.dll"

echo "Built ${out}/msys-2.0.dll (msys2-runtime ${MSYS2_RUNTIME_VERSION}-${MSYS2_RUNTIME_PATCHED_COMMIT})"
