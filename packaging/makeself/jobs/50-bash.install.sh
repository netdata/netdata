#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1
# Source of truth for all the packages we bundle in static builds
. "$(dirname "${0}")/../bundled-packages.version"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::building bash" || true

# For some stupid reason, the GNU bash project does not actually tag
# releases, so we need to use a branch ofr the version, and that in turn
# only tracks down to the minor version. Thus, we need to cache by day to
# ensure we get the latest copy.
cache_key="${BASH_VERSION}-$(date +%F)"
fetch_git bash "${BASH_REPO}" "${BASH_VERSION}" "${cache_key}"

export CFLAGS="${TUNING_FLAGS} -pipe"
export CXXFLAGS="${CFLAGS}"
export PKG_CONFIG_PATH="/openssl-static/lib64/pkgconfig"

if [ "${CACHE_HIT:-0}" -eq 0 ]; then
    run ./configure \
        --prefix="${NETDATA_INSTALL_PATH}" \
        --without-bash-malloc \
        --with-installed-readline \
        --enable-static-link \
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

store_cache "${cache_key}" "${NETDATA_MAKESELF_PATH}/tmp/bash"

if [ "${NETDATA_BUILD_WITH_DEBUG}" -eq 0 ]; then
  run strip "${NETDATA_INSTALL_PATH}"/bin/bash
fi

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
