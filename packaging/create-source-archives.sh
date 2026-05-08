#!/bin/sh

set -e

SCRIPT_SOURCE="$(
    self=${0}
    while [ -L "${self}" ]
    do
        cd "${self%/*}" || exit 1
        self=$(readlink "${self}")
    done
    cd "${self%/*}" || exit 1
    echo "$(pwd -P)/${self##*/}"
)"
SOURCE_DIR="$(dirname "$(dirname "${SCRIPT_SOURCE}")")"

cd "${SOURCE_DIR}" || exit 1

tar="$(command -v tar 2>/dev/null || true)"
gzip="$(command -v gzip 2>/dev/null || true)"
zstd="$(command -v zstd 2>/dev/null || true)"

[ -n "${tar}" ] || exit 1
[ -n "${gzip}" ] || exit 1
[ -n "${zstd}" ] || exit 1

mkdir -p artifacts artifacts/tmp

echo "Determining archive name..."

archive_name="netdata"
[ -d ./.git ] && archive_name="netdata-$(git describe)"
archive_tmp="artifacts/tmp/${archive_name}.tar"

echo "::group::Generating tar archive"

${tar} --create \
       --file "${archive_tmp}" \
       --sort=name \
       --posix \
       --exclude=./artifacts \
       --exclude=.git \
       --exclude=.gitignore \
       --exclude=.gitatributes \
       --exclude=.gitmodules \
       --transform "s/^\\.\\//${archive_name}\\//" \
       --verbose \
       .

echo "::endgroup::"

echo "::group::Creating gzip compressed archive"

${gzip} -v -k -9 "${archive_tmp}"

mv -v "${archive_tmp}.gz" "./artifacts"

echo "::endgroup::"

echo "::group::Creating zstd compressed archive"

${zstd} -v -19 -T0 "${archive_tmp}"

mv -v "${archive_tmp}.zst" "./artifacts"

echo "::endgroup::"

rm -v "${archive_tmp}"
