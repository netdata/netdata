# SPDX-License-Identifier: GPL-3.0-or-later
#
# The order of this file, and why it is that order. Most of it is free to move;
# the constraints below are the ones that are not, and each has been paid for.
#
#   1. version, module path, build type
#   2. platform detection and compiler flags. Every directory-property mutation
#      that applies to the whole tree happens here: the mold link option,
#      NetdataPlatform's -D_GNU_SOURCE and include paths, NetdataCompilerFlags,
#      and HAVE_OSLOG. One more sits far below, in phase 7, on purpose.
#   3. platform capability probes (NetdataPlatformChecks). Free to move within
#      this file - it creates no target and touches no directory property - but
#      kept here, ahead of every subproject, so that our HAVE_* macros are
#      answered by our own probes. check_* results share one cache namespace with
#      every project configured as a subdirectory and the first writer wins, so a
#      probe that runs after a vendor's silently inherits the vendor's answer to
#      a question the vendor may have asked differently.
#   4. install layout, build options, host integration paths
#   5. toolchains and code generation: Go, Rust/corrosion
#   6. dependency discovery, then the subprojects we bundle. Every external
#      library is looked up once, in NetdataDependencies, with no REQUIRED and no
#      guard; whether a missing one is fatal is decided next to the target that
#      needs it, in phase 8.
#   7. the stack-trace flag pair, wedged between three netdata_bundle_* calls.
#      Load-bearing and explained in full at that point. Do not tidy it.
#   8. source inventories, then targets: libraries, the daemon, the plugins
#   9. tests, then packaging
#  10. config.h and the other generated files, on the LAST line. Not because of
#      the compile-option snapshot it takes - that only has to follow phase 7 -
#      but because its inputs resolve during phase 8: HAVE_CAPABILITY,
#      HAVE_MONGOC, HAVE_LIBUNWIND, HAVE_LIBMNL, HAVE_SYSTEMD and the HAVE_SD_*
#      set, ENABLE_LZ4/ZSTD/BROTLI, and ENABLE_PROMETHEUS_REMOTE_WRITE, which
#      binds latest of all. Moving it earlier fails SILENTLY: about fifteen
#      #cmakedefine lines would emit as undefs, the build would succeed, and
#      mongoc, brotli, zstd and systemd support would be quietly dropped.

cmake_minimum_required(VERSION 3.16.0...3.30)

list(APPEND CMAKE_MODULE_PATH "${CMAKE_SOURCE_DIR}/packaging/cmake/Modules")

include(NetdataVersion)
netdata_version()

if(NOT CMAKE_BUILD_TYPE)
  set(ENV{CMAKE_BUILD_TYPE} "RelWithDebInfo")
  set(CMAKE_BUILD_TYPE "RelWithDebInfo")
endif()

project(netdata
        VERSION "${NETDATA_VERSION_MAJOR}.${NETDATA_VERSION_MINOR}.${NETDATA_VERSION_PATCH}.${NETDATA_VERSION_TWEAK}"
        HOMEPAGE_URL "https://www.netdata.cloud"
        LANGUAGES C CXX)
include(CMakeDependentOption)
include(NetdataUtil)
netdata_fixup_system_processor()

option(STATIC_BUILD "Use static linking instead of dynamic linking for the build." FALSE)
mark_as_advanced(STATIC_BUILD)
set(BUILD_SHARED_LIBS "${STATIC_BUILD}")

if(STATIC_BUILD)
  set(CMAKE_FIND_LIBRARY_PREFIXES "${CMAKE_STATIC_LIBRARY_PREFIX}")
  set(CMAKE_FIND_LIBRARY_SUFFIXES "${CMAKE_STATIC_LIBRARY_SUFFIX}")
endif()

find_package(PkgConfig REQUIRED)

if(STATIC_BUILD)
    list(APPEND PKG_CONFIG_EXECUTABLE "--static")
endif()

set(CMAKE_INSTALL_MESSAGE LAZY)
set(CMAKE_INSTALL_DEFAULT_COMPONENT_NAME "netdata")

if("c_std_17" IN_LIST CMAKE_C_KNOWN_FEATURES)
    set(CMAKE_C_STANDARD 17)
else()
    set(CMAKE_C_STANDARD 11)
endif()

set(CMAKE_CXX_STANDARD 17)

option(USE_CXX_11 "Use C++11 instead of C++17 (should only be used on legacy systems that cannot support C++17, may disable some features)" False)
mark_as_advanced(USE_CXX_11)

if(USE_CXX_11)
    set(CMAKE_CXX_STANDARD 11)
endif()

set(CMAKE_C_STANDARD_REQUIRED On)
set(CMAKE_CXX_STANDARD_REQUIRED On)

set(SIZEOF_VOID_P ${CMAKE_SIZEOF_VOID_P})

set(CMAKE_EXPORT_COMPILE_COMMANDS On)

# Check for the mold linker and try to use it if available
option(USE_MOLD "If the MOLD linker is available on the system, use it instead of the default linker." TRUE)

if(USE_MOLD)
        message(CHECK_START "Searching for MOLD linker")
        find_program(MOLD_LINKER NAMES ld.mold mold)

        if(MOLD_LINKER)
                execute_process(COMMAND ${MOLD_LINKER} --version
                                RESULT_VARIABLE MOLD_VERSION_RESULT
                                OUTPUT_VARIABLE MOLD_VERSION)

                if(NOT MOLD_VERSION_RESULT)
                        string(REPLACE "\n" "" MOLD_VERSION "${MOLD_VERSION}")
                        message(CHECK_PASS "found (version: ${MOLD_VERSION})")
                        message(STATUS "Using mold instead of the system default for linking.")
                        add_link_options("-fuse-ld=mold")
                else()
                        message(CHECK_FAIL "failed")
                endif()
        else()
                message(CHECK_FAIL "failed")
        endif()
