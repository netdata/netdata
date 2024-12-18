#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1
# Source of truth for all the packages we bundle in static builds
. "$(dirname "${0}")/../bundled-packages.version"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::building bash" || true

fetch "bash-${BASH_VERSION}" "${BASH_ARTIFACT_SOURCE}/bash-${BASH_VERSION}.tar.gz" \
    "${BASH_ARTIFACT_SHA256}" bash

export CFLAGS="${TUNING_FLAGS} -pipe"
export CXXFLAGS="${CFLAGS}"
export PKG_CONFIG_PATH="/openssl-static/lib64/pkgconfig"

if [ "${CACHE_HIT:-0}" -eq 0 ]; then
    run ./configure \
        --prefix="${NETDATA_INSTALL_PATH}" \
        --without-bash-malloc \
        --enable-static-link \
        --enable-net-redirections \
        --enable-array-variables \
        --disable-progcomp \
        --disable-profiling \
        --disable-nls \
        --disable-dependency-tracking

    run make clean
    run make -j "$(nproc)"

    cat > examples/loadables/Makefile <<-EOF
	all:
	clean:
	install:
	EOF
fi

run make install

store_cache bash "${NETDATA_MAKESELF_PATH}/tmp/bash-${BASH_VERSION}"

if [ "${NETDATA_BUILD_WITH_DEBUG}" -eq 0 ]; then
  run strip "${NETDATA_INSTALL_PATH}"/bin/bash
fi

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
