# SPDX-License-Identifier: GPL-3.0-or-later
# The install layout: every Netdata-owned path once, as a *_DEST/*_DIR pair, plus the host-owned destinations and the staging predicate.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.

if(OS_WINDOWS)
  string(REGEX REPLACE "[/\\\\]+$" "" NETDATA_WINDOWS_PATH_PREFIX "${NETDATA_WINDOWS_PATH_PREFIX}")
  string(REPLACE "\\" "\\\\" NETDATA_WINDOWS_PATH_PREFIX_ESCAPED "${NETDATA_WINDOWS_PATH_PREFIX}")

  # Both arms of the Windows prefix decision, in one place. A native package
  # owns the filesystem root and addresses everything from it; any other Windows
  # build is relocatable under the install prefix. NetdataPlatform used to set
  # the packaging arm, which made it read an option defined ninety lines later.
  if(BUILD_FOR_PACKAGING)
    set(NETDATA_RUNTIME_PREFIX "/")
  else()
    netdata_windows_path_to_runtime_path(NETDATA_RUNTIME_PREFIX "${NETDATA_WINDOWS_PATH_PREFIX}")
  endif()
endif()

if(NOT NETDATA_RUNTIME_PREFIX STREQUAL "")
  string(REGEX REPLACE "/$" "" NETDATA_RUNTIME_PREFIX "${NETDATA_RUNTIME_PREFIX}")
endif()

# Install layout: each Netdata-owned path is defined once here, as a *_DEST
# (install-relative, for install(DESTINATION ...)) plus a *_DIR (runtime
# absolute) derived from it, so the staged tree and the runtime paths cannot
# drift apart. New install rules must use a *_DEST variable, not a literal path.
#
# This covers install() destinations only. The CPack manifests and the RPM file
# lists in packaging/cmake/Modules/ still spell the layout out, deliberately,
# because they describe package ownership rather than staging; they do not read
# these variables.
#
# *_DIR reaches the rest of the product through config.h (C), -ldflags (Go),
# corrosion env vars (Rust) and configure_file (shell, service units); the daemon
# re-exports them to its child plugins as NETDATA_*_DIR. BINDIR/NETDATA_BIN_DIR
# are the same pairing for the daemon binary, with BINDIR set before
# include(NetdataPlatform) because Windows overrides it.
set(LIBEXEC_DEST "usr/libexec/netdata")
set(PLUGINS_DEST "${LIBEXEC_DEST}/plugins.d")
set(LIBDIR_DEST "usr/lib/netdata")
set(LIBCONFIG_DEST "${LIBDIR_DEST}/conf.d")
set(SYSTEM_DEST "${LIBDIR_DEST}/system")
set(CONFIG_DEST "etc/netdata")
set(CACHE_DEST "var/cache/netdata")
set(LOG_DEST "var/log/netdata")
set(VARLIB_DEST "var/lib/netdata")
set(RUN_DEST "var/run/netdata")
set(STOCK_DATA_DEST "usr/share/netdata")

set(CACHE_DIR "${NETDATA_RUNTIME_PREFIX}/${CACHE_DEST}")
set(CONFIG_DIR "${NETDATA_RUNTIME_PREFIX}/${CONFIG_DEST}")
set(LIBCONFIG_DIR "${NETDATA_RUNTIME_PREFIX}/${LIBCONFIG_DEST}")
set(LOG_DIR "${NETDATA_RUNTIME_PREFIX}/${LOG_DEST}")
set(PLUGINS_DIR "${NETDATA_RUNTIME_PREFIX}/${PLUGINS_DEST}")
set(STOCK_DATA_DIR "${NETDATA_RUNTIME_PREFIX}/${STOCK_DATA_DEST}")
set(VARLIB_DIR "${NETDATA_RUNTIME_PREFIX}/${VARLIB_DEST}")
set(NETDATA_BIN_DIR "${NETDATA_RUNTIME_PREFIX}/${BINDIR}")
# WEB_DIR arrives install-relative and is declared in the options module. A
# leading slash is tolerated because the Debian packaging passes an absolute path.
# From the last line on, the name means the runtime absolute path instead; that
# second value is a plain variable and shadows the cache entry for every read
# after it, which is also what happens today when -DWEB_DIR= is passed.
string(REGEX REPLACE "^/" "" WEB_DEST "${WEB_DIR}")
set(WEB_DIR "${NETDATA_RUNTIME_PREFIX}/${WEB_DEST}")

# Host integration paths. These directories belong to the platform, not to
# Netdata: systemd, logrotate and the distro init system own them and fix their
# location. They are named here so the whole set is visible in one place, but
# they are deliberately NOT derived from NETDATA_RUNTIME_PREFIX - rebasing them
# onto the Netdata layout would install our drop-ins where nothing reads them.
#
# All of these are Linux-only - systemd, logrotate and the LSB init system are
# Linux concepts - so every rule that writes one is guarded on OS_LINUX. macOS
# and FreeBSD reach their own service managers through the Netdata-owned copies
# under SYSTEM_DEST, which system/install-service.sh selects at install time.
# A native macOS package will add its launchd destination here under its own
# guard; do not widen an existing one to cover it.
set(HOST_LOGROTATE_DEST "etc/logrotate.d")
set(HOST_INITD_DEST "etc/init.d")
set(HOST_DEFAULT_DEST "etc/default")
set(HOST_SYSUSERS_DEST "usr/lib/sysusers.d")
set(HOST_TMPFILES_DEST "usr/lib/tmpfiles.d")
set(HOST_JOURNALD_CONF_DEST "usr/lib/systemd/journald@netdata.conf.d")
set(HOST_SYSTEMD_PRESET_DEST "usr/lib/systemd/system-preset")

# RPM distros ship systemd units in %{_unitdir} (/usr/lib/systemd/system); the
# DEB layout (and the historical default) uses /lib/systemd/system.
if(NETDATA_PACKAGING_FORMAT STREQUAL "rpm")
        set(HOST_SYSTEMD_UNIT_DEST "usr/lib/systemd/system")
else()
        set(HOST_SYSTEMD_UNIT_DEST "lib/systemd/system")
endif()

# Guards every rule that writes a HOST_* destination bar the one named below:
# this platform owns those directories, and this artifact is a native package
# that will claim them in its own manifest. Naming the intersection once keeps
# the two questions - whose path is it, and are we packaging - out of the five
# conditions that would otherwise carry both, and makes a new packaging target
# a change here rather than at every site.
#
# The logrotate host copy is the exception, guarded on OS_LINUX alone. It has
# no packaging condition because a non-package Linux build has always staged
# it, and adding one would change the static installer's payload.
if(OS_LINUX AND BUILD_FOR_PACKAGING)
  set(NETDATA_STAGE_HOST_FILES TRUE)
else()
  set(NETDATA_STAGE_HOST_FILES FALSE)
endif()

# DEB policy requires a copyright file in every binary package, under that
# package's own documentation directory.
set(PKG_DOC_DEST "usr/share/doc")
