# SPDX-License-Identifier: GPL-3.0-or-later
#
# This file orchestrates. The work lives in packaging/cmake/Modules/ and in the
# components' own directories; what this file fixes is the ORDER things happen
# in. Most of it is free to move; the constraints below are the ones that are
# not, and each has been paid for.
#
#   1. version, module path, build type, project().
#   2. platform detection (NetdataPlatform), then every build option
#      (NetdataOptions), then compiler flags (NetdataCompilerFlags). Detection
#      and options come first so that every option is declared before anything
#      reads one; that is a constraint, not a preference. Every directory
#      property that applies to the whole tree is also set in this phase -
#      -D_GNU_SOURCE, the compiler and linker flags, HAVE_OSLOG - because a
#      directory property only reaches targets created after the mutation -
#      which is also why the stack-trace flag pair lives here and not lower:
#      every target, vendored included, is instrumented on purpose.
#   3. platform capability probes (NetdataPlatformChecks), ahead of every
#      bundled subproject so that our HAVE_* macros are answered by our own
#      probes: check_* results share one cache namespace with every project
#      configured as a subdirectory, and the first writer wins.
#   4. the install layout (NetdataInstallLayout): every path once, as a
#      *_DEST/*_DIR pair, plus the host-owned destinations.
#   5. toolchains and code generation: Rust/corrosion (NetdataRust), Go
#      (NetdataGo), and the optional WebRTC stack (NetdataWebRTC).
#   6. dependency discovery (NetdataDependencies), then the subprojects we
#      bundle. Every external library is looked up once, with no REQUIRED and
#      no guard; whether a missing one is fatal is decided in that module's
#      requirement half or next to the target that needs it.
#   7. targets: libnetdata and its libraries (add_subdirectory), the compiled
#      plugins and helper binaries (one module each), then the daemon
#      (NetdataDaemon) and the Windows artifacts beside it.
#   8. config.h and the other generated files (NetdataSystemFiles), on the
#      LAST line. Not because of the compile-option snapshot it takes - that
#      only has to follow phase 6 - but because its inputs resolve during
#      phase 7: HAVE_CAPABILITY, HAVE_MONGOC, HAVE_LIBUNWIND, HAVE_LIBMNL,
#      HAVE_SYSTEMD and the HAVE_SD_* set, ENABLE_LZ4/ZSTD/BROTLI, and
#      ENABLE_PROMETHEUS_REMOTE_WRITE, which binds latest of all. Moving it
#      earlier fails SILENTLY: about fifteen #cmakedefine lines would emit as
#      undefs, the build would succeed, and mongoc, brotli, zstd and systemd
#      support would be quietly dropped.

cmake_minimum_required(VERSION 3.16.0...3.30)

list(APPEND CMAKE_MODULE_PATH "${CMAKE_SOURCE_DIR}/packaging/cmake/Modules")

include(NetdataVersion)
netdata_version()

if(NOT CMAKE_BUILD_TYPE)
  set(ENV{CMAKE_BUILD_TYPE} "RelWithDebInfo")
  set(CMAKE_BUILD_TYPE "RelWithDebInfo")
endif()

# The macOS support floor (packaging/PLATFORM_SUPPORT.md). Without this every
# Mach-O silently targets the build host's OS version, so a binary built on a
# newer macOS refuses to load on the versions we claim to support. It must be
# set before project() so every toolchain and every bundled subproject
# inherits it; packaging/macos/artifact-gate.sh verifies the result on the
# produced binaries, which also catches a subproject overriding it.
if(CMAKE_HOST_APPLE AND NOT DEFINED CMAKE_OSX_DEPLOYMENT_TARGET AND NOT DEFINED ENV{MACOSX_DEPLOYMENT_TARGET})
  set(CMAKE_OSX_DEPLOYMENT_TARGET "14.0" CACHE STRING "Minimum macOS version the build targets")
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

# Stack-trace flags for EVERY target, ours and vendored alike. Placed here -
# before any bundled subproject exists - because add_compile_options sets a
# directory property that reaches only targets created after the mutation.
# The flags used to sit between the bundling calls, splitting the vendored
# code into an instrumented half and an uninstrumented half by accident of
# statement order; instrumenting everything was decided deliberately, on the
# measurement that it costs zero bytes of shipped .text on the default build
# and keeps a crash inside vendored code (sqlite above all) readable.
if(CMAKE_C_COMPILER_ID MATCHES "GNU|Clang")
    # -fno-omit-frame-pointer = add frame pointers to all functions
    # -funwind-tables = generate unwind tables for all functions
    # -fasynchronous-unwind-tables = the unwind table generated is precise at instruction boundary, instead of function boundary
    add_compile_options(-fno-omit-frame-pointer -funwind-tables -fasynchronous-unwind-tables)
    add_link_options(-rdynamic)
    message(STATUS "Added compiler and linker flags for better stack trace support")