endif()

set(BINDIR usr/sbin)
set(NETDATA_RUNTIME_PREFIX "${CMAKE_INSTALL_PREFIX}")
# This can be changed depending on the platform with the include below
include(NetdataPlatform)
include(NetdataCompilerFlags)

if(OS_MACOS)
  enable_language(OBJC)
  if(OSLOG AND FOUNDATION)
    add_compile_definitions(HAVE_OSLOG)
  endif()
endif()

#
# Platform capability probes. Placed here, immediately after the last of the four
# directory-property mutations above (the mold link option, NetdataPlatform's
# -D_GNU_SOURCE and include paths, NetdataCompilerFlags, and HAVE_OSLOG), and ahead of
# every dependency lookup and every bundled subproject.
#
# Ahead of the subprojects matters: check_* results are cached in a namespace shared
# with every project configured as a subdirectory, and the first writer wins. Running
# our probes first means our HAVE_* macros are answered by our own probes rather than
# inherited from whichever vendor happened to configure earlier.
#
include(NetdataPlatformChecks)

set(CONFIG_H_DIR ${CMAKE_BINARY_DIR})

if(OS_WINDOWS)
  set(NETDATA_WINDOWS_PATH_PREFIX "C:\\Program Files\\Netdata" CACHE STRING
      "Native Windows install prefix used to derive runtime paths")
  string(REGEX REPLACE "[/\\\\]+$" "" NETDATA_WINDOWS_PATH_PREFIX "${NETDATA_WINDOWS_PATH_PREFIX}")
  string(REPLACE "\\" "\\\\" NETDATA_WINDOWS_PATH_PREFIX_ESCAPED "${NETDATA_WINDOWS_PATH_PREFIX}")

  if(NOT BUILD_FOR_PACKAGING)
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
if(NOT DEFINED NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR)
  set(NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR "")
endif()

# A non-default value is only used when building Debian packages (/var/lib/netdata/www)
if(NOT DEFINED WEB_DIR)
  set(WEB_DIR "usr/share/netdata/web")
else()
  string(REGEX REPLACE "^/" "" WEB_DIR "${WEB_DIR}")
endif()
set(WEB_DEST "${WEB_DIR}")
set(WEB_DIR "${NETDATA_RUNTIME_PREFIX}/${WEB_DEST}")

#
# Build options, the package-format validation they feed, and the per-format component remap.
#

include(NetdataOptions)

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

# The one predicate every rule that writes a HOST_* destination is guarded on:
# this platform owns those directories, and this artifact is a native package
# that will claim them in its own manifest. Naming the intersection once keeps
# the two questions - whose path is it, and are we packaging - out of eleven
# separate conditions, and makes a new packaging target a change here rather
# than at every site.
#
# The logrotate host copy is the deliberate exception; it is guarded on OS_LINUX
# alone because a non-package Linux build has always staged it.
if(OS_LINUX AND BUILD_FOR_PACKAGING)
  set(NETDATA_STAGE_HOST_FILES TRUE)
else()
  set(NETDATA_STAGE_HOST_FILES FALSE)
endif()

# DEB policy requires a copyright file in every binary package, under that
# package's own documentation directory.
set(PKG_DOC_DEST "usr/share/doc")

cmake_dependent_option(ENABLE_LIBBACKTRACE "Use libbacktrace for stack traces in log output" True "OS_LINUX OR OS_WINDOWS" False)
mark_as_advanced(ENABLE_LIBBACKTRACE)
cmake_dependent_option(ENABLE_LIBUNWIND "Use libunwind for stack traces in log output" False "NOT ENABLE_LIBBACKTRACE" False)
mark_as_advanced(ENABLE_LIBUNWIND)

cmake_dependent_option(FORCE_LEGACY_LIBBPF "Force usage of libbpf 0.0.9 instead of the latest version." False "ENABLE_PLUGIN_EBPF" False)
mark_as_advanced(FORCE_LEGACY_LIBBPF)

cmake_dependent_option(ENABLE_NETDATA_JOURNAL_FILE_READER "Enable netdata's journal file reader implementation" False "ENABLE_PLUGIN_SYSTEMD_JOURNAL" False)

