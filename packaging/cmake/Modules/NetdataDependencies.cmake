# SPDX-License-Identifier: GPL-3.0-or-later
#
# Every external library Netdata links against is looked up here, once.
#
# This file answers one question - "what does Netdata depend on?" - and only that
# question. It performs no policy: it does not decide whether a missing library is
# fatal, it does not set a HAVE_* or ENABLE_* macro, and it does not turn a feature
# off. Those decisions belong to whoever uses the result, next to the target that
# needs it, where the error message can say which feature is affected and how to
# build without it.
#
# So a lookup here carries no REQUIRED keyword. Read <PREFIX>_FOUND at the point of
# use and act on it there. pkg_check_modules and find_package write their results to
# the CACHE, so every result below is visible in every scope, including inside a
# function or a bundled subproject's own directory.
#
# The lookups are grouped by what consumes them, not by mechanism.

#
# Linked into libnetdata, and through it into everything else.
#

pkg_check_modules(CURL libcurl>=7.21 IMPORTED_TARGET)

pkg_check_modules(TLS IMPORTED_TARGET openssl)
pkg_check_modules(CRYPTO IMPORTED_TARGET libcrypto)

pkg_check_modules(LIBLZ4 liblz4>=1.7.1)
pkg_check_modules(LIBZSTD libzstd)
pkg_check_modules(LIBBROTLI libbrotlidec libbrotlienc libbrotlicommon)

pkg_check_modules(LIBUV libuv)

#
# Helper binaries.
#

pkg_check_modules(PCRE2 libpcre2-8)
