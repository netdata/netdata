# SPDX-License-Identifier: GPL-3.0-or-later
#
# The order of this file, and why it is that order. Most of it is free to move;
# the constraints below are the ones that are not, and each has been paid for.
#
#   1. version, module path, build type
#   2. platform detection, then every build option, then compiler flags. Detection
#      and the options module come first so that every option is declared before
#      anything reads one; that is a constraint, not a preference. Every
#      directory-property mutation that applies to the whole tree also happens in
#      this phase: NetdataPlatform's -D_GNU_SOURCE and include paths, the mold link
#      option, NetdataCompilerFlags, and HAVE_OSLOG. One more sits far below, in
#      phase 7, on purpose.
#   3. platform capability probes (NetdataPlatformChecks). Free to move within
#      this file - it creates no target and touches no directory property - but
#      kept here, ahead of every subproject, so that our HAVE_* macros are
#      answered by our own probes. check_* results share one cache namespace with
#      every project configured as a subdirectory and the first writer wins, so a
#      probe that runs after a vendor's silently inherits the vendor's answer to
#      a question the vendor may have asked differently.
#   4. install layout and host integration paths
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

# Platform detection and then every build option, before anything reads either.
#
# BINDIR is the default NetdataPlatform overrides on Windows, and NETDATA_RUNTIME_PREFIX
# is the default the Windows block further down derives from, so both are set first.
# NetdataPlatform reads no build option; NetdataOptions reads nothing but platform facts
# and variables project() and the build-type default have already settled. That ordering
# is the whole reason both sit here rather than further down: every declaration precedes
# every read, so no consumer can depend on whether a value happened to reach the cache
# via -D.
set(BINDIR usr/sbin)
set(NETDATA_RUNTIME_PREFIX "${CMAKE_INSTALL_PREFIX}")
include(NetdataPlatform)
include(NetdataOptions)

# Bundled subprojects read this as their default library type; netdata always
# builds them static. It used to mirror STATIC_BUILD, which is the negation of
# what the name means - a static build asked its subprojects for shared
# libraries - and silently overrode the -DBUILD_SHARED_LIBS=Off the source
# installer has passed since the inversion shipped.
set(BUILD_SHARED_LIBS OFF)

if(STATIC_BUILD)
  set(CMAKE_FIND_LIBRARY_PREFIXES "${CMAKE_STATIC_LIBRARY_PREFIX}")
  set(CMAKE_FIND_LIBRARY_SUFFIXES "${CMAKE_STATIC_LIBRARY_SUFFIX}")
endif()

find_package(PkgConfig REQUIRED)

# Looked up here, not in NetdataPlatform, so that every find_library in the tree runs
# after the static-build prefix override above. OSLOG and FOUNDATION are read a few
# lines below to decide HAVE_OSLOG, which is why they cannot wait for
# NetdataDependencies like every other external library.
if(OS_MACOS)
  find_library(IOKIT IOKit)
  find_library(FOUNDATION Foundation)
  find_library(OSLOG OSLog)
endif()

if(STATIC_BUILD)
    list(APPEND PKG_CONFIG_EXECUTABLE "--static")
endif()

set(CMAKE_INSTALL_MESSAGE LAZY)
set(CMAKE_INSTALL_DEFAULT_COMPONENT_NAME "netdata")

set(SIZEOF_VOID_P ${CMAKE_SIZEOF_VOID_P})

set(CMAKE_EXPORT_COMPILE_COMMANDS On)

include(NetdataCompilerFlags)

if(OS_MACOS)
  enable_language(OBJC)
  if(OSLOG AND FOUNDATION)
    add_compile_definitions(HAVE_OSLOG)
  endif()
endif()

#
# Platform capability probes. Placed here, immediately after the last of the four
# directory-property mutations above (NetdataPlatform's -D_GNU_SOURCE and include paths,
# the mold link option, NetdataCompilerFlags, and HAVE_OSLOG), and ahead of every
# dependency lookup and every bundled subproject.
#
# Ahead of the subprojects matters: check_* results are cached in a namespace shared
# with every project configured as a subdirectory, and the first writer wins. Running
# our probes first means our HAVE_* macros are answered by our own probes rather than
# inherited from whichever vendor happened to configure earlier.
#
include(NetdataPlatformChecks)

set(CONFIG_H_DIR ${CMAKE_BINARY_DIR})

#
# The install layout: every Netdata-owned path once, as a *_DEST/*_DIR pair, plus the host-owned destinations and the staging predicate.
#

include(NetdataInstallLayout)

include(NetdataRust)

include(NetdataGo)

include(NetdataWebRTC)
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

# mnl. The probe itself is skipped on macOS, so MNL_FOUND carries the platform
# exclusion already.
if(MNL_FOUND)
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

#
# go.d.plugin, the snmp trap profile generator, and the nd-mcp bridge: every Go binary gated on ENABLE_PLUGIN_GO.
#

include(NetdataPluginGo)

#
# Build scripts.d.plugin
#

#
# scripts.d.plugin: the Go Nagios-compatibility plugin and its config tree.
#

include(NetdataPluginScripts)

#
# Build ibm.d.plugin
#

include(NetdataIBMPlugin)

#
# otel.plugin
#
# Handle OTel Collector plugin
#
#
# otel-plugin: installs the Rust OpenTelemetry plugin and its stock receiver config.
#

include(NetdataPluginOTel)


#
# netflow-plugin
#
# Handle NetFlow/IPFIX/sFlow flow analysis plugin
#
# netflow-plugin: the Rust flow-analysis plugin, its config, and the Go topology-ip-intel downloader that ships beside it.
#

include(NetdataPluginNetflow)

#
# Generated config header, system files, and packaging.
#
# Must stay at this position: the included file opens by snapshotting the
# compile and link options accumulated up to here.
#

include(NetdataSystemFiles)