# Setup Rust/Corrosion for plugins that need it
if(ENABLE_NETDATA_JOURNAL_FILE_READER OR ENABLE_PLUGIN_OTEL OR ENABLE_PLUGIN_NETFLOW)
    include(FetchContent)
    FetchContent_Declare(
        Corrosion
        GIT_REPOSITORY https://github.com/netdata/corrosion.git
        GIT_TAG f3b91559efca32c6b54837866ef35ba98ff5b2ca # stable/v0.5
    )
    FetchContent_MakeAvailable(Corrosion)

    # Corrosion places cargo build artifacts under ${CMAKE_BINARY_DIR}/cargo/
    # (see Corrosion.cmake cargo_target_dir). Register it for cleanup so that
    # `ninja clean` removes it.  If a future Corrosion version changes this
    # path, this line must be updated to match.
    set_directory_properties(PROPERTIES ADDITIONAL_CLEAN_FILES "${CMAKE_BINARY_DIR}/cargo")

    if(ENABLE_NETDATA_JOURNAL_FILE_READER)
        corrosion_import_crate(MANIFEST_PATH src/crates/jf/Cargo.toml
                               CRATES journal_reader_ffi)
    endif()

    # Import crates from the main cargo workspace
    set(_WORKSPACE_CRATES "")
    if(ENABLE_PLUGIN_OTEL)
        list(APPEND _WORKSPACE_CRATES otel-plugin)
    endif()
    if(ENABLE_PLUGIN_NETFLOW)
        list(APPEND _WORKSPACE_CRATES netflow-plugin)
    endif()
    if(_WORKSPACE_CRATES)
        corrosion_import_crate(MANIFEST_PATH src/crates/Cargo.toml
                               CRATES ${_WORKSPACE_CRATES})
    endif()

    if(ENABLE_PLUGIN_OTEL)
      if(STATIC_BUILD)
        corrosion_add_target_rustflags(otel-plugin --cfg=tracing_unstable "-C" "target-feature=+crt-static")
      else()
        corrosion_add_target_rustflags(otel-plugin --cfg=tracing_unstable)
      endif()

      # The workspace release profile propagates fat LTO, single-codegen-unit,
      # and debuginfo settings through the whole dependency graph. On 32-bit
      # targets that makes rustc-LLVM exhaust the ~3 GB process address space
      # during codegen ("rustc-LLVM ERROR: out of memory"). Override Cargo's
      # release profile so the otel-plugin build stays within budget.
      if(CMAKE_SIZEOF_VOID_P EQUAL 4 AND CMAKE_SYSTEM_NAME STREQUAL "Linux")
        # We depend (transitively) on the `io-uring` crate which doesn't provide
        # prebuilt bindings for 32-bit arches. Considering this is a compile-time
        # and not a runtime dependency (because we don't use foyer's io-uring
        # engine), we can simply skip the check.
        corrosion_add_target_rustflags(otel-plugin --cfg=io_uring_skip_arch_check)
        corrosion_set_env_vars(
                otel-plugin
                "CARGO_PROFILE_RELEASE_DEBUG=0"
                "CARGO_PROFILE_RELEASE_LTO=off"
                "CARGO_PROFILE_RELEASE_CODEGEN_UNITS=8")
      endif()
    endif()

    if(ENABLE_PLUGIN_NETFLOW)
      set(NETFLOW_PLUGIN_RUSTFLAGS "")
      set(NETFLOW_PLUGIN_ENV_VARS
              "NETDATA_BUILD_CACHE_DIR=${CACHE_DIR}"
              "NETDATA_BUILD_LIB_DIR=${VARLIB_DIR}"
              "NETDATA_BUILD_STOCK_DATA_DIR=${STOCK_DATA_DIR}")

      # We depend (transitively) on the `io-uring` crate which doesn't provide
      # prebuilt bindings for 32-bit arches. Considering this is a compile-time
      # and not a runtime dependency (because we don't use foyer's io-uring
      # engine), we can simply skip the check.
      if(CMAKE_SIZEOF_VOID_P EQUAL 4 AND CMAKE_SYSTEM_NAME STREQUAL "Linux")
        list(APPEND NETFLOW_PLUGIN_RUSTFLAGS --cfg=io_uring_skip_arch_check)
        # The workspace release profile still propagates fat LTO, single-codegen-unit,
        # and debuginfo settings through Rust dependencies on 32-bit builds. Override
        # Cargo's release profile too so the entire dependency graph stays linkable.
        list(APPEND NETFLOW_PLUGIN_ENV_VARS
                "CARGO_PROFILE_RELEASE_DEBUG=0"
                "CARGO_PROFILE_RELEASE_LTO=off"
                "CARGO_PROFILE_RELEASE_CODEGEN_UNITS=8")
      endif()

      if(STATIC_BUILD)
        list(APPEND NETFLOW_PLUGIN_RUSTFLAGS "-C" "target-feature=+crt-static")
      endif()

      if(NETFLOW_PLUGIN_RUSTFLAGS)
        corrosion_add_target_rustflags(netflow-plugin ${NETFLOW_PLUGIN_RUSTFLAGS})
      endif()

      corrosion_set_env_vars(netflow-plugin ${NETFLOW_PLUGIN_ENV_VARS})
    endif()
endif()

set(ENABLE_PLUGIN_EBPF_GO OFF)
if(ENABLE_PLUGIN_GO OR ENABLE_PLUGIN_EBPF OR ENABLE_PLUGIN_IBM OR ENABLE_PLUGIN_SCRIPTS OR ENABLE_PLUGIN_NETFLOW OR ENABLE_CGROUP_NAME)
    include(NetdataGoTools)

    find_min_go_version("${CMAKE_SOURCE_DIR}/src/go")

    find_package(Go "${MIN_GO_VERSION}" QUIET)
    if(NOT GO_FOUND)
        if(ENABLE_PLUGIN_GO OR ENABLE_PLUGIN_IBM OR ENABLE_PLUGIN_SCRIPTS OR ENABLE_PLUGIN_NETFLOW)
            find_package(Go "${MIN_GO_VERSION}" REQUIRED)
        endif()
    endif()

    if(GO_FOUND)
        set(ENABLE_PLUGIN_EBPF_GO ON)
    endif()
endif()

if(OS_MACOS OR ENABLE_PLUGIN_GO OR ENABLE_PLUGIN_SCRIPTS)
    set(NEED_NDSUDO TRUE)
else()
    set(NEED_NDSUDO FALSE)
endif()

