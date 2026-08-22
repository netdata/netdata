# SPDX-License-Identifier: GPL-3.0-or-later
#
# Every external library Netdata links against is looked up here, once.
#
# This file answers one question - "what does Netdata depend on?" - and only that
# question. It performs no policy: it does not decide whether a missing library is
# fatal, it does not set a HAVE_* or ENABLE_* macro, and it does not turn a feature
# off. Those decisions belong to whoever uses the result, next to the target that
# needs it, where the error message can name the feature and say how to build
# without it.
#
# So no lookup here carries REQUIRED, and none carries the OS or feature guard its
# consumer has. Probing for a library a platform does not have is harmless - the
# result is simply not-found - and it keeps the answer to "what do we depend on"
# free of the question "what are we building today".
#
# pkg_check_modules and find_package write their results to the CACHE, so every
# result below is readable from every scope, including inside a function and inside
# a bundled subproject's own directory.
#
# find_package(PkgConfig REQUIRED) is deliberately NOT here. It stays in the root
# file, where STATIC_BUILD appends --static to PKG_CONFIG_EXECUTABLE on the line
# after it; that append needs the variable the find_package defines, so the two
# cannot be separated.
#
# Four groups of external resolution stay where they are, listed here so this file
# answers "what do we depend on" by enumeration even where it does not by
# execution:
#
#   json-c, libyaml, protobuf     NetdataJSONC.cmake, NetdataYAML.cmake,
#                                 NetdataProtobuf.cmake. Each of those modules
#                                 exists to choose between the system copy and a
#                                 bundled one. The lookup is one branch of that
#                                 choice, so extracting it would leave a module
#                                 that bundles but cannot decide whether to.
#
#   CUPS                          NetdataPluginCups.cmake. Three mechanisms in
#                                 sequence: pkg-config libcups, then pkg-config
#                                 cups, then cups-config through find_program and
#                                 execute_process, with an API-version floor. A
#                                 host that resolves only by the last one would
#                                 read CUPS_FOUND false from here and ship a
#                                 package with no cups.plugin.
#
#   PkgConfig, Go, Python3, Git   Tools, not libraries. A missing tool switches a
#                                 feature off or fails a build step; it is not a
#                                 link error, and the resolution is find_program
#                                 rather than a library search.
#
#   IOKit, Foundation, OSLog      NetdataPlatform.cmake, next to the platform
#                                 detection that selects them. Part of being on
#                                 macOS rather than part of what Netdata depends
#                                 on.

#
# Linked into libnetdata, and through it into everything that links libnetdata.
#

pkg_check_modules(CURL libcurl>=7.21 IMPORTED_TARGET)

pkg_check_modules(TLS IMPORTED_TARGET openssl)
pkg_check_modules(CRYPTO IMPORTED_TARGET libcrypto)

pkg_check_modules(LIBLZ4 liblz4>=1.7.1)
pkg_check_modules(LIBZSTD libzstd)
pkg_check_modules(LIBBROTLI libbrotlidec libbrotlienc libbrotlicommon)

pkg_check_modules(LIBUV libuv)
pkg_check_modules(UUID uuid)
pkg_check_modules(MNL libmnl)
pkg_check_modules(SYSTEMD libsystemd)
pkg_check_modules(LIBUNWIND libunwind IMPORTED_TARGET)

# zlib is the one dependency resolved by two mechanisms, and they are not
# interchangeable. The macOS consumer links the ZLIB::ZLIB imported target, which
# only FindZLIB creates - pkg-config does not. Trying a pkg-config lookup first
# and falling back would therefore break every macOS host where pkg-config can
# see zlib, and it would neuter CMAKE_DISABLE_FIND_PACKAGE_ZLIB as well. The OS
# branch moves here intact instead: one place, one decision, two mechanisms.
if(OS_MACOS)
  find_package(ZLIB)
else()
  pkg_check_modules(ZLIB zlib)
endif()

#
# Helper binaries.
#

pkg_check_modules(PCRE2 libpcre2-8)
pkg_check_modules(CAP IMPORTED_TARGET libcap)

#
# Collector plugins.
#

pkg_check_modules(ODBC odbc)
pkg_check_modules(IPMI libipmimonitoring)
pkg_check_modules(NFACCT libnetfilter_acct)
pkg_check_modules(XENSTAT xenstat)
pkg_check_modules(XENLIGHT xenlight)

#
# Exporters.
#

pkg_check_modules(MONGOC libmongoc-1.0>=1.7)
pkg_check_modules(SNAPPY snappy)

#
# Build inputs for a subproject we compile ourselves.
#

pkg_check_modules(ELF libelf)
