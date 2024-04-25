#!/bin/sh

FLUENT_BIT_SRC="${1}"
NEED_MUSL_FTS_PATCHES="${2}"

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
PATCH_DIR="$(dirname "$(dirname "${SCRIPT_SOURCE}")")"

cd "${FLUENT_BIT_SRC}" || exit 1

patch -p1 < "${PATCH_DIR}/CMakeLists.patch"
patch -p1 < "${PATCH_DIR}/flb-log-format.patch"

if [ "${NEED_MUSL_FTS_PATCHES:-0}" -eq 1 ]; then
    patch -p1 "${PATCH_DIR}/chunkio-static-lib-fts.patch"
    patch -p1 "${PATCH_DIR}/exclude-luajit.patch"
    patch -p1 "${PATCH_DIR}/xsi-strerror.patch"
fi