if(ENABLE_WEBRTC)
        include(FetchContent)
        include(NetdataFetchContentExtra)

        # ignore debhelper
        set(FETCHCONTENT_FULLY_DISCONNECTED Off)

        set(PREFER_SYSTEM_LIB True)
        set(NO_MEDIA True)
        set(NO_WEBSOCKET True)

        set(HAVE_LIBDATACHANNEL True)

        FetchContent_Declare(libdatachannel
            GIT_REPOSITORY https://github.com/paullouisageneau/libdatachannel.git
            GIT_TAG v0.20.1
            CMAKE_ARGS ${NETDATA_PROPAGATE_TOOLCHAIN_ARGS}
        )
        FetchContent_MakeAvailable(libdatachannel)
endif()

include(NetdataDependencies)

include(NetdataProtobuf)

if(ENABLE_BUNDLED_PROTOBUF)
        netdata_bundle_protobuf()
endif()

set(PKG_FILES_PATH "${CMAKE_SOURCE_DIR}/packaging/cmake/pkg-files")

include(NetdataOSRelease)

if(ENABLE_PLUGIN_EBPF)
        include(NetdataLibBPF)
        include(NetdataEBPFCORE)

        if(NOT OS_LINUX)
            message(FATAL_ERROR "The eBPF plugin is not supported on non-Linux systems")
        endif()

        netdata_bundle_libbpf()
        netdata_fetch_ebpf_co_re()
endif()

#
# Custom Modules
#

include(NetdataJSONC)
include(NetdataYAML)
include(NetdataBacktrace)
include(NetdataSQLite)
include(NetdataDlib)

if(ENABLE_LEGACY_EBPF_PROGRAMS)
        include(NetdataEBPFLegacy)
endif()

if(ENABLE_SENTRY)
        include(NetdataSentry)
endif()

#
# Checks from custom modules
#

netdata_detect_jsonc()
netdata_detect_libyaml()

if(ENABLE_LEGACY_EBPF_PROGRAMS)
        netdata_fetch_legacy_ebpf_code()
endif()

if(ENABLE_SENTRY)
        netdata_bundle_sentry()
endif()

#
# Bundled dependencies, and the stack-trace flags whose position between them is
# load-bearing. This is a braid, not a design: do not tidy it.
#
# add_compile_options sets a DIRECTORY property, and add_subdirectory - so also
# FetchContent_MakeAvailable - passes a directory property only to targets created
# AFTER the mutation. So this block partitions every bundled target in the tree by
# which side of it the target is created on:
#
#   uninstrumented, created before it: dlib (below), and Corrosion, libdatachannel,
#                                      protobuf+absl, libbpf, json-c, libyaml and
#                                      sentry, all bundled earlier in this file
#   instrumented, created after it:    libbacktrace, sqlite3, and every netdata target
#
# Measured, not argued: dlib's 49 translation units carry none of the three flags and
# sqlite3's 12 carry all three, in the same configure.
#
# Nothing explains why sqlite3 should be instrumented and dlib should not - it falls
# out of one netdata_bundle_* call sitting nine lines below a statement and another
# eighty-four lines above it. Moving the flags, or gathering the netdata_bundle_*
# calls, changes shipped object code whichever side the new position lands on, so it
# is a product decision about whether vendored code carries frame pointers, not a
# refactor. Until that decision is taken, the order below is the contract:
#
#   dlib  <  the flags  <  libbacktrace  <  sqlite3
#
# The flags also feed CONFIGURE_COMMAND, the accumulated-compile-options string
# that NetdataSystemFiles.cmake snapshots into config.h. That string reaches users
# compiled into the binary, through netdata -W buildinfo and the info API - NOT
# through build-info-cmake-cache.gz, which is gzip of CMakeCache.txt and holds no
# such entry. So it is a user-visible build-info field, which raises the bar on
# changing it.
#
if(ENABLE_ML)
  netdata_bundle_dlib()
endif()

if(CMAKE_C_COMPILER_ID MATCHES "GNU|Clang")
    # -fno-omit-frame-pointer = add frame pointers to all functions
    # -funwind-tables = generate unwind tables for all functions
    # -fasynchronous-unwind-tables = the unwind table generated is precise at instruction boundary, instead of function boundary
    add_compile_options(-fno-omit-frame-pointer -funwind-tables -fasynchronous-unwind-tables)
    add_link_options(-rdynamic)
    message(STATUS "Added compiler and linker flags for better stack trace support")
endif()

if(ENABLE_LIBBACKTRACE)
    netdata_bundle_libbacktrace()
endif()

netdata_bundle_sqlite3()

netdata_detect_protobuf()

if(OS_LINUX)
        include(NetdataDetectSystemd)
        detect_systemd()
endif()

# source file inventory for everything defined in this file; every operation on
# those lists - the conditional list(APPEND ...) calls and the NETDATA_FILES
# aggregation - stays here. A component that owns its own build definition
# keeps its inventory beside it instead: a plugin in its
# NetdataPlugin*.cmake module, libnetdata in src/libnetdata/SourceLists.cmake.
include(NetdataSourceLists)

if(ENABLE_SYSTEMD_DBUS)
        list(APPEND DAEMON_FILES ${DAEMON_SYSTEMD_WATCHER_FILES})
endif()

if(ENABLE_ML)
        set(ML_FILES
                ${ML_ENABLED_FILES}
        )

        list(APPEND ML_FILES ${ML_MEMORY_FILES})
        set_source_files_properties(${ML_FILES} PROPERTIES COMPILE_OPTIONS "-Wno-attributes")
