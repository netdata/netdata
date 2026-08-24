# SPDX-License-Identifier: GPL-3.0-or-later
#
# Every external library Netdata links against is looked up here, once.
#
# It answers that in two halves. First every lookup: what we go and look for, with no
# REQUIRED and no guard, because probing for a library a platform does not have is
# harmless and it keeps "what do we depend on" free of "what are we building today".
# Then every requirement: which of those results are mandatory, and under what
# condition. Read together, the two halves are the whole dependency contract, and
# reading them is not a grep across seven files.
#
# What stays out of the second half is derivation. Whether a dependency is mandatory
# is a fact about the dependency and belongs here; what a found dependency switches
# on is a fact about the feature, so every HAVE_* and ENABLE_* is still set next to
# the target it configures. ENABLE_SYSTEMD_DBUS could not move anyway - it depends on
# five capability probes that run much later.
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

include_guard()

#
# Linked into libnetdata, and through it into everything that links libnetdata.
#

pkg_check_modules(CURL libcurl>=7.21 IMPORTED_TARGET)



pkg_check_modules(UUID uuid)
# libmnl wraps Linux netlink; a Homebrew libmnl.pc on macOS is a false positive
# that would hand network-viewer and local-listeners a dylib they never call.
if(NOT OS_MACOS)
  pkg_check_modules(MNL libmnl)
endif()
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

# pcre2 gates only log2journal, which the macOS package deliberately does not
# ship; skipping the lookup under the pkg kind keeps a build-host copy out of
# the payload instead of adding pcre2 to the bundling set. The explicit FALSE
# matters: pkg_check_modules caches its result, so a build directory that
# already probed pcre2 would otherwise keep building log2journal after a
# reconfigure to the pkg kind.
if(NETDATA_PACKAGE_KIND STREQUAL "pkg")
  set(PCRE2_FOUND FALSE)
else()
  pkg_check_modules(PCRE2 libpcre2-8)
endif()
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

#
# Build inputs for a subproject we compile ourselves.
#

pkg_check_modules(ELF libelf)

#
# Requirements.
#
# A missing library is fatal here or nowhere. Each check carries the condition under
# which the dependency is actually needed, so it fires in exactly the cases it fired
# in when it lived next to its target - and it aborts earlier, before any of the work
# that was going to fail anyway.
#
# The message does the naming, not the location: it says which feature is affected
# and which option turns that feature off.
#

if(NOT CURL_FOUND)
  message(FATAL_ERROR "libcurl >= 7.21 is required for building Netdata, but could not be found.")
endif()

if(NOT ZLIB_FOUND)
  message(FATAL_ERROR "zlib is required for building Netdata, but could not be found.")
endif()

# macOS and Windows provide the UUID functions in their system libraries, so the OS
# condition is the requirement rather than an optimisation of the lookup.
if(NOT (OS_MACOS OR OS_WINDOWS) AND NOT UUID_FOUND)
  message(FATAL_ERROR "libuuid is required for building Netdata on this platform, but could not be found.")
endif()

if(ENABLE_LIBUNWIND AND NOT TARGET PkgConfig::LIBUNWIND)
  message(FATAL_ERROR "Could not find libunwind for logging of stack traces")
endif()

if(ENABLE_PLUGIN_EBPF AND NOT ELF_FOUND)
  message(FATAL_ERROR "The eBPF plugin needs libelf, but it could not be found. Pass -DENABLE_PLUGIN_EBPF=Off to build without it.")
endif()

if(ENABLE_PLUGIN_FREEIPMI AND NOT IPMI_FOUND)
  message(FATAL_ERROR "The freeipmi plugin needs libipmimonitoring, but it could not be found. Pass -DENABLE_PLUGIN_FREEIPMI=Off to build without it.")
endif()

if(ENABLE_PLUGIN_IBM AND NOT ODBC_FOUND)
  message(FATAL_ERROR "The IBM plugin needs unixODBC, but it could not be found. Pass -DENABLE_PLUGIN_IBM=Off to build without it.")
endif()

if(ENABLE_PLUGIN_NFACCT AND NOT MNL_FOUND)
  message(FATAL_ERROR "Can not build nfacct.plugin because MNL library could not be found.")
endif()

if(ENABLE_PLUGIN_NFACCT AND NOT NFACCT_FOUND)
  message(FATAL_ERROR "The nfacct plugin needs libnetfilter_acct, but it could not be found. Pass -DENABLE_PLUGIN_NFACCT=Off to build without it.")
endif()

if(ENABLE_PLUGIN_SYSTEMD_UNITS AND NOT SYSTEMD_FOUND)
  message(FATAL_ERROR "Systemd units plugin requires systemd, but systemd was not found.")
endif()

# Reached only when the check above did not abort, so systemd is known to be present.
# VERSION_LESS, not LESS: pkg-config can report shapes like 251.rc1, which a
# numeric comparison parses as 0 and falsely rejects.
if(ENABLE_PLUGIN_SYSTEMD_UNITS AND SYSTEMD_VERSION VERSION_LESS 221)
  message(FATAL_ERROR "Systemd units plugin requires systemd 221 or newer, but only systemd ${SYSTEMD_VERSION} was found.")
endif()

if(ENABLE_PLUGIN_XENSTAT AND NOT XENSTAT_FOUND)
  message(FATAL_ERROR "The xenstat plugin needs xenstat, but it could not be found. Pass -DENABLE_PLUGIN_XENSTAT=Off to build without it.")
endif()

if(ENABLE_PLUGIN_XENSTAT AND NOT XENLIGHT_FOUND)
  message(FATAL_ERROR "The xenstat plugin needs xenlight, but it could not be found. Pass -DENABLE_PLUGIN_XENSTAT=Off to build without it.")
endif()
