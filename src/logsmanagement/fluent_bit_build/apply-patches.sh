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
PATCH_DIR="$(dirname "${SCRIPT_SOURCE}")"

cd "${FLUENT_BIT_SRC}" || exit 1

patch -p1 -i "${PATCH_DIR}/CMakeLists.patch" || exit 1
patch -p1 -i "${PATCH_DIR}/flb-log-fmt.patch" || exit 1
patch -p1 -i "${PATCH_DIR}/flb-ninja.patch" || exit 1

if [ "${NEED_MUSL_FTS_PATCHES:-0}" -eq 1 ]; then
    patch -p1 -i "${PATCH_DIR}/chunkio-static-lib-fts.patch" || exit 1
    patch -p1 -i "${PATCH_DIR}/exclude-luajit.patch" || exit 1
    patch -p1 -i "${PATCH_DIR}/xsi-strerror.patch" || exit 1
fi