else()
        set(ML_FILES
                ${ML_DISABLED_FILES}
        )
endif()

if(ENABLE_DBENGINE)
    list(APPEND RRD_PLUGIN_FILES
            ${RRD_DBENGINE_FILES}
    )
endif()

set(NETDATA_FILES
        ${COLLECTORS_ALL_FILES}
        ${DAEMON_FILES}
        ${API_PLUGIN_FILES}
        ${EXPORTING_ENGINE_FILES}
        ${HEALTH_PLUGIN_FILES}
        ${IDLEJITTER_PLUGIN_FILES}
        ${ML_FILES}
        ${PLUGINSD_PLUGIN_FILES}
        ${RRD_PLUGIN_FILES}
        ${REGISTRY_PLUGIN_FILES}
        ${STATSD_PLUGIN_FILES}
        ${STREAMING_PLUGIN_FILES}
        ${WEB_PLUGIN_FILES}
        ${CLAIM_PLUGIN_FILES}
        ${ACLK_ALWAYS_BUILD}
        ${PROFILE_PLUGIN_FILES}
)

if(OS_LINUX)
        list(APPEND NETDATA_FILES
                ${DAEMON_LINUX_FILES}
                ${CGROUPS_PLUGIN_FILES}
                ${DISKSPACE_PLUGIN_FILES}
                ${PROC_PLUGIN_FILES}
                ${TC_PLUGIN_FILES}
                ${TIMEX_PLUGIN_FILES}
                ${INTERNAL_COLLECTORS_FILES}
        )

        if(ENABLE_SENTRY)
            list(APPEND NETDATA_FILES
                    ${DAEMON_SENTRY_FILES})
        endif()
elseif(OS_MACOS)
        list(APPEND NETDATA_FILES
                ${DAEMON_MACOS_FILES}
                ${MACOS_PLUGIN_FILES}
                ${TIMEX_PLUGIN_FILES}
                ${INTERNAL_COLLECTORS_FILES}
        )
elseif(OS_FREEBSD)
        list(APPEND NETDATA_FILES
                ${DAEMON_FREEBSD_FILES}
                ${FREEBSD_PLUGIN_FILES}
                ${TIMEX_PLUGIN_FILES}
                ${INTERNAL_COLLECTORS_FILES}
        )
elseif(OS_WINDOWS)
        list(APPEND NETDATA_FILES
                ${DAEMON_WINDOWS_FILES}
                ${WINDOWS_PLUGIN_FILES}
                ${INTERNAL_COLLECTORS_FILES}
        )
endif()

set_source_files_properties(src/aclk/schema-wrappers/proto_2_json.cc PROPERTIES COMPILE_OPTIONS "-Wno-unused-result")

#
# libnetdata, judy and netipc
#
# Defined in src/libnetdata/CMakeLists.txt, together with their source
# inventory. add_subdirectory() rather than include() so the component owns
# its own build definition; the call sits where the definitions used to, so
# nothing changes order.
#
add_subdirectory(src/libnetdata)

# mnl
if(MNL_FOUND AND NOT OS_MACOS)
    set(HAVE_LIBMNL True)
endif()

#
# mqtt library
#
set(ENABLE_MQTTWEBSOCKETS True)
if(ENABLE_MQTTWEBSOCKETS)
        add_library(mqttwebsockets STATIC ${MQTT_WEBSOCKETS_FILES})

        target_compile_options(mqttwebsockets PUBLIC -DMQTT_WSS_CUSTOM_ALLOC
                                                     -DRBUF_CUSTOM_MALLOC
                                                     -DMQTT_WSS_CPUSTATS)

        target_include_directories(mqttwebsockets PUBLIC ${CMAKE_SOURCE_DIR}/aclk/helpers)

        target_link_libraries(mqttwebsockets PRIVATE libnetdata)

endif()

#
# proto definitions
#
# aclk-schemas moved to Buf v2 (PR netdata/aclk-schemas#57): the module root is
# now "proto/" and internal imports are relative to it (e.g. "aclk/v1/lib.proto"),
# so protoc's include root must be the proto/ directory. Generated headers are
# therefore included without the "proto/" prefix (e.g. "aclk/v1/lib.pb.h").
netdata_protoc_generate_cpp("${CMAKE_SOURCE_DIR}/src/aclk/aclk-schemas/proto"
                            "${CMAKE_BINARY_DIR}/src/aclk/aclk-schemas"
                            ACLK_PROTO_BUILT_SRCS
                            ACLK_PROTO_BUILT_HDRS
                            ${ACLK_PROTO_DEFS})

list(APPEND ACLK_FILES ${ACLK_PROTO_BUILT_SRCS}
                       ${ACLK_PROTO_BUILT_HDRS})

#
# nd-run: runs a helper program with the capabilities it needs, if any.
#

include(NetdataNdRun)

#
# build plugins
#

include(NetdataPluginDebugfs)

#
# Compiled collector plugins and helpers, one module each. Each module
# carries its own source list, executable and install rule, so a
# plugin's whole definition is in one place.
#
include(NetdataPluginApps)
include(NetdataPluginFreeipmi)
include(NetdataPluginNfacct)
include(NetdataPluginXenstat)
include(NetdataPluginPerf)
include(NetdataPluginSlabinfo)
include(NetdataPluginCups)
include(NetdataNdsudo)
include(NetdataCgroupNetwork)

#
# Test executables. None is installed, referenced elsewhere, or registered with CTest; the project defines no add_test().
#

include(NetdataTests)

include(NetdataPluginSystemdJournal)

include(NetdataPluginSystemdUnits)