endif()

if(OS_MACOS)
  enable_language(OBJC)
  if(OSLOG AND FOUNDATION)
    add_compile_definitions(HAVE_OSLOG)
  endif()
endif()

#
# Platform capability probes. Placed here, immediately after the last of the
# directory-property mutations above (NetdataPlatform's -D_GNU_SOURCE and include
# paths, NetdataCompilerFlags - which also carries the mold link option - and
# HAVE_OSLOG), and ahead of every dependency lookup and every bundled subproject.
#
# Ahead of the subprojects matters: check_* results are cached in a namespace shared
# with every project configured as a subdirectory, and the first writer wins. Running
# our probes first means our HAVE_* macros are answered by our own probes rather than
# inherited from whichever vendor happened to configure earlier.
#
include(NetdataPlatformChecks)

# The install layout, then the toolchains, then every external library lookup.
include(NetdataInstallLayout)
include(NetdataRust)
include(NetdataGo)
include(NetdataWebRTC)
include(NetdataDependencies)
include(NetdataProtobuf)

if(ENABLE_BUNDLED_PROTOBUF)
        netdata_bundle_protobuf()
endif()

include(NetdataOSRelease)

if(ENABLE_PLUGIN_EBPF)
        include(NetdataLibBPF)
        include(NetdataEBPFCORE)

        netdata_bundle_libbpf()
        netdata_fetch_ebpf_co_re()
endif()

# The bundled-or-system dependencies: each module owns the choice between the
# system copy and its vendored fallback.
include(NetdataJSONC)
include(NetdataYAML)
include(NetdataLZ4)
include(NetdataZSTD)
include(NetdataBrotli)
include(NetdataBacktrace)
include(NetdataSQLite)
include(NetdataDlib)

if(ENABLE_LEGACY_EBPF_PROGRAMS)
        include(NetdataEBPFLegacy)
endif()

if(ENABLE_SENTRY)
        include(NetdataSentry)
endif()

netdata_detect_jsonc()
netdata_detect_libyaml()
netdata_detect_lz4()
netdata_detect_zstd()
netdata_detect_brotli()

if(ENABLE_LEGACY_EBPF_PROGRAMS)
        netdata_fetch_legacy_ebpf_code()
endif()

if(ENABLE_SENTRY)
        netdata_bundle_sentry()
endif()

# The remaining bundled dependencies. The stack-trace flags that used to sit
# between these calls - making their order load-bearing - moved up to the
# compiler-flags phase, so every bundled target below is instrumented and the
# calls are free to move.
if(ENABLE_ML)
  netdata_bundle_dlib()
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

# The compiled collector plugins and helper binaries, one module each: a
# module carries its own source list, executable, guards and install rule, so
# a component's whole definition is in one place.
include(NetdataNdRun)
include(NetdataPluginDebugfs)
include(NetdataPluginApps)
include(NetdataPluginFreeipmi)
include(NetdataPluginNfacct)
include(NetdataPluginXenstat)
include(NetdataPluginPerf)
include(NetdataPluginSlabinfo)
include(NetdataPluginCups)
include(NetdataNdsudo)
include(NetdataCgroupNetwork)
include(NetdataPluginSystemdJournal)
include(NetdataPluginSystemdUnits)
include(NetdataPluginWindowsEvents)
include(NetdataPluginMacosLogs)
include(NetdataPluginEBPF)
include(NetdataPluginLocalListeners)
include(NetdataPluginNetworkViewer)
include(NetdataSystemdCatNative)
include(NetdataLog2Journal)
include(NetdataCli)

# Test executables, built on demand and never installed.
include(NetdataTests)

# The daemon itself, and the Windows artifacts that ship beside it.
include(NetdataDaemon)
include(NetdataClaim)
include(NetdataWindowsDriver)
include(NetdataMSI)

# The Go and Rust plugins: go.d (with the snmp trap profile generator and the
# nd-mcp bridge), scripts.d, ibm.d, the OTel collector, and the
# NetFlow/IPFIX/sFlow analyser with the topology-ip-intel downloader that
# ships beside it.
include(NetdataPluginGo)
include(NetdataPluginScripts)
include(NetdataIBMPlugin)
include(NetdataPluginOTel)
include(NetdataPluginNetflow)

#
# Generated config header, system files, and packaging.
#
# Must stay at this position: the included file opens by snapshotting the
# compile and link options accumulated up to here.
#

include(NetdataSystemFiles)
