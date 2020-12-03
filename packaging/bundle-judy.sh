#!/bin/sh

JUDY_TARBALL="v$(cat "${1}/packaging/judy.version").tar.gz"
JUDY_BUILD_PATH="${1}/externaldeps/libJudy/libjudy-$(cat "${1}/packaging/judy.version")"

mkdir -p "${1}/externaldeps/libJudy" || exit 1
curl -sSL --connect-timeout 10 --retry 3 "https://github.com/netdata/libjudy/archive/${JUDY_TARBALL}" > "${JUDY_TARBALL}" || exit 1
sha256sum -c "${1}/packaging/judy.checksums" || exit 1
tar -xzf "${JUDY_TARBALL}" -C "${1}/externaldeps/libJudy" || exit 1
OLDPWD="${PWD}"
cd "${JUDY_BUILD_PATH}" || exit 1
libtoolize --force --copy || exit 1
aclocal || exit 1
autoheader || exit 1
automake --add-missing --force --copy --include-deps || exit 1
autoconf || exit 1
./configure || exit 1
make -C src || exit 1
ar -r src/libJudy.a src/Judy*/*.o || exit 1
cd "${OLDPWD}" || exit 1

cp -a "${JUDY_BUILD_PATH}/src/libJudy.a" "${1}/externaldeps/libJudy" || exit 1
cp -a "${JUDY_BUILD_PATH}/src/Judy.h" "${1}/externaldeps/libJudy" || exit 1