include(NetdataPluginWindowsEvents)

include(NetdataPluginMacosLogs)

include(NetdataPluginEBPF)

include(NetdataPluginLocalListeners)

include(NetdataPluginNetworkViewer)

#
# exporters
#

if(ENABLE_EXPORTER_MONGODB)
        if(MONGOC_FOUND)
                SET(HAVE_MONGOC True)
        else()
                SET(ENABLE_EXPORTER_MONGODB False)
        endif()
endif()

if(ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE)
        if (NOT SNAPPY_FOUND)
                include(CheckLibraryExists)
                check_library_exists(snappy snappy_compress "" HAVE_SNAPPY_LIB)

                if(HAVE_SNAPPY_LIB)
                        set(SNAPPY_INCLUDE_DIRS "")
                        set(SNAPPY_CFLAGS_OTHER "")
                        set(SNAPPY_LIBRARIES "-lsnappy")
                else()
                        message(FATAL_ERROR "Could not find snappy libraries with pkg-config or internal cmake checks.")
                endif()
        endif()

        netdata_protoc_generate_cpp("${CMAKE_SOURCE_DIR}/src/exporting/prometheus/remote_write"
                                    "${CMAKE_BINARY_DIR}/src/exporting/prometheus/remote_write"
                                    PROMETHEUS_REMOTE_WRITE_BUILT_SRCS
                                    PROMETHEUS_REMOTE_WRITE_BUILT_HDRS
                                    "src/exporting/prometheus/remote_write/remote_write.proto")

        list(APPEND PROMETHEUS_REMOTE_WRITE_EXPORTING_FILES
                    ${PROMETHEUS_REMOTE_WRITE_BUILT_SRCS}
                    ${PROMETHEUS_REMOTE_WRITE_BUILT_HDRS})

        set(ENABLE_PROMETHEUS_REMOTE_WRITE True)
endif()

#
# build netdata (only Linux ATM)
#

if(OS_WINDOWS)
        configure_file(packaging/windows/resources/netdata_claim.manifest.in ${CMAKE_SOURCE_DIR}/packaging/windows/resources/netdata_claim.manifest @ONLY)

        configure_file(packaging/windows/resources/netdatacli.manifest.in ${CMAKE_SOURCE_DIR}/packaging/windows/resources/netdatacli.manifest @ONLY)

        configure_file(packaging/windows/resources/netdata.manifest.in ${CMAKE_SOURCE_DIR}/packaging/windows/resources/netdata.manifest @ONLY)

        configure_file(packaging/windows/netdata.wxs.in netdata.wxs @ONLY)
        configure_file(packaging/windows/NetdataWhite.ico NetdataWhite.ico COPYONLY)
        configure_file(packaging/windows/eula.rtf eula.rtf COPYONLY)
        configure_file(packaging/windows/Top.bmp Top.bmp COPYONLY)
        configure_file(packaging/windows/BackGround.bmp BackGround.bmp COPYONLY)
        configure_file(src/collectors/windows.plugin/driver/netdata_driver.inf netdata_driver.inf COPYONLY)
endif()

add_executable(netdata
        ${NETDATA_FILES}
        "${ACLK_FILES}"
        "$<$<BOOL:${ENABLE_EXPORTER_MONGODB}>:${MONGODB_EXPORTING_FILES}>"
        "$<$<BOOL:${ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE}>:${PROMETHEUS_REMOTE_WRITE_EXPORTING_FILES}>"
        "$<$<BOOL:${OS_WINDOWS}>:${NETDATA_RES_FILES}>"
)

if(ENABLE_ML)
  netdata_add_dlib_to_target(netdata)
endif()

if(OS_WINDOWS)
        add_executable(NetdataClaim ${CLAIM_WINDOWS_FILES} ${NETDATA_CLAIM_RES_FILES})
        target_link_libraries(NetdataClaim shell32 gdi32 msftedit)
        target_compile_options(NetdataClaim PUBLIC -mwindows)

        set(NETDATA_DRIVER_FILE "${CMAKE_BINARY_DIR}/netdata_driver.sys")
        set(NETDATA_DRIVER_FILE_INF "${CMAKE_BINARY_DIR}/netdata_driver.inf")

        add_library(netdata_driver SHARED ${WINDOWS_DRIVER_FILES})
        set_target_properties(netdata_driver PROPERTIES LIBRARY_OUTPUT_NAME "netdata_driver")
        set_target_properties(netdata_driver PROPERTIES PREFIX "")
        set_target_properties(netdata_driver PROPERTIES SUFFIX ".sys")
        target_include_directories(netdata_driver PRIVATE BEFORE "/mingw64/include/ddk" "${CMAKE_SOURCE_DIR}/src/collectors/windows.plugin" "${CMAKE_SOURCE_DIR}/src/collectors/windows.plugin/driver")
        target_compile_options(netdata_driver PRIVATE
                -Wall
                -Wextra
                -Werror
                -ffreestanding
                -fno-exceptions
                -fno-stack-protector
                -fshort-wchar
                -mno-red-zone
        )
        target_link_options(netdata_driver PRIVATE
                -Wl,--entry,DriverEntry@8
                -nostdlib
                -Wl,--subsystem,native
                -Wl,--image-base,0x10000
        )
        target_link_libraries(netdata_driver kernel32 ntoskrnl)

        install(FILES "${NETDATA_DRIVER_FILE}"
                COMPONENT netdata_driver
                DESTINATION "${BINDIR}")

        install(FILES "${NETDATA_DRIVER_FILE_INF}"
                COMPONENT netdata_driver_inf
                DESTINATION "${BINDIR}")
