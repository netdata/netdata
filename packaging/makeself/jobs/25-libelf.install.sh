#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1
# Source of truth for all the packages we bundle in static builds
. "$(dirname "${0}")/../bundled-packages.version"
# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Building OpenSSL" || true

export CFLAGS="${TUNING_FLAGS} -pipe"
export CXXFLAGS="${CFLAGS}"
#export LDFLAGS="-static"
#export PKG_CONFIG="pkg-config --static"

fetch_git libelf "${LIBELF_SOURCE}" "${LIBELF_VERSION}" "${LIBELF_VERSION}"

if [ "${CACHE_HIT:-0}" -eq 0 ]; then
  patch -p1 <<-'EOF'
	diff --git a/libelf/Makefile.am b/libelf/Makefile.am
	index 3402863e..2d3dbdf2 100644
	--- a/libelf/Makefile.am
	+++ b/libelf/Makefile.am
	@@ -122,6 +122,9 @@ libelf.so: $(srcdir)/libelf.map $(libelf_so_LIBS) $(libelf_so_DEPS)
	 	@$(textrel_check)
	 	$(AM_V_at)ln -fs $@ $@.$(VERSION)
	 
	+libeu_objects = $(shell $(AR) t ../lib/libeu.a)
	+libelf_a_LIBADD = $(addprefix ../lib/,$(libeu_objects))
	+
	 install: install-am libelf.so
	 	$(mkinstalldirs) $(DESTDIR)$(libdir)
	 	$(INSTALL_PROGRAM) libelf.so $(DESTDIR)$(libdir)/libelf-$(PACKAGE_VERSION).so
EOF

  run autoreconf -ivf

  run ./configure \
    --enable-year2038 \
    --enable-install-elfh \
    --enable-maintainer-mode \
    --disable-dependency-tracking \
    --disable-nls \
    --disable-libdebuginfod \
    --disable-debuginfod \
    --prefix /libelf-static

  run make -j "$(nproc)"
fi

run make -j "$(nproc)" install

if [ -d "/libelf-static/lib" ]; then
  cd "/libelf-static" || exit 1
  ln -s "lib" "lib64" || true
  cd - || exit 1
fi

store_cache libelf "${NETDATA_MAKESELF_PATH}/tmp/libelf"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