endif()

target_compile_options(netdata PRIVATE
        "$<$<BOOL:${ENABLE_EXPORTER_MONGODB}>:${MONGOC_CFLAGS_OTHER}>"
        "$<$<BOOL:${ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE}>:${SNAPPY_CFLAGS_OTHER}>"
)

target_include_directories(netdata PRIVATE
        "${CMAKE_BINARY_DIR}/src/aclk/aclk-schemas"
        "$<$<BOOL:${ENABLE_EXPORTER_MONGODB}>:${MONGOC_INCLUDE_DIRS}>"
        "$<$<BOOL:${ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE}>:${SNAPPY_INCLUDE_DIRS}>"
)

target_link_libraries(netdata PRIVATE
        m
        libnetdata
        "$<$<BOOL:${HAVE_LIBRT}>:rt>"
        "$<$<BOOL:${ENABLE_MQTTWEBSOCKETS}>:mqttwebsockets>"
        "$<$<BOOL:${ENABLE_EXPORTER_MONGODB}>:${MONGOC_LIBRARIES}>"
        "$<$<BOOL:${ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE}>:${SNAPPY_LIBRARIES}>"
        "$<$<BOOL:${OS_MACOS}>:${IOKIT};${FOUNDATION}>"
        "$<$<BOOL:${ENABLE_SENTRY}>:sentry>"
        "$<$<BOOL:${ENABLE_WEBRTC}>:LibDataChannel::LibDataChannelStatic>"
        PkgConfig::CURL
        "$<$<BOOL:${OS_WINDOWS}>:odbc32;setupapi>"
)

netdata_add_protobuf(netdata)

#
# systemd-cat-native: sends log entries to a local or remote systemd journal.
#

include(NetdataSystemdCatNative)

#
# log2journal: converts structured text logs into systemd Journal Export Format.
#

include(NetdataLog2Journal)

#
# netdatacli: the command-line client for the running agent.
#

include(NetdataCli)

#
# Build go.d.plugin
#

if(ENABLE_PLUGIN_GO)
    if (OS_WINDOWS)
        set(GO_PLUGIN_BIN go.d.plugin.exe)
    else()
        set(GO_PLUGIN_BIN go.d.plugin)
    endif()

    add_go_target(go-plugin ${GO_PLUGIN_BIN} src/go cmd/godplugin)

    install(PROGRAMS ${CMAKE_BINARY_DIR}/${GO_PLUGIN_BIN}
            COMPONENT plugin-go
            DESTINATION ${PLUGINS_DEST})

    if (OS_WINDOWS)
        set(SNMP_TRAP_PROFILE_GEN_BIN snmp-trap-profile-gen.exe)
    else()
        set(SNMP_TRAP_PROFILE_GEN_BIN snmp-trap-profile-gen)
    endif()

    add_go_target(snmp_trap_profile_gen ${SNMP_TRAP_PROFILE_GEN_BIN} src/go cmd/snmptrapprofilegen)

    install(PROGRAMS ${CMAKE_BINARY_DIR}/${SNMP_TRAP_PROFILE_GEN_BIN}
            COMPONENT plugin-go
            DESTINATION ${PLUGINS_DEST})

    # Build and install nd-mcp (stdio-golang bridge) exactly like go.d.plugin
    if(ENABLE_ND_MCP)
        if (OS_WINDOWS)
            set(ND_MCP_NAME nd-mcp.exe)
        else()
            set(ND_MCP_NAME nd-mcp)
        endif()

        add_go_target(nd-mcp-target ${ND_MCP_NAME} src/web/mcp/bridges/stdio-golang .)
        install(PROGRAMS
                ${CMAKE_BINARY_DIR}/${ND_MCP_NAME}
                COMPONENT ${NETDATA_ND_MCP_COMPONENT}
                DESTINATION "${BINDIR}")
    endif()
endif()

#
# Build scripts.d.plugin
#

if(ENABLE_PLUGIN_SCRIPTS)
    if (OS_WINDOWS)
        set(SCRIPTS_PLUGIN_BIN scripts.d.plugin.exe)
    else()
        set(SCRIPTS_PLUGIN_BIN scripts.d.plugin)
    endif()

    add_go_target(scripts-plugin ${SCRIPTS_PLUGIN_BIN} src/go cmd/scriptsdplugin)

    install(PROGRAMS ${CMAKE_BINARY_DIR}/${SCRIPTS_PLUGIN_BIN}
            COMPONENT plugin-scripts
            DESTINATION ${PLUGINS_DEST})

    install(FILES src/go/plugin/scripts.d/config/scripts.d.conf
            COMPONENT plugin-scripts
            DESTINATION ${LIBCONFIG_DEST})

    install(DIRECTORY src/go/plugin/scripts.d/config/scripts.d
            COMPONENT plugin-scripts
            DESTINATION ${LIBCONFIG_DEST}
            FILES_MATCHING PATTERN "*.conf")
endif()

#
# Build ibm.d.plugin
#

if(ENABLE_PLUGIN_IBM)
    include(NetdataIBMPlugin)
    add_ibm_plugin_target()

    install(PROGRAMS ${CMAKE_BINARY_DIR}/ibm.d.plugin
            COMPONENT plugin-ibm
            DESTINATION ${PLUGINS_DEST})

    # Install IBM plugin configuration files
    install(FILES src/go/plugin/ibm.d/config/ibm.d.conf
            COMPONENT plugin-ibm
            DESTINATION ${LIBCONFIG_DEST})

    install(FILES
            src/go/plugin/ibm.d/config/ibm.d/as400.conf
            src/go/plugin/ibm.d/config/ibm.d/db2.conf
            src/go/plugin/ibm.d/config/ibm.d/websphere_jmx.conf
            src/go/plugin/ibm.d/config/ibm.d/websphere_mp.conf
            src/go/plugin/ibm.d/config/ibm.d/websphere_pmi.conf
            COMPONENT plugin-ibm
            DESTINATION ${LIBCONFIG_DEST}/ibm.d
    )

    install_ibm_runtime(plugin-ibm-libs)
endif()

#
# otel.plugin
#
# Handle OTel Collector plugin
#
if(ENABLE_PLUGIN_OTEL)
    corrosion_install(TARGETS otel-plugin
                      PERMISSIONS OWNER_READ OWNER_WRITE OWNER_EXECUTE GROUP_READ GROUP_EXECUTE
                      RUNTIME DESTINATION ${PLUGINS_DEST}
                      COMPONENT plugin-otel)

    install(FILES src/crates/otel-ingestor/configs/otel.d/v1/metrics/hostmetrics-receiver.yaml
            COMPONENT ${NETDATA_OTEL_CONF_COMPONENT}
            DESTINATION ${LIBCONFIG_DEST}/otel.d/v1/metrics)

    install(DIRECTORY COMPONENT plugin-otel DESTINATION ${CONFIG_DEST}/otel.d/v1/metrics)
endif()


#
# netflow-plugin
#
# Handle NetFlow/IPFIX/sFlow flow analysis plugin
if(ENABLE_PLUGIN_NETFLOW)
    corrosion_install(TARGETS netflow-plugin
                      PERMISSIONS OWNER_READ OWNER_WRITE OWNER_EXECUTE GROUP_READ GROUP_EXECUTE
                      RUNTIME DESTINATION ${PLUGINS_DEST}
                      COMPONENT plugin-netflow)

    install(FILES src/crates/netflow-plugin/configs/netflow.yaml
            COMPONENT plugin-netflow
            DESTINATION ${LIBCONFIG_DEST})

    if (OS_WINDOWS)
        set(TOPOLOGY_IP_INTEL_DOWNLOADER_BIN topology-ip-intel-downloader.exe)
    else()
        set(TOPOLOGY_IP_INTEL_DOWNLOADER_BIN topology-ip-intel-downloader)
    endif()

    if(CMAKE_SIZEOF_VOID_P LESS 8)
        message(STATUS
                "plugin-netflow: skipping topology-ip-intel-downloader binary on 32-bit targets; "
                "stock topology-ip-intel payload installation remains enabled")
    else()
        add_go_target(topology-ip-intel-downloader-target ${TOPOLOGY_IP_INTEL_DOWNLOADER_BIN} src/go tools/topology-ip-intel-downloader)
        install(PROGRAMS
                ${CMAKE_BINARY_DIR}/${TOPOLOGY_IP_INTEL_DOWNLOADER_BIN}
                COMPONENT plugin-netflow
                DESTINATION "${BINDIR}")
        install(CODE "file(REMOVE \"\$ENV{DESTDIR}\${CMAKE_INSTALL_PREFIX}/usr/libexec/netdata/plugins.d/topology-ip-intel-downloader\")"
                COMPONENT plugin-netflow)
        install(CODE "file(REMOVE \"\$ENV{DESTDIR}\${CMAKE_INSTALL_PREFIX}/usr/libexec/netdata/plugins.d/topology-ip-intel-downloader.plugin\")"
                COMPONENT plugin-netflow)
        install(CODE "file(REMOVE \"\$ENV{DESTDIR}\${CMAKE_INSTALL_PREFIX}/usr/libexec/netdata/plugins.d/topology-ip-intel-downloader.exe\")"
                COMPONENT plugin-netflow)
        install(CODE "file(REMOVE \"\$ENV{DESTDIR}\${CMAKE_INSTALL_PREFIX}/usr/libexec/netdata/plugins.d/topology-ip-intel-downloader.plugin.exe\")"
                COMPONENT plugin-netflow)
    endif()

    install(FILES src/go/tools/topology-ip-intel-downloader/configs/topology-ip-intel.yaml
            COMPONENT plugin-netflow
            DESTINATION ${LIBCONFIG_DEST})

    if(NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR)
        set(TOPOLOGY_IP_INTEL_STOCK_PAYLOAD
                "${NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR}/README.md"
                "${NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR}/topology-ip-asn.mmdb"
                "${NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR}/topology-ip-geo.mmdb"
                "${NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR}/topology-ip-intel.json")
        foreach(stock_file IN LISTS TOPOLOGY_IP_INTEL_STOCK_PAYLOAD)
            if(NOT EXISTS "${stock_file}")
                message(FATAL_ERROR
                        "Missing topology IP intelligence stock payload file: ${stock_file}")
            endif()
        endforeach()
        install(FILES
                ${TOPOLOGY_IP_INTEL_STOCK_PAYLOAD}
                COMPONENT plugin-netflow
                DESTINATION ${STOCK_DATA_DEST}/topology-ip-intel)
    else()
        message(STATUS
                "plugin-netflow: no staged topology IP intelligence stock payload provided; "
                "skipping stock topology-ip-intel install")
    endif()
endif()

#
# Generated config header, system files, and packaging.
#
# Must stay at this position: the included file opens by snapshotting the
# compile and link options accumulated up to here.
#

include(NetdataSystemFiles)
