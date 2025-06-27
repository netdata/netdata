# SPDX-License-Identifier: GPL-3.0-or-later

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

if(DEFINED BUILD_SHARED_LIBS)
    if(NOT BUILD_SHARED_LIBS)
        set(STATIC_BUILD TRUE)
    endif()
endif()

if(STATIC_BUILD)
    set(CMAKE_FIND_LIBRARY_PREFIXES "${CMAKE_STATIC_LIBRARY_PREFIX}")
    set(CMAKE_FIND_LIBRARY_SUFFIXES "${CMAKE_STATIC_LIBRARY_SUFFIX}")
endif()

find_package(PkgConfig REQUIRED)

if(STATIC_BUILD)
    list(APPEND PKG_CONFIG_EXECUTABLE "--static")
endif()

set(CMAKE_INSTALL_MESSAGE LAZY)

set(CMAKE_C_STANDARD 11)
set(CMAKE_CXX_STANDARD 14)
set(CMAKE_INSTALL_DEFAULT_COMPONENT_NAME "netdata")

option(USE_CXX_11 "Use C++11 instead of C++14 (should only be used on legacy systems that cannot support C++14, may disable some features)" False)
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

set(CONFIG_H_DIR ${CMAKE_BINARY_DIR})
set(CONFIG_H ${CONFIG_H_DIR}/config.h)

#
# detect OS
#

set(OS_FREEBSD     False)
set(OS_LINUX       False)
set(OS_MACOS       False)
set(OS_WINDOWS     False)

set(NETDATA_RUNTIME_PREFIX "${CMAKE_INSTALL_PREFIX}")
set(BINDIR usr/sbin)

if("${CMAKE_SYSTEM_NAME}" STREQUAL "Darwin")
        set(OS_MACOS True)
        find_library(IOKIT IOKit)
        find_library(FOUNDATION Foundation)
        message(STATUS " Compiling for MacOS... ")
elseif("${CMAKE_SYSTEM_NAME}" STREQUAL "FreeBSD")
        set(OS_FREEBSD True)
        message(STATUS " Compiling for FreeBSD... ")
elseif("${CMAKE_SYSTEM_NAME}" STREQUAL "Linux")
        set(OS_LINUX True)
        add_definitions(-D_GNU_SOURCE)
        message(STATUS " Compiling for Linux... ")
elseif("${CMAKE_SYSTEM_NAME}" STREQUAL "CYGWIN" OR "${CMAKE_SYSTEM_NAME}" STREQUAL "MSYS" OR "${CMAKE_SYSTEM_NAME}" STREQUAL "Windows")
        set(OS_WINDOWS True)

        if(NOT "${CMAKE_INSTALL_PREFIX}" STREQUAL "/opt/netdata")
                message(FATAL_ERROR "CMAKE_INSTALL_PREFIX must be set to /opt/netdata, but it is set to ${CMAKE_INSTALL_PREFIX}")
        endif()

        if(BUILD_FOR_PACKAGING)
                set(NETDATA_RUNTIME_PREFIX "/")
        endif()

        set(BINDIR usr/bin)
        set(CMAKE_RC_COMPILER_INIT windres)
        ENABLE_LANGUAGE(RC)

        SET(CMAKE_RC_COMPILE_OBJECT "<CMAKE_RC_COMPILER> <FLAGS> -O coff <DEFINES> -i <SOURCE> -o <OBJECT>")
        add_definitions(-D_GNU_SOURCE)

        if($ENV{CLION_IDE})
                set(RUN_UNDER_CLION True)

                # clion needs these to find the includes
                if("${CMAKE_SYSTEM_NAME}" STREQUAL "MSYS" OR "${CMAKE_SYSTEM_NAME}" STREQUAL "Windows")
                        if("$ENV{MSYSTEM}" STREQUAL "MSYS")
                                include_directories(c:/msys64/usr/include)
                                include_directories(c:/msys64/usr/include/w32api)
                        elseif("$ENV{MSYSTEM}" STREQUAL "MINGW64")
                                include_directories(c:/msys64/mingw64/include)
                        elseif("$ENV{MSYSTEM}" STREQUAL "UCRT64")
                                include_directories(c:/msys64/ucrt64/include)
                        endif()
                endif()
        endif()

        message(STATUS " Compiling for Windows (${CMAKE_SYSTEM_NAME}, MSYSTEM=$ENV{MSYSTEM})... ")
else()
        message(FATAL_ERROR "Unknown/unsupported platform: ${CMAKE_SYSTEM_NAME} (Supported platforms: FreeBSD, Linux, macOS, Windows)")
endif()

include(NetdataCompilerFlags)

check_c_compiler_flag("-fexceptions" HAVE_FEXCEPTIONS)
if (NOT HAVE_FEXCEPTIONS)
    message(FATAL_ERROR "Missing required compiler flag: -fexceptions.")
endif()

# This is intended to make life easier for developers who are working on one
# specific feature.
#
# NOTE: DO NOT USE THIS OPTION FOR PRODUCTION BUILDS.
option(DEFAULT_FEATURE_STATE "Specify the default state for most optional features" True)
mark_as_advanced(DEFAULT_FEATURE_STATE)

# High-level features
option(ENABLE_ML "Enable machine learning features" ${DEFAULT_FEATURE_STATE})

if(ENABLE_ML)
  set(NETDATA_DLIB_SOURCE_DIR "" CACHE PATH "Path to local dlib sources for building ML code")
endif()

option(ENABLE_DBENGINE "Enable dbengine metrics storage" True)
option(ENABLE_DASHBOARD "Enable local dashboard" True)
mark_as_advanced(ENABLE_DASHBOARD)

# Data collection plugins
option(ENABLE_PLUGIN_GO "Enable metric collectors written in Go" ${DEFAULT_FEATURE_STATE})
option(ENABLE_PLUGIN_OTEL "Enable collection of OpenTelemetry metrics and logs" ${DEFAULT_FEATURE_STATE})
option(ENABLE_PLUGIN_IBM "Enable IBM ecosystem collectors (requires CGO)" False)
option(ENABLE_PLUGIN_PYTHON "Enable metric collectors written in Python" ${DEFAULT_FEATURE_STATE})

cmake_dependent_option(ENABLE_PLUGIN_APPS "Enable per-process resource usage monitoring" ${DEFAULT_FEATURE_STATE} "OS_LINUX OR OS_FREEBSD OR OS_MACOS OR OS_WINDOWS" False)
cmake_dependent_option(ENABLE_PLUGIN_CHARTS "Enable metric collectors written in Bash" ${DEFAULT_FEATURE_STATE} "NOT OS_WINDOWS" False)
cmake_dependent_option(ENABLE_PLUGIN_CUPS "Enable CUPS monitoring" ${DEFAULT_FEATURE_STATE} "NOT OS_WINDOWS" False)

cmake_dependent_option(ENABLE_PLUGIN_FREEIPMI "Enable IPMI monitoring" ${DEFAULT_FEATURE_STATE} "OS_LINUX OR OS_FREEBSD" False)

cmake_dependent_option(ENABLE_PLUGIN_CGROUP_NETWORK "Enable Linux CGroup network usage monitoring" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_PLUGIN_DEBUGFS "Enable Linux DebugFS metric collection" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_PLUGIN_EBPF "Enable Linux eBPF metric collection" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_LEGACY_EBPF_PROGRAMS "Enable eBPF programs for kernels without BTF support" True "ENABLE_PLUGIN_EBPF" False)
mark_as_advanced(ENABLE_LEGACY_EBPF_PROGRAMS)
cmake_dependent_option(ENABLE_PLUGIN_LOCAL_LISTENERS "Enable local listening socket tracking (including service auto-discovery support)" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_PLUGIN_NETWORK_VIEWER "Enable network viewer functionality" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_PLUGIN_NFACCT "Enable Linux NFACCT metric collection" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_PLUGIN_PERF "Enable Linux performance counter monitoring" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_PLUGIN_SLABINFO "Enable Linux kernel SLAB allocator monitoring" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_PLUGIN_SYSTEMD_JOURNAL "Enable systemd journal log collection" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_PLUGIN_SYSTEMD_UNITS "Enable systemd units information collection" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)
cmake_dependent_option(ENABLE_PLUGIN_XENSTAT "Enable Xen domain monitoring" ${DEFAULT_FEATURE_STATE} "OS_LINUX" False)

# Metrics exporters
option(ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE "Enable exporting to Prometheus via remote write API" ${DEFAULT_FEATURE_STATE})
option(ENABLE_EXPORTER_MONGODB "Enable exporting to MongoDB" ${DEFAULT_FEATURE_STATE})

# Vendoring
option(ENABLE_BUNDLED_JSONC "Force use of a vendored copy of JSON-C" False)
option(ENABLE_BUNDLED_YAML "Force use of a vendored copy of libyaml" False)
option(ENABLE_BUNDLED_PROTOBUF "Use a vendored copy of protobuf" False)

# Experimental features
option(ENABLE_WEBRTC "Enable WebRTC dashboard communications (experimental)" False)
mark_as_advanced(ENABLE_WEBRTC)

# Other optional functionality
option(ENABLE_SENTRY "Build with Sentry Native crash reporting" False)
mark_as_advanced(ENABLE_SENTRY)

option(BUILD_FOR_PACKAGING "Include component files for native packages" False)
mark_as_advanced(BUILD_FOR_PACKAGING)

cmake_dependent_option(ENABLE_LIBBACKTRACE "Use libbacktrace for stack traces in log output" True "OS_LINUX OR OS_WINDOWS OR OS_FREEBSD" False)
mark_as_advanced(ENABLE_LIBBACKTRACE)
cmake_dependent_option(ENABLE_LIBUNWIND "Use libunwind for stack traces in log output" False "NOT ENABLE_LIBBACKTRACE" False)
mark_as_advanced(ENABLE_LIBUNWIND)

cmake_dependent_option(FORCE_LEGACY_LIBBPF "Force usage of libbpf 0.0.9 instead of the latest version." False "ENABLE_PLUGIN_EBPF" False)
mark_as_advanced(FORCE_LEGACY_LIBBPF)

cmake_dependent_option(ENABLE_NETDATA_JOURNAL_FILE_READER "Enable netdata's journal file reader implementation" False "ENABLE_PLUGIN_SYSTEMD_JOURNAL" False)

# Setup Rust/Corrosion for plugins that need it
if(ENABLE_NETDATA_JOURNAL_FILE_READER OR ENABLE_PLUGIN_OTEL)
    include(FetchContent)
    FetchContent_Declare(
        Corrosion
        GIT_REPOSITORY https://github.com/netdata/corrosion.git
        GIT_TAG f3b91559efca32c6b54837866ef35ba98ff5b2ca # stable/v0.5
    )
    FetchContent_MakeAvailable(Corrosion)
    corrosion_import_crate(MANIFEST_PATH src/crates/jf/Cargo.toml
                           CRATES journal_reader_ffi otel-plugin)
endif()

option(ENABLE_MIMALLOC "Enable mimalloc allocator" OFF)

if(ENABLE_MIMALLOC)
    if(CMAKE_MINOR_VERSION LESS 18)
        message(WARNING "Mimalloc disabled: Requires CMake version 3.18 or higher.")
        set(ENABLE_MIMALLOC OFF CACHE BOOL "Enable mimalloc allocator" FORCE)
    elseif(CMAKE_SIZEOF_VOID_P LESS 8)
        message(WARNING "Mimalloc disabled: Only supported on 64-bit platforms.")
        set(ENABLE_MIMALLOC OFF CACHE BOOL "Enable mimalloc allocator" FORCE)
    elseif(OS_FREEBSD OR OS_MACOS OR OS_WINDOWS)
        message(WARNING "Mimalloc disabled: Not supported on FreeBSD, macOS, or Windows.")
        set(ENABLE_MIMALLOC OFF CACHE BOOL "Enable mimalloc allocator" FORCE)
    elseif(ENABLE_ADDRESS_SANITIZER)
        message(WARNING "Mimalloc disabled: Cannot be used with Address Sanitizer.")
        set(ENABLE_MIMALLOC OFF CACHE BOOL "Enable mimalloc allocator" FORCE)
    else()
        message(STATUS "Mimalloc is enabled.")
    endif()
else()
    message(STATUS "Mimalloc is disabled.")
endif()

if(ENABLE_MIMALLOC)
        function(netdata_add_mimalloc)
                set(MI_BUILD_STATIC ON CACHE INTERNAL "")
                set(MI_BUILD_SHARED OFF CACHE INTERNAL "")
                set(MI_BUILD_OBJECT OFF CACHE INTERNAL "")
                set(MI_BUILD_TESTS OFF CACHE INTERNAL "")

                include(FetchContent)
                include(NetdataFetchContentExtra)

                if(CMAKE_VERSION VERSION_GREATER_EQUAL 3.28)
                        FetchContent_Declare(mimalloc
                                GIT_REPOSITORY https://github.com/microsoft/mimalloc.git
                                GIT_TAG 8c532c32c3c96e5ba1f2283e032f69ead8add00f
                                EXCLUDE_FROM_ALL
                        )
                else()
                        FetchContent_Declare(mimalloc
                                GIT_REPOSITORY https://github.com/microsoft/mimalloc.git
                                GIT_TAG 8c532c32c3c96e5ba1f2283e032f69ead8add00f
                        )
                endif()

                FetchContent_MakeAvailable_NoInstall(mimalloc)
        endfunction()

        netdata_add_mimalloc()
endif()

option(ENABLE_JEMALLOC "Disable jemalloc allocator" OFF)

if(ENABLE_JEMALLOC)
    pkg_check_modules(JEMALLOC QUIET jemalloc)
    if(JEMALLOC_FOUND)
        # Check if jemalloc has arena API
        set(CMAKE_REQUIRED_INCLUDES ${JEMALLOC_INCLUDE_DIRS})
        set(CMAKE_REQUIRED_LIBRARIES ${JEMALLOC_LIBRARIES})
        check_c_source_compiles("
            #include <jemalloc/jemalloc.h>
            int main() {
                unsigned narenas;
                size_t sz = sizeof(narenas);
                mallctl(\"arenas.narenas\", &narenas, &sz, NULL, 0);
                return 0;
            }
        " HAVE_JEMALLOC_ARENA_API)

        if(HAVE_JEMALLOC_ARENA_API)
            set(ENABLE_JEMALLOC ON CACHE BOOL "Enable jemalloc allocator" FORCE)
            message(STATUS "Jemalloc found with arena API support - enabling")
        else()
            if(ENABLE_JEMALLOC)
                message(FATAL_ERROR "Jemalloc was found but does not have arena API support")
            endif()
            message(STATUS "Jemalloc found but does not have arena API support - disabling")
        endif()
    else()
        if(ENABLE_JEMALLOC)
            message(FATAL_ERROR "Jemalloc support was explicitly enabled but jemalloc was not found")
        endif()
        message(STATUS "Jemalloc not found - disabling")
    endif()
endif()

if(ENABLE_PLUGIN_GO)
    include(NetdataGoTools)

    find_min_go_version("${CMAKE_SOURCE_DIR}/src/go")

    find_package(Go "${MIN_GO_VERSION}" REQUIRED)
endif()

if(ENABLE_PLUGIN_GO)
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

include(NetdataProtobuf)

if(ENABLE_BUNDLED_PROTOBUF)
        netdata_bundle_protobuf()
endif()

set(PKG_FILES_PATH "${CMAKE_SOURCE_DIR}/packaging/cmake/pkg-files")

if(ENABLE_PLUGIN_EBPF)
        include(NetdataLibBPF)
        include(NetdataEBPFCORE)

        if(NOT OS_LINUX)
            message(FATAL_ERROR "The eBPF plugin is not supported on non-Linux systems")
        endif()

        netdata_bundle_libbpf()
        netdata_fetch_ebpf_co_re()
endif()

pkg_check_modules(CURL libcurl>=7.21 REQUIRED IMPORTED_TARGET)
set(HAVE_LIBCURL TRUE)

#
# Libm
#

# checks link with cmake required libs
cmake_policy(SET CMP0075 NEW)

include(CheckFunctionExists)

check_function_exists(log10 HAVE_LOG10)
if(NOT HAVE_LOG10)
        unset(HAVE_LOG10 CACHE)
        list(APPEND CMAKE_REQUIRED_LIBRARIES m)
        check_function_exists(log10 HAVE_LOG10)
        if(HAVE_LOG10)
                set(LINK_LIBM True)
        else()
                message(FATAL_ERROR "Can not use log10 with/without libm.")
        endif()
endif()

#
# Custom Modules
#

include(NetdataJSONC)
include(NetdataYAML)
include(NetdataBacktrace)
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

if(ENABLE_ML)
  netdata_bundle_dlib()
endif()

#
# check include files
#

include(CheckIncludeFile)

check_include_file("netinet/in.h" HAVE_NETINET_IN_H)
check_include_file("resolv.h" HAVE_RESOLV_H)
check_include_file("netdb.h" HAVE_NETDB_H)
check_include_file("sys/prctl.h" HAVE_SYS_PRCTL_H)
check_include_file("sys/stat.h" HAVE_SYS_STAT_H)
check_include_file("sys/vfs.h" HAVE_SYS_VFS_H)
check_include_file("sys/statfs.h" HAVE_SYS_STATFS_H)
check_include_file("linux/magic.h" HAVE_LINUX_MAGIC_H)
check_include_file("sys/mount.h" HAVE_SYS_MOUNT_H)
check_include_file("sys/statvfs.h" HAVE_SYS_STATVFS_H)
check_include_file("inttypes.h" HAVE_INTTYPES_H)
check_include_file("stdint.h" HAVE_STDINT_H)
check_include_file("arpa/inet.h" HAVE_ARPA_INET_H)
check_include_file("netinet/tcp.h" HAVE_NETINET_TCP_H)
check_include_file("sys/ioctl.h" HAVE_SYS_IOCTL_H)
check_include_file("grp.h" HAVE_GRP_H)
check_include_file("pwd.h" HAVE_PWD_H)
check_include_file("net/if.h" HAVE_NET_IF_H)
check_include_file("poll.h" HAVE_POLL_H)
check_include_file("syslog.h" HAVE_SYSLOG_H)
check_include_file("sys/mman.h" HAVE_SYS_MMAN_H)
check_include_file("sys/resource.h" HAVE_SYS_RESOURCE_H)
check_include_file("sys/socket.h" HAVE_SYS_SOCKET_H)
check_include_file("sys/wait.h" HAVE_SYS_WAIT_H)
check_include_file("sys/un.h" HAVE_SYS_UN_H)
check_include_file("spawn.h" HAVE_SPAWN_H)
if(OS_LINUX)
    check_include_file("sys/capability.h" HAVE_SYS_CAPABILITY_H)
endif()

#
# check symbols
#

include(CheckSymbolExists)
check_symbol_exists(major "sys/sysmacros.h" MAJOR_IN_SYSMACROS)
check_symbol_exists(major "sys/mkdev.h" MAJOR_IN_MKDEV)
check_symbol_exists(clock_gettime "time.h" HAVE_CLOCK_GETTIME)
check_symbol_exists(strerror_r "string.h" HAVE_STRERROR_R)
check_symbol_exists(finite "math.h" HAVE_FINITE)
check_symbol_exists(isfinite "math.h" HAVE_ISFINITE)
check_symbol_exists(dlsym "dlfcn.h" HAVE_DLSYM)

check_function_exists(pthread_getthreadid_np HAVE_PTHREAD_GETTHREADID_NP)
check_function_exists(pthread_threadid_np HAVE_PTHREAD_THREADID_NP)
check_function_exists(gettid HAVE_GETTID)
check_function_exists(waitid HAVE_WAITID)
check_function_exists(nice HAVE_NICE)
check_function_exists(recvmmsg HAVE_RECVMMSG)
check_function_exists(getpriority HAVE_GETPRIORITY)
check_function_exists(setenv HAVE_SETENV)
check_function_exists(strndup HAVE_STRNDUP)

check_function_exists(sched_getscheduler HAVE_SCHED_GETSCHEDULER)
check_function_exists(sched_setscheduler HAVE_SCHED_SETSCHEDULER)
check_function_exists(sched_get_priority_min HAVE_SCHED_GET_PRIORITY_MIN)
check_function_exists(sched_get_priority_max HAVE_SCHED_GET_PRIORITY_MAX)

check_function_exists(close_range HAVE_CLOSE_RANGE)
check_function_exists(backtrace HAVE_BACKTRACE)

check_function_exists(arc4random_buf HAVE_ARC4RANDOM_BUF)
check_function_exists(arc4random_uniform HAVE_ARC4RANDOM_UNIFORM)
check_function_exists(getrandom HAVE_GETRANDOM)
check_function_exists(sysinfo HAVE_SYSINFO)

check_function_exists(timegm HAVE_TIMEGM)

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

#
# check source compilation
#

include(CheckCSourceCompiles)
include(CheckCXXSourceCompiles)

check_c_source_compiles("
#include <time.h>
int main(void) {
    struct tm t;
    (void)t.tm_gmtoff;
    return 0;
}
" HAVE_TM_GMTOFF)

set(CMAKE_REQUIRED_LIBRARIES pthread)
check_c_source_compiles("
#define _GNU_SOURCE
#include <pthread.h>
int main() {
        char name[16];
        pthread_t thread = pthread_self();
        return pthread_getname_np(thread, name, sizeof(name));
}
" HAVE_PTHREAD_GETNAME_NP)

check_c_source_compiles("
#include <stdio.h>
#define mytype(X) _Generic((X), int: 'i', float: 'f', default: 'u')
int main() {
        char type = mytype(0);
        return 0;
}
" HAVE_C__GENERIC)

check_c_source_compiles("
#include <malloc.h>
int main() {
        mallopt(M_ARENA_MAX, 1);
        mallopt(M_PERTURB, 0x5A);
        return 0;
}
" HAVE_C_MALLOPT)

check_c_source_compiles("
#include <malloc.h>
int main() {
        malloc_trim(0);
        return 0;
}
" HAVE_C_MALLOC_TRIM)

check_c_source_compiles("
#include <malloc.h>
int main() {
        malloc_info(0, stdout);
        return 0;
}
" HAVE_C_MALLOC_INFO)

check_c_source_compiles("
#include <malloc.h>
int main() {
        struct mallinfo2 m = mallinfo2();
        return 0;
}
" HAVE_C_MALLINFO2)

check_c_source_compiles("
#define _GNU_SOURCE
#include <stdio.h>
#include <sys/socket.h>
int main() {
        accept4(0, NULL, NULL, 0);
        return 0;
}
" HAVE_ACCEPT4)

check_c_source_compiles("
#define _GNU_SOURCE
#include <string.h>
int main() {
        char x = *strerror_r(0, &x, sizeof(x)); return 0;
}
" STRERROR_R_CHAR_P)

check_c_source_compiles("
#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif
#include <sched.h>
int main() {
        setns(0, 0); return 0;
}
" HAVE_SETNS)

check_cxx_source_compiles("
int main() {
        __atomic_load_8(nullptr, 0);
        return 0;
}
" HAVE_BUILTIN_ATOMICS)

check_cxx_source_compiles("
#include <stdint.h>
int main(void) {
        uint64_t a;
        __sync_add_and_fetch(&a, 1);
        return 0;
}
" ARCH_SUPPORTS_64BIT_ATOMICS)

check_c_source_compiles("
void my_printf(char const *s, ...) __attribute__((format(gnu_printf, 1, 2)));
int main() { return 0; }
" HAVE_FUNC_ATTRIBUTE_FORMAT_GNU_PRINTF FAIL_REGEX "warning:")

check_c_source_compiles("
void my_printf(char const *s, ...) __attribute__((format(printf, 1, 2)));
int main() { return 0; }
" HAVE_FUNC_ATTRIBUTE_FORMAT_PRINTF FAIL_REGEX "warning:")

check_c_source_compiles("
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
void* my_alloc(size_t size) __attribute__((malloc));
int main() {
    void *x = my_alloc(1);
    free(x);
    return 0;
}
void* my_alloc(size_t size) {
    void *ret = malloc(size);
    if(!ret) exit(1);
    return ret;
}
" HAVE_FUNC_ATTRIBUTE_MALLOC)

check_c_source_compiles("
void my_function() __attribute__((noinline));
int main() { my_function(); return 0; }
void my_function() { ; }
" HAVE_FUNC_ATTRIBUTE_NOINLINE)

check_c_source_compiles("
#include <stdlib.h>
void my_exit_function() __attribute__((noreturn));
int main() {
        my_exit_function(); // Call the noreturn function
        return 0;
}
void my_exit_function() {
        exit(1);
}
" HAVE_FUNC_ATTRIBUTE_NORETURN)

check_c_source_compiles("
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
void* my_alloc(size_t size) __attribute__((returns_nonnull));
int main() {
        void* ptr = my_alloc(10);
        free(ptr);
        return 0;
}
void* my_alloc(size_t size) {
        void *ret = malloc(size);
        if(!ret) exit(1);
        return ret;
}
" HAVE_FUNC_ATTRIBUTE_RETURNS_NONNULL)

check_c_source_compiles("
int my_function() __attribute__((warn_unused_result));
int main() {
        return my_function();
}
int my_function() {
        return 1;
}
" HAVE_FUNC_ATTRIBUTE_WARN_UNUSED_RESULT)

# Windows MSVCRT random number generator
# used only when compiling natively (not MSYS/CYGWIN)
check_c_source_compiles("
    #define _CRT_RAND_S
    #include <stdlib.h>
    int main() {
        unsigned int x;
        return rand_s(&x);
    }
" HAVE_RAND_S)

if(OS_FREEBSD OR OS_MACOS)
        set(HAVE_BUILTIN_ATOMICS True)
endif()

# openssl/crypto
pkg_check_modules(TLS IMPORTED_TARGET openssl)

if(NOT TARGET PkgConfig::TLS)
        if(OS_MACOS)
                execute_process(COMMAND
                                brew --prefix --installed openssl
                                RESULT_VARIABLE BREW_OPENSSL
                                OUTPUT_VARIABLE BREW_OPENSSL_PREFIX
                                OUTPUT_STRIP_TRAILING_WHITESPACE)

                if((BREW_OPENSSL NOT EQUAL 0) OR (NOT EXISTS "${BREW_OPENSSL_PREFIX}"))
                        message(FATAL_ERROR "OpenSSL (or LibreSSL) is required for building Netdata, but could not be found.")
                endif()

                add_library(PkgConfig::CRYPTO IMPORTED)
                set_target_properties(PkgConfig::CRYPTO
                        IMPORTED_LOCATION ${BREW_OPENSSL_PREFIX}/lib/libcrypto.dylib
                        INTERFACE_INCLUDE_DIRECTORIES ${BREW_OPENSSL_PREFIX}/include)

                add_library(PkgConfig::TLS IMPORTED)
                set_target_properties(PkgConfig::TLS
                        IMPORTED_LOCATION ${BREW_OPENSSL_PREFIX}/lib/libssl.dylib
                        INTERFACE_LINK_LIBRARIES PkgConfig::CRYPTO
                        INTERFACE_INCLUDE_DIRECTORIES ${BREW_OPENSSL_PREFIX}/include)
        else()
            message(FATAL_ERROR "OpenSSL (or LibreSSL) is required for building Netdata, but could not be found.")
        endif()
else()
        pkg_check_modules(CRYPTO IMPORTED_TARGET REQUIRED libcrypto)
endif()

netdata_detect_protobuf()

if(OS_LINUX)
        include(NetdataDetectSystemd)
        detect_systemd()
endif()

#
# source files
#

set(LIBJUDY_PREV_FILES
        src/libnetdata/libjudy/vendored/JudyL/JudyLPrev.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLPrevEmpty.c
)

set(LIBJUDY_NEXT_FILES
        src/libnetdata/libjudy/vendored/JudyL/JudyLNext.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLNextEmpty.c
)

set(LIBJUDY_SOURCES
        src/libnetdata/libjudy/vendored/Judy.h
        src/libnetdata/libjudy/vendored/JudyCommon/JudyMalloc.c
        src/libnetdata/libjudy/vendored/JudyCommon/JudyPrivate.h
        src/libnetdata/libjudy/vendored/JudyCommon/JudyPrivate1L.h
        src/libnetdata/libjudy/vendored/JudyCommon/JudyPrivateBranch.h
        src/libnetdata/libjudy/vendored/JudyL/JudyL.h
        src/libnetdata/libjudy/vendored/JudyL/JudyLByCount.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLCascade.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLCount.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLCreateBranch.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLDecascade.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLDel.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLFirst.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLFreeArray.c
        src/libnetdata/libjudy/vendored/JudyL/j__udyLGet.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLGet.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLInsArray.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLIns.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLInsertBranch.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLMallocIF.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLMemActive.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLMemUsed.c
        src/libnetdata/libjudy/vendored/JudyL/JudyLTables.c
        src/libnetdata/libjudy/vendored/JudyHS/JudyHS.c
        ${LIBJUDY_PREV_FILES}
        ${LIBJUDY_NEXT_FILES}
)

set(INICFG_FILES
        src/libnetdata/inicfg/inicfg.c
        src/libnetdata/inicfg/inicfg.h
        src/libnetdata/inicfg/inicfg_internals.h
        src/libnetdata/inicfg/inicfg_exporters.c
        src/libnetdata/inicfg/inicfg_conf_file.c
        src/libnetdata/inicfg/inicfg_cleanup.c
        src/libnetdata/inicfg/inicfg_sections.c
        src/libnetdata/inicfg/inicfg_options.c
        src/libnetdata/inicfg/inicfg_migrate.c
        src/libnetdata/inicfg/inicfg_traversal.c
        src/libnetdata/inicfg/inicfg_api.c
        src/libnetdata/inicfg/dyncfg.c
        src/libnetdata/inicfg/dyncfg.h
)

set(CONFIG_FILES
        src/libnetdata/config/config.h
)

set(LIBNETDATA_FILES
        ${CONFIG_H}
        src/libnetdata/adaptive_resortable_list/adaptive_resortable_list.c
        src/libnetdata/adaptive_resortable_list/adaptive_resortable_list.h
        src/libnetdata/aral/aral.c
        src/libnetdata/aral/aral.h
        src/libnetdata/avl/avl.c
        src/libnetdata/avl/avl.h
        src/libnetdata/bitmap/bitmap64.h
        src/libnetdata/buffer/buffer.c
        src/libnetdata/buffer/buffer.h
        src/libnetdata/buffer/functions_fields.c
        src/libnetdata/buffer/functions_fields.h
        src/libnetdata/ringbuffer/ringbuffer.c
        src/libnetdata/ringbuffer/ringbuffer.h
        src/libnetdata/circular_buffer/circular_buffer.c
        src/libnetdata/circular_buffer/circular_buffer.h
        src/libnetdata/clocks/clocks.c
        src/libnetdata/clocks/clocks.h
        src/libnetdata/completion/completion.c
        src/libnetdata/completion/completion.h
        src/libnetdata/datetime/iso8601.c
        src/libnetdata/datetime/iso8601.h
        src/libnetdata/datetime/rfc7231.c
        src/libnetdata/datetime/rfc7231.h
        src/libnetdata/datetime/rfc3339.c
        src/libnetdata/datetime/rfc3339.h
        src/libnetdata/dictionary/dictionary.c
        src/libnetdata/dictionary/dictionary-debug.c
        src/libnetdata/dictionary/dictionary-debug.h
        src/libnetdata/dictionary/dictionary.h
        src/libnetdata/eval/eval-parser-legacy.c
        src/libnetdata/eval/eval-evaluate.c
        src/libnetdata/eval/eval-utils.c
        src/libnetdata/eval/eval.h
        src/libnetdata/eval/eval-internal.h
        src/libnetdata/eval/eval-unittest.c
        src/libnetdata/eval/eval-unittest-hardcoding.c
        src/libnetdata/eval/re2c_lemon/lexer.c
        src/libnetdata/eval/re2c_lemon/parser.c
        src/libnetdata/eval/re2c_lemon/parser_wrapper.c
        src/libnetdata/facets/facets.c
        src/libnetdata/facets/facets.h
        src/libnetdata/functions_evloop/functions_evloop.c
        src/libnetdata/functions_evloop/functions_evloop.h
        src/libnetdata/gorilla/gorilla.cc
        src/libnetdata/gorilla/gorilla.h
        src/libnetdata/inlined.h
        src/libnetdata/json/json.c
        src/libnetdata/json/json.h
        src/libnetdata/json/json-keys.c
        src/libnetdata/json/json-keys.h
        src/libnetdata/json/vendored/jsmn.c
        src/libnetdata/json/vendored/jsmn.h
        src/libnetdata/libnetdata.c
        src/libnetdata/libnetdata.h
        src/libnetdata/line_splitter/line_splitter.c
        src/libnetdata/line_splitter/line_splitter.h
        src/libnetdata/libnetdata.h
        src/libnetdata/linked_lists/linked_lists.h
        src/libnetdata/locks/locks.c
        src/libnetdata/locks/locks.h
        src/libnetdata/log/systemd-journal-helpers.c
        src/libnetdata/log/systemd-journal-helpers.h
        src/libnetdata/log/nd_log.c
        src/libnetdata/log/nd_log.h
        src/libnetdata/os/os.c
        src/libnetdata/os/os.h
        src/libnetdata/os/byteorder.h
        src/libnetdata/onewayalloc/onewayalloc.c
        src/libnetdata/onewayalloc/onewayalloc.h
        src/libnetdata/procfile/procfile.c
        src/libnetdata/procfile/procfile.h
        src/libnetdata/query_progress/progress.c
        src/libnetdata/query_progress/progress.h
        src/libnetdata/required_dummies.h
        src/libnetdata/socket/security.c
        src/libnetdata/socket/security.h
        src/libnetdata/simple_hashtable/simple_hashtable.h
        src/libnetdata/simple_hashtable/simple_hashtable_undef.h
        src/libnetdata/simple_pattern/simple_pattern.c
        src/libnetdata/simple_pattern/simple_pattern.h
        src/libnetdata/socket/socket.c
        src/libnetdata/socket/socket.h
        src/libnetdata/statistical/statistical.c
        src/libnetdata/statistical/statistical.h
        src/libnetdata/storage_number/storage_number.c
        src/libnetdata/storage_number/storage_number.h
        src/libnetdata/string/string.c
        src/libnetdata/string/string.h
        src/libnetdata/threads/threads.c
        src/libnetdata/threads/threads.h
        src/libnetdata/url/url.c
        src/libnetdata/url/url.h
        src/libnetdata/uuid/uuid.c
        src/libnetdata/uuid/uuid.h
        src/libnetdata/string/utf8.h
        src/libnetdata/worker_utilization/worker_utilization.c
        src/libnetdata/worker_utilization/worker_utilization.h
        src/libnetdata/user-auth/http-access.c
        src/libnetdata/user-auth/http-access.h
        src/libnetdata/user-auth/user-auth.c
        src/libnetdata/user-auth/user-auth.h
        src/libnetdata/http/http_defs.c
        src/libnetdata/http/http_defs.h
        src/libnetdata/http/content_type.c
        src/libnetdata/http/content_type.h
        src/libnetdata/json/json-c-parser-inline.h
        src/libnetdata/template-enum.h
        src/libnetdata/dictionary/dictionary-internals.h
        src/libnetdata/dictionary/dictionary-unittest.c
        src/libnetdata/dictionary/thread-cache.c
        src/libnetdata/dictionary/thread-cache.h
        src/libnetdata/dictionary/dictionary-traversal.c
        src/libnetdata/dictionary/dictionary-statistics.h
        src/libnetdata/dictionary/dictionary-locks.h
        src/libnetdata/dictionary/dictionary-refcount.h
        src/libnetdata/dictionary/dictionary-hashtable.h
        src/libnetdata/dictionary/dictionary-item.h
        src/libnetdata/dictionary/dictionary-callbacks.h
        src/libnetdata/storage-point.h
        src/libnetdata/parsers/parsers.h
        src/libnetdata/parsers/duration.c
        src/libnetdata/parsers/duration-unittest.c
        src/libnetdata/os/gettid.c
        src/libnetdata/os/gettid.h
        src/libnetdata/os/adjtimex.c
        src/libnetdata/os/adjtimex.h
        src/libnetdata/os/setresuid.c
        src/libnetdata/os/setresuid.h
        src/libnetdata/os/setresgid.c
        src/libnetdata/os/setresgid.h
        src/libnetdata/os/getgrouplist.c
        src/libnetdata/os/getgrouplist.h
        src/libnetdata/os/get_pid_max.c
        src/libnetdata/os/get_pid_max.h
        src/libnetdata/os/os-freebsd-wrappers.c
        src/libnetdata/os/os-freebsd-wrappers.h
        src/libnetdata/os/os-macos-wrappers.c
        src/libnetdata/os/os-macos-wrappers.h
        src/libnetdata/os/os-windows-wrappers.c
        src/libnetdata/os/os-windows-wrappers.h
        src/libnetdata/os/get_system_cpus.c
        src/libnetdata/os/get_system_cpus.h
        src/libnetdata/os/sleep.c
        src/libnetdata/os/sleep.h
        src/libnetdata/os/uuid_generate.c
        src/libnetdata/os/uuid_generate.h
        src/libnetdata/os/setenv.c
        src/libnetdata/os/setenv.h
        src/libnetdata/os/strndup.c
        src/libnetdata/os/strndup.h
        src/libnetdata/os/windows-wmi/windows-wmi.c
        src/libnetdata/os/windows-wmi/windows-wmi.h
        src/libnetdata/os/windows-wmi/windows-wmi-GetDiskDriveInfo.c
        src/libnetdata/os/windows-wmi/windows-wmi-GetDiskDriveInfo.h
        src/libnetdata/os/windows-perflib/perflib.c
        src/libnetdata/os/windows-perflib/perflib.h
        src/libnetdata/os/windows-perflib/perflib-names.c
        src/libnetdata/os/windows-perflib/perflib-dump.c
        src/libnetdata/os/system-maps/cached-uid-username.c
        src/libnetdata/os/system-maps/cached-uid-username.h
        src/libnetdata/os/system-maps/cached-sid-username.c
        src/libnetdata/os/system-maps/cached-sid-username.h
        src/libnetdata/os/system-maps/cached-gid-groupname.c
        src/libnetdata/os/system-maps/cached-gid-groupname.h
        src/libnetdata/os/system-maps/cache-host-users-and-groups.c
        src/libnetdata/os/system-maps/cache-host-users-and-groups.h
        src/libnetdata/spawn_server/spawn_server_nofork.c
        src/libnetdata/spawn_server/spawn_server.h
        src/libnetdata/spawn_server/spawn_popen.c
        src/libnetdata/spawn_server/spawn_popen.h
        src/libnetdata/spawn_server/spawn_server_windows.c
        src/libnetdata/spawn_server/spawn_server_internals.h
        src/libnetdata/spawn_server/spawn_server_libuv.c
        src/libnetdata/spawn_server/spawn_server_posix.c
        src/libnetdata/spawn_server/spawn_library.c
        src/libnetdata/spawn_server/spawn_library.h
        src/libnetdata/os/close_range.c
        src/libnetdata/os/close_range.h
        src/libnetdata/os/setproctitle.c
        src/libnetdata/os/setproctitle.h
        src/libnetdata/paths/paths.c
        src/libnetdata/paths/paths.h
        src/libnetdata/stacktrace/stacktrace.h
        src/libnetdata/stacktrace/stacktrace-common.h
        src/libnetdata/stacktrace/stacktrace-common.c
        src/libnetdata/stacktrace/stacktrace-array.h
        src/libnetdata/stacktrace/stacktrace-array.c
        src/libnetdata/stacktrace/stacktrace-libbacktrace.c
        src/libnetdata/stacktrace/stacktrace-libunwind.c
        src/libnetdata/stacktrace/stacktrace-backtrace.c
        src/libnetdata/stacktrace/stacktrace-none.c
        src/libnetdata/stacktrace/stacktrace-log.c
        src/libnetdata/stacktrace/stacktrace-unittest.c
        src/libnetdata/json/json-c-parser-inline.c
        src/libnetdata/parsers/duration.h
        src/libnetdata/parsers/timeframe.c
        src/libnetdata/parsers/timeframe.h
        src/libnetdata/parsers/size.c
        src/libnetdata/parsers/size.h
        src/libnetdata/libjudy/judy-malloc.c
        src/libnetdata/libjudy/judy-malloc.h
        src/libnetdata/facets/logs_query_status.h
        src/libnetdata/os/timestamps.c
        src/libnetdata/os/timestamps.h
        src/libnetdata/parsers/entries.c
        src/libnetdata/parsers/entries.h
        src/libnetdata/sanitizers/chart_id_and_name.c
        src/libnetdata/sanitizers/chart_id_and_name.h
        src/libnetdata/sanitizers/utf8-sanitizer.c
        src/libnetdata/sanitizers/utf8-sanitizer.h
        src/libnetdata/sanitizers/sanitizers.h
        src/libnetdata/sanitizers/sanitizers-labels.c
        src/libnetdata/sanitizers/sanitizers-labels.h
        src/libnetdata/sanitizers/sanitizers-functions.c
        src/libnetdata/sanitizers/sanitizers-functions.h
        src/libnetdata/sanitizers/sanitizers-pluginsd.c
        src/libnetdata/sanitizers/sanitizers-pluginsd.h
        src/libnetdata/log/nd_log-internals.c
        src/libnetdata/log/nd_log-internals.h
        src/libnetdata/log/nd_log_limit.c
        src/libnetdata/log/nd_log_limit.h
        src/libnetdata/log/nd_log-config.c
        src/libnetdata/log/nd_log-init.c
        src/libnetdata/log/nd_log-to-syslog.c
        src/libnetdata/log/nd_log-to-systemd-journal.c
        src/libnetdata/log/nd_log-annotators.c
        src/libnetdata/log/nd_log-field-formatters.c
        src/libnetdata/log/nd_log-format-logfmt.c
        src/libnetdata/log/nd_log-format-json.c
        src/libnetdata/log/nd_log-to-file.c
        src/libnetdata/log/nd_log-to-windows-events.c
        src/libnetdata/string/utf8.c
        src/libnetdata/spawn_server/log-forwarder.c
        src/libnetdata/spawn_server/log-forwarder.h
        src/libnetdata/log/nd_log-common.h
        src/libnetdata/log/nd_log-to-windows-common.h
        src/libnetdata/common.h
        src/libnetdata/xxHash/xxhash.h
        src/libnetdata/os/random.c
        src/libnetdata/os/random.h
        src/libnetdata/socket/nd-sock.c
        src/libnetdata/socket/nd-sock.h
        src/libnetdata/socket/listen-sockets.c
        src/libnetdata/socket/listen-sockets.h
        src/libnetdata/socket/poll-events.c
        src/libnetdata/socket/poll-events.h
        src/libnetdata/socket/connect-to.c
        src/libnetdata/socket/connect-to.h
        src/libnetdata/socket/socket-peers.c
        src/libnetdata/socket/socket-peers.h
        src/libnetdata/libjudy/judyl-typed.h
        src/libnetdata/os/system_memory.c
        src/libnetdata/os/system_memory.h
        src/libnetdata/socket/nd-poll.c
        src/libnetdata/socket/nd-poll.h
        src/libnetdata/locks/spinlock.c
        src/libnetdata/locks/spinlock.h
        src/libnetdata/locks/rw-spinlock.c
        src/libnetdata/locks/rw-spinlock.h
        src/libnetdata/atomics/atomic_flags.h
        src/libnetdata/atomics/atomics.h
        src/libnetdata/locks/waitq.c
        src/libnetdata/locks/waitq.h
        src/libnetdata/object-state/object-state.c
        src/libnetdata/object-state/object-state.h
        src/libnetdata/uuid/uuidmap.c
        src/libnetdata/uuid/uuidmap.h
        src/libnetdata/atomics/refcount.h
        src/libnetdata/log/nd_log-fatal.h
        src/libnetdata/locks/benchmark.c
        src/libnetdata/locks/benchmark.h
        src/libnetdata/locks/benchmark-rw.c
        src/libnetdata/locks/benchmark-rw.h
        src/libnetdata/memory/nd-mallocz.c
        src/libnetdata/memory/nd-mallocz.h
        src/libnetdata/memory/nd-mmap.c
        src/libnetdata/memory/nd-mmap.h
        src/libnetdata/memory/alignment.h
        src/libnetdata/os/get_system_pagesize.c
        src/libnetdata/os/get_system_pagesize.h
        src/libnetdata/os/hostname.c
        src/libnetdata/os/hostname.h
        src/libnetdata/exit/exit_initiated.c
        src/libnetdata/exit/exit_initiated.h
        src/libnetdata/os/disk_space.c
        src/libnetdata/os/disk_space.h
        src/libnetdata/os/file_metadata.c
        src/libnetdata/os/file_metadata.h
        src/libnetdata/os/process_path.c
        src/libnetdata/os/process_path.h
        src/libnetdata/os/boottime.c
        src/libnetdata/os/boottime.h
        src/libnetdata/os/boot_id.c
        src/libnetdata/os/boot_id.h
        src/libnetdata/os/run_dir.c
        src/libnetdata/os/run_dir.h
        src/libnetdata/os/file_lock.c
        src/libnetdata/os/file_lock.h
        src/libnetdata/os/mmap_limit.c
        src/libnetdata/os/mmap_limit.h
        src/libnetdata/signals/signals.c
        src/libnetdata/signals/signals.h
        src/libnetdata/os/machine_id.c
        src/libnetdata/os/machine_id.h
        src/libnetdata/os/process_memory.c
        src/libnetdata/os/process_memory.h
        src/libnetdata/os/dir_size.c
        src/libnetdata/os/dir_size.h
        src/libnetdata/signals/signal-code.c
        src/libnetdata/signals/signal-code.h
)

list(APPEND LIBNETDATA_FILES ${INICFG_FILES})
list(APPEND LIBNETDATA_FILES ${CONFIG_FILES})

set(DAEMON_FILES
        src/daemon/buildinfo.c
        src/daemon/buildinfo.h
        src/daemon/common.c
        src/daemon/common.h
        src/daemon/daemon.c
        src/daemon/daemon.h
        src/daemon/libuv_workers.c
        src/daemon/libuv_workers.h
        src/daemon/pulse/pulse.c
        src/daemon/pulse/pulse.h
        src/daemon/analytics.c
        src/daemon/analytics.h
        src/daemon/main.c
        src/daemon/main.h
        src/daemon/environment.c
        src/daemon/win_system-info.c
        src/daemon/win_system-info.h
        src/daemon/signal-handler.c
        src/daemon/signal-handler.h
        src/daemon/service.c
        src/daemon/daemon-shutdown-watcher.c
        src/daemon/daemon-shutdown-watcher.h
        src/daemon/static_threads.c
        src/daemon/static_threads.h
        src/daemon/commands.c
        src/daemon/commands.h
        src/daemon/pipename.c
        src/daemon/pipename.h
        src/daemon/unit_test.c
        src/daemon/unit_test.h
        src/daemon/dyncfg/dyncfg.c
        src/daemon/dyncfg/dyncfg.h
        src/daemon/dyncfg/dyncfg-files.c
        src/daemon/dyncfg/dyncfg-unittest.c
        src/daemon/dyncfg/dyncfg-inline.c
        src/daemon/dyncfg/dyncfg-echo.c
        src/daemon/dyncfg/dyncfg-internals.h
        src/daemon/dyncfg/dyncfg-intercept.c
        src/daemon/dyncfg/dyncfg-tree.c
        src/daemon/pulse/pulse-http-api.c
        src/daemon/pulse/pulse-http-api.h
        src/daemon/pulse/pulse-queries.c
        src/daemon/pulse/pulse-queries.h
        src/daemon/pulse/pulse-ingestion.c
        src/daemon/pulse/pulse-ingestion.h
        src/daemon/pulse/pulse-ml.c
        src/daemon/pulse/pulse-ml.h
        src/daemon/pulse/pulse-gorilla.c
        src/daemon/pulse/pulse-gorilla.h
        src/daemon/pulse/pulse-daemon.c
        src/daemon/pulse/pulse-daemon.h
        src/daemon/pulse/pulse-daemon-memory.c
        src/daemon/pulse/pulse-daemon-memory.h
        src/daemon/pulse/pulse-sqlite3.c
        src/daemon/pulse/pulse-sqlite3.h
        src/daemon/pulse/pulse-db-dbengine.c
        src/daemon/pulse/pulse-db-dbengine.h
        src/daemon/pulse/pulse-string.c
        src/daemon/pulse/pulse-string.h
        src/daemon/pulse/pulse-heartbeat.c
        src/daemon/pulse/pulse-heartbeat.h
        src/daemon/pulse/pulse-dictionary.c
        src/daemon/pulse/pulse-dictionary.h
        src/daemon/pulse/pulse-workers.c
        src/daemon/pulse/pulse-workers.h
        src/daemon/pulse/pulse-trace-allocations.c
        src/daemon/pulse/pulse-trace-allocations.h
        src/daemon/pulse/pulse-aral.c
        src/daemon/pulse/pulse-aral.h
        src/daemon/config/netdata-conf-db.c
        src/daemon/config/netdata-conf-db.h
        src/daemon/config/netdata-conf.h
        src/daemon/config/netdata-conf-backwards-compatibility.c
        src/daemon/config/netdata-conf-backwards-compatibility.h
        src/daemon/config/netdata-conf-web.c
        src/daemon/config/netdata-conf-web.h
        src/daemon/config/netdata-conf-directories.c
        src/daemon/config/netdata-conf-directories.h
        src/daemon/config/netdata-conf-logs.c
        src/daemon/config/netdata-conf-logs.h
        src/daemon/config/netdata-conf-global.c
        src/daemon/config/netdata-conf-global.h
        src/daemon/config/netdata-conf.c
        src/daemon/daemon-shutdown.c
        src/daemon/daemon-shutdown.h
        src/daemon/daemon-service.c
        src/daemon/daemon-service.h
        src/daemon/pulse/pulse-db-rrd.c
        src/daemon/pulse/pulse-db-rrd.h
        src/daemon/config/netdata-conf-cloud.c
        src/daemon/config/netdata-conf-cloud.h
        src/daemon/config/netdata-conf-profile.c
        src/daemon/config/netdata-conf-profile.h
        src/daemon/pulse/pulse-daemon-memory-system.c
        src/daemon/pulse/pulse-network.c
        src/daemon/pulse/pulse-network.h
        src/daemon/pulse/pulse-db-dbengine-retention.c
        src/daemon/pulse/pulse-db-dbengine-retention.h
        src/daemon/pulse/pulse-parents.c
        src/daemon/pulse/pulse-parents.h
        src/daemon/status-file.c
        src/daemon/status-file.h
        src/daemon/config/netdata-conf-ssl.c
        src/daemon/config/netdata-conf-ssl.h
        src/daemon/machine-guid.c
        src/daemon/machine-guid.h
        src/daemon/status-file-dedup.c
        src/daemon/status-file-dedup.h
        src/daemon/status-file-io.c
        src/daemon/status-file-io.h
        src/daemon/status-file-dmi.c
        src/daemon/status-file-dmi.h
        src/daemon/status-file-product.c
        src/daemon/status-file-product.h
        src/daemon/protected-access.c
        src/daemon/protected-access.h
)

set(DAEMON_SYSTEMD_WATCHER_FILES
    src/daemon/daemon-systemd-watcher.c
    src/daemon/daemon-systemd-watcher.h
)

if(ENABLE_SYSTEMD_DBUS)
        list(APPEND DAEMON_FILES ${DAEMON_SYSTEMD_WATCHER_FILES})
endif()

set(API_PLUGIN_FILES
        src/web/api/web_api.c
        src/web/api/web_api.h
        src/web/api/web_api_v1.c
        src/web/api/web_api_v1.h
        src/web/api/web_api_v2.c
        src/web/api/web_api_v2.h
        src/web/api/web_api_v3.c
        src/web/api/web_api_v3.h
        src/web/api/http_auth.c
        src/web/api/http_auth.h
        src/web/api/http_header.c
        src/web/api/http_header.h
        src/web/api/maps/rrdr_options.c
        src/web/api/maps/rrdr_options.h
        src/web/api/maps/contexts_options.c
        src/web/api/maps/contexts_options.h
        src/web/api/maps/datasource_formats.c
        src/web/api/maps/datasource_formats.h
        src/web/api/maps/maps.h
        src/web/api/maps/contexts_alert_statuses.c
        src/web/api/maps/contexts_alert_statuses.h
        src/web/api/v1/api_v1_allmetrics.c
        src/web/api/v1/api_v1_badge/web_buffer_svg.c
        src/web/api/v1/api_v1_function.c
        src/web/api/v1/api_v1_manage.c
        src/web/api/v1/api_v1_calls.h
        src/web/api/v1/api_v1_dbengine.c
        src/web/api/v1/api_v1_config.c
        src/web/api/v1/api_v1_functions.c
        src/web/api/v1/api_v1_weights.c
        src/web/api/v1/api_v1_info.c
        src/web/api/v1/api_v1_registry.c
        src/web/api/v1/api_v1_data.c
        src/web/api/v1/api_v1_contexts.c
        src/web/api/v1/api_v1_ml_info.c
        src/web/api/v1/api_v1_aclk.c
        src/web/api/v1/api_v1_context.c
        src/web/api/v1/api_v1_alarms.c
        src/web/api/v1/api_v1_charts.c
        src/web/api/v2/api_v2_info.c
        src/web/api/v2/api_v2_nodes.c
        src/web/api/v2/api_v2_node_instances.c
        src/web/api/v2/api_v2_q.c
        src/web/api/v2/api_v2_versions.c
        src/web/api/v2/api_v2_functions.c
        src/web/api/v2/api_v2_alerts.c
        src/web/api/v2/api_v2_alert_transitions.c
        src/web/api/v2/api_v2_bearer.c
        src/web/api/v2/api_v2_calls.h
        src/web/api/v2/api_v2_data.c
        src/web/api/v2/api_v2_progress.c
        src/web/api/v2/api_v2_weights.c
        src/web/api/v2/api_v2_alert_config.c
        src/web/api/v2/api_v2_contexts.c
        src/web/api/v2/api_v2_claim.c
        src/web/api/v2/api_v2_webrtc.c
        src/web/api/v3/api_v3_calls.h
        src/web/api/v3/api_v3_settings.c
        src/web/api/functions/functions.c
        src/web/api/functions/functions.h
        src/web/api/functions/function-progress.c
        src/web/api/functions/function-progress.h
        src/web/api/functions/function-streaming.c
        src/web/api/functions/function-streaming.h
        src/web/api/queries/rrdr.c
        src/web/api/queries/rrdr.h
        src/web/api/queries/query.c
        src/web/api/queries/query.h
        src/web/api/queries/query-group-by.c
        src/web/api/queries/query-group-over-time.c
        src/web/api/queries/query-internal.h
        src/web/api/queries/query-plan.c
        src/web/api/queries/average/average.c
        src/web/api/queries/average/average.h
        src/web/api/queries/countif/countif.c
        src/web/api/queries/countif/countif.h
        src/web/api/queries/incremental_sum/incremental_sum.c
        src/web/api/queries/incremental_sum/incremental_sum.h
        src/web/api/queries/max/max.c
        src/web/api/queries/max/max.h
        src/web/api/queries/min/min.c
        src/web/api/queries/min/min.h
        src/web/api/queries/sum/sum.c
        src/web/api/queries/sum/sum.h
        src/web/api/queries/median/median.c
        src/web/api/queries/median/median.h
        src/web/api/queries/percentile/percentile.c
        src/web/api/queries/percentile/percentile.h
        src/web/api/queries/stddev/stddev.c
        src/web/api/queries/stddev/stddev.h
        src/web/api/queries/ses/ses.c
        src/web/api/queries/ses/ses.h
        src/web/api/queries/des/des.c
        src/web/api/queries/des/des.h
        src/web/api/queries/trimmed_mean/trimmed_mean.c
        src/web/api/queries/trimmed_mean/trimmed_mean.h
        src/web/api/queries/extremes/extremes.c
        src/web/api/queries/extremes/extremes.h
        src/web/api/queries/weights.c
        src/web/api/queries/weights.h
        src/web/api/formatters/rrd2json.c
        src/web/api/formatters/rrd2json.h
        src/web/api/formatters/csv/csv.c
        src/web/api/formatters/csv/csv.h
        src/web/api/formatters/json/json.c
        src/web/api/formatters/json/json.h
        src/web/api/formatters/ssv/ssv.c
        src/web/api/formatters/ssv/ssv.h
        src/web/api/formatters/value/value.c
        src/web/api/formatters/value/value.h
        src/web/api/formatters/jsonwrap.c
        src/web/api/formatters/jsonwrap.h
        src/web/api/formatters/jsonwrap-internal.h
        src/web/api/formatters/jsonwrap-objects-tree.c
        src/web/api/formatters/jsonwrap-query-plan.c
        src/web/api/formatters/jsonwrap-summary-alerts.c
        src/web/api/formatters/jsonwrap-summary-contexts.c
        src/web/api/formatters/jsonwrap-summary-dimensions.c
        src/web/api/formatters/jsonwrap-summary-instances.c
        src/web/api/formatters/jsonwrap-summary-labels.c
        src/web/api/formatters/jsonwrap-summary-nodes.c
        src/web/api/formatters/jsonwrap-v1.c
        src/web/api/formatters/jsonwrap-v2.c
        src/web/api/formatters/charts2json.c
        src/web/api/formatters/charts2json.h
        src/web/api/formatters/rrdset2json.c
        src/web/api/formatters/rrdset2json.h
        src/web/rtc/webrtc.c
        src/web/rtc/webrtc.h
        src/web/api/functions/function-bearer_get_token.c
        src/web/api/functions/function-bearer_get_token.h
        src/web/api/v3/api_v3_me.c
)

set(EXPORTING_ENGINE_FILES
        src/exporting/exporting_engine.c
        src/exporting/exporting_engine.h
        src/exporting/graphite/graphite.c
        src/exporting/graphite/graphite.h
        src/exporting/json/json.c
        src/exporting/json/json.h
        src/exporting/opentsdb/opentsdb.c
        src/exporting/opentsdb/opentsdb.h
        src/exporting/prometheus/prometheus.c
        src/exporting/prometheus/prometheus.h
        src/exporting/read_config.c
        src/exporting/clean_connectors.c
        src/exporting/init_connectors.c
        src/exporting/process_data.c
        src/exporting/check_filters.c
        src/exporting/send_data.c
        src/exporting/send_internal_metrics.c
)

set(HEALTH_PLUGIN_FILES
        src/health/health.c
        src/health/health.h
        src/health/health_config.c
        src/health/health_json.c
        src/health/health_log.c
        src/health/health_prototypes.c
        src/health/health_prototypes.h
        src/health/health_silencers.c
        src/health/health_silencers.h
        src/health/health_internals.h
        src/health/health_notifications.c
        src/health/health_event_loop.c
        src/health/health_dyncfg.c
        src/health/health_variable.c
        src/health/rrdcalc.c
        src/health/rrdcalc.h
        src/health/rrdvar.c
        src/health/rrdvar.h
        src/health/health-alert-entry.h
        src/health/health-alert-log.h
)

set(IDLEJITTER_PLUGIN_FILES src/collectors/idlejitter.plugin/plugin_idlejitter.c)

if(ENABLE_ML)
        set(ML_FILES
                src/ml/ad_charts.h
                src/ml/ad_charts.cc
                src/ml/ml.cc
                src/ml/ml_calculated_number.h
                src/ml/ml_host.h
                src/ml/ml_config.h
                src/ml/ml_config.cc
                src/ml/ml_dimension.h
                src/ml/ml_enums.h
                src/ml/ml_enums.cc
                src/ml/ml_features.h
                src/ml/ml_features.cc
                src/ml/ml_kmeans.h
                src/ml/ml_kmeans.cc
                src/ml/ml_queue.h
                src/ml/ml_worker.h
                src/ml/ml_string_wrapper.h
                src/ml/ml_queue.cc
                src/ml/ml_private.h
                src/ml/ml_public.h
                src/ml/ml_public.cc
        )

        if(NOT ENABLE_MIMALLOC)
                list(APPEND ML_FILES src/ml/ml_memory.cc)
        endif()
else()
        set(ML_FILES
                src/ml/ml_public.h
                src/ml/ml-dummy.c
        )
endif()

set(INTERNAL_COLLECTORS_FILES
        src/collectors/common-contexts/common-contexts.h
        src/collectors/common-contexts/disk-await.h
        src/collectors/common-contexts/disk-avgsz.h
        src/collectors/common-contexts/disk-busy.h
        src/collectors/common-contexts/disk-io.h
        src/collectors/common-contexts/disk-iotime.h
        src/collectors/common-contexts/disk-ops.h
        src/collectors/common-contexts/disk-qops.h
        src/collectors/common-contexts/disk-svctm.h
        src/collectors/common-contexts/disk-util.h
        src/collectors/common-contexts/system-io.h
        src/collectors/common-contexts/system-interrupts.h
        src/collectors/common-contexts/system-processes.h
        src/collectors/common-contexts/system-ram.h
        src/collectors/common-contexts/mem-swap.h
        src/collectors/common-contexts/mem-pgfaults.h
        src/collectors/common-contexts/mem-available.h
        src/collectors/common-contexts/power-supply.h
)

set(INTERCOMMUNICATION_COLLECTORS_FILES
        src/collectors/collectors-ipc/ebpf-ipc.c
        src/collectors/collectors-ipc/ebpf-ipc.h
        )

set(PLUGINSD_PLUGIN_FILES
        src/plugins.d/plugins_d.c
        src/plugins.d/plugins_d.h
        src/plugins.d/pluginsd_dyncfg.c
        src/plugins.d/pluginsd_dyncfg.h
        src/plugins.d/pluginsd_functions.c
        src/plugins.d/pluginsd_functions.h
        src/plugins.d/pluginsd_internals.c
        src/plugins.d/pluginsd_internals.h
        src/plugins.d/pluginsd_parser.c
        src/plugins.d/pluginsd_parser.h
        src/plugins.d/pluginsd_replication.c
        src/plugins.d/pluginsd_replication.h
)

set(RRD_PLUGIN_FILES
        src/database/contexts/api_v1_contexts.c
        src/database/contexts/api_v2_contexts.c
        src/database/contexts/api_v2_contexts.h
        src/database/contexts/api_v2_contexts_agents.c
        src/database/contexts/api_v2_contexts_alerts.c
        src/database/contexts/api_v2_contexts_alerts.h
        src/database/contexts/api_v2_contexts_alert_transitions.c
        src/database/contexts/api_v2_contexts_alert_config.c
        src/database/contexts/rrdcontext-context.c
        src/database/contexts/rrdcontext-instance.c
        src/database/contexts/rrdcontext-internal.h
        src/database/contexts/rrdcontext-metric.c
        src/database/contexts/query_scope.c
        src/database/contexts/query_target.c
        src/database/contexts/rrdcontext.c
        src/database/contexts/rrdcontext.h
        src/database/contexts/rrdcontext-worker.c
        src/database/rrdcollector.c
        src/database/rrdcollector.h
        src/database/rrddim.c
        src/database/rrdfunctions.c
        src/database/rrdfunctions.h
        src/database/rrdfunctions-inline.c
        src/database/rrdfunctions-inline.h
        src/database/rrdhost.c
        src/database/rrdlabels.c
        src/database/rrdlabels-aggregated.c
        src/database/rrdlabels-aggregated.h
        src/database/rrd.c
        src/database/rrd.h
        src/database/rrd-metadata.c
        src/database/rrd-metadata.h
        src/database/rrd-retention.c
        src/database/rrd-retention.h
        src/database/rrdset.c
        src/database/storage-engine.c
        src/database/storage-engine.h
        src/database/ram/rrddim_mem.c
        src/database/ram/rrddim_mem.h
        src/database/sqlite/sqlite_metadata.c
        src/database/sqlite/sqlite_metadata.h
        src/database/sqlite/sqlite_functions.c
        src/database/sqlite/sqlite_functions.h
        src/database/sqlite/sqlite_context.c
        src/database/sqlite/sqlite_context.h
        src/database/sqlite/sqlite_db_migration.c
        src/database/sqlite/sqlite_db_migration.h
        src/database/sqlite/sqlite_aclk.c
        src/database/sqlite/sqlite_aclk.h
        src/database/sqlite/sqlite_health.c
        src/database/sqlite/sqlite_health.h
        src/database/sqlite/sqlite_aclk_node.c
        src/database/sqlite/sqlite_aclk_node.h
        src/database/sqlite/sqlite_aclk_alert.c
        src/database/sqlite/sqlite_aclk_alert.h
        src/database/sqlite/vendored/sqlite3.c
        src/database/sqlite/vendored/sqlite3.h
        src/database/sqlite/vendored/sqlite3recover.c
        src/database/sqlite/vendored/sqlite3recover.h
        src/database/sqlite/vendored/dbdata.c
        src/web/api/queries/KolmogorovSmirnovDist.c
        src/web/api/queries/KolmogorovSmirnovDist.h
        src/database/rrdfunctions-inflight.c
        src/database/rrdfunctions-inflight.h
        src/database/rrdfunctions-exporters.c
        src/database/rrdfunctions-exporters.h
        src/database/rrdfunctions-internals.h
        src/database/rrdcollector-internals.h
        src/database/rrd-database-mode.h
        src/database/rrd-database-mode.c
        src/database/rrdhost-system-info.c
        src/database/rrdhost-system-info.h
        src/database/contexts/rrdcontext-loading.c
        src/database/rrdset-index-id.c
        src/database/rrdset-index-id.h
        src/database/rrdset-index-name.c
        src/database/rrdset-index-name.h
        src/database/rrdset-slots.c
        src/database/rrdset-slots.h
        src/database/rrdset-collection.c
        src/database/rrdset-collection.h
        src/database/rrdset.h
        src/database/rrdhost.h
        src/database/rrdhost-status.c
        src/database/rrdhost-status.h
        src/database/rrddim.h
        src/database/rrddim-backfill.c
        src/database/rrddim-backfill.h
        src/database/rrddim-collection.c
        src/database/rrddim-collection.h
        src/database/rrdset-type.c
        src/database/rrdset-type.h
        src/database/rrdhost-slots.c
        src/database/rrdhost-slots.h
        src/database/rrd-algorithm.c
        src/database/rrd-algorithm.h
        src/database/rrdhost-labels.c
        src/database/rrdhost-labels.h
        src/database/rrdhost-collection.c
        src/database/rrdhost-collection.h
        src/database/pattern-array.c
        src/database/pattern-array.h
        src/database/contexts/rrdcontext-queues.c
        src/database/contexts/rrdcontext-context-registry.c
        src/database/contexts/rrdcontext-context-registry.h
)

if(ENABLE_DBENGINE)
    list(APPEND RRD_PLUGIN_FILES
            src/database/engine/rrdengine.c
            src/database/engine/rrdengine.h
            src/database/engine/rrddiskprotocol.h
            src/database/engine/datafile.c
            src/database/engine/datafile.h
            src/database/engine/journalfile.c
            src/database/engine/journalfile.h
            src/database/engine/rrdenginelib.c
            src/database/engine/rrdenginelib.h
            src/database/engine/rrdengineapi.c
            src/database/engine/rrdengineapi.h
            src/database/engine/pagecache.c
            src/database/engine/pagecache.h
            src/database/engine/page_test.cc
            src/database/engine/page.c
            src/database/engine/page.h
            src/database/engine/cache.c
            src/database/engine/cache.h
            src/database/engine/mrg.c
            src/database/engine/mrg.h
            src/database/engine/mrg-internals.h
            src/database/engine/mrg-unittest.c
            src/database/engine/mrg-load.c
            src/database/engine/pdc.c
            src/database/engine/pdc.h
            src/database/engine/dbengine-unittest.c
            src/database/engine/dbengine-stresstest.c
            src/database/engine/dbengine-compression.c
            src/database/engine/dbengine-compression.h
    )
endif()

set(REGISTRY_PLUGIN_FILES
        src/registry/registry.c
        src/registry/registry.h
        src/registry/registry_db.c
        src/registry/registry_init.c
        src/registry/registry_internals.c
        src/registry/registry_internals.h
        src/registry/registry_log.c
        src/registry/registry_machine.c
        src/registry/registry_machine.h
        src/registry/registry_person.c
        src/registry/registry_person.h
)

set(STATSD_PLUGIN_FILES
        src/collectors/statsd.plugin/statsd.c
)

set(SYSTEMD_JOURNAL_PLUGIN_FILES
        src/collectors/systemd-journal.plugin/systemd-journal.c
        src/collectors/systemd-journal.plugin/systemd-journal-fstat.c
        src/collectors/systemd-journal.plugin/systemd-internals.h
        src/collectors/systemd-journal.plugin/systemd-main.c
        src/collectors/systemd-journal.plugin/systemd-journal.c
        src/collectors/systemd-journal.plugin/systemd-journal-annotations.c
        src/collectors/systemd-journal.plugin/systemd-journal-files.c
        src/collectors/systemd-journal.plugin/systemd-journal-watcher.c
        src/collectors/systemd-journal.plugin/systemd-journal-dyncfg.c
        src/collectors/systemd-journal.plugin/provider/netdata_provider.c
        src/collectors/systemd-journal.plugin/provider/netdata_provider.h
        src/collectors/systemd-journal.plugin/provider/rust_provider.h
        src/libnetdata/os/system-maps/system-services.h
        src/collectors/systemd-journal.plugin/systemd-journal-sampling.h
)

set(SYSTEMD_UNITS_PLUGIN_FILES
        src/collectors/systemd-units.plugin/plugin_systemd_units.c
)

set(STREAMING_PLUGIN_FILES
        src/streaming/stream.h
        src/streaming/stream-compression/compression.c
        src/streaming/stream-compression/compression.h
        src/streaming/stream-compression/brotli.c
        src/streaming/stream-compression/brotli.h
        src/streaming/stream-compression/gzip.c
        src/streaming/stream-compression/gzip.h
        src/streaming/stream-compression/lz4.c
        src/streaming/stream-compression/lz4.h
        src/streaming/stream-compression/zstd.c
        src/streaming/stream-compression/zstd.h
        src/streaming/stream-receiver.c
        src/streaming/stream-sender.c
        src/streaming/stream-replication-sender.c
        src/streaming/stream-replication-sender.h
        src/streaming/protocol/command-nodeid.c
        src/streaming/protocol/commands.c
        src/streaming/protocol/commands.h
        src/streaming/protocol/command-claimed_id.c
        src/streaming/stream-path.c
        src/streaming/stream-path.h
        src/streaming/stream-capabilities.c
        src/streaming/stream-capabilities.h
        src/streaming/stream-connector.c
        src/streaming/stream-sender-internals.h
        src/streaming/stream-sender-execute.c
        src/streaming/stream-sender-commit.c
        src/streaming/stream-parents.c
        src/streaming/stream-handshake.c
        src/streaming/protocol/command-function.c
        src/streaming/protocol/command-host-labels.c
        src/streaming/protocol/command-chart-definition.c
        src/streaming/protocol/command-begin-set-end-v2.c
        src/streaming/protocol/command-host-variables.c
        src/streaming/stream-conf.c
        src/streaming/stream-conf.h
        src/streaming/stream-handshake.h
        src/streaming/stream-parents.h
        src/streaming/stream-sender-api.c
        src/streaming/stream-receiver-internals.h
        src/streaming/stream-receiver-api.c
        src/streaming/stream-thread.c
        src/streaming/stream-thread.h
        src/streaming/stream-receiver-connection.c
        src/streaming/stream-sender-commit.h
        src/streaming/stream-traffic-types.h
        src/streaming/stream-circular-buffer.c
        src/streaming/stream-circular-buffer.h
        src/streaming/stream-control.c
        src/streaming/stream-control.h
        src/streaming/stream-waiting-list.c
        src/streaming/stream-waiting-list.h
        src/streaming/stream-replication-receiver.c
        src/streaming/stream-replication-receiver.h
        src/streaming/stream-replication-tracking.c
        src/streaming/stream-replication-tracking.h
        src/streaming/protocol/command-begin-set-end-v1.c
        src/streaming/protocol/command-begin-set-end-init.c
)

set(WEB_PLUGIN_FILES
        src/web/api/functions/function-metrics-cardinality.c
        src/web/api/functions/function-metrics-cardinality.h
        src/web/api/queries/backfill.c
        src/web/api/queries/backfill.h
        src/web/api/v3/api_v3_stream_info.c
        src/web/api/v3/api_v3_stream_path.c
        src/web/mcp/adapters/mcp-websocket.c
        src/web/mcp/adapters/mcp-websocket.h
        src/web/mcp/mcp-initialize.c
        src/web/mcp/mcp-initialize.h
        src/web/mcp/mcp-prompts.c
        src/web/mcp/mcp-prompts.h
        src/web/mcp/mcp-resources.c
        src/web/mcp/mcp-resources.h
        src/web/mcp/mcp-tools.c
        src/web/mcp/mcp-tools.h
        src/web/mcp/mcp-tools-list-metadata.c
        src/web/mcp/mcp-tools-list-metadata.h
        src/web/mcp/mcp-tools-execute-function.c
        src/web/mcp/mcp-tools-execute-function.h
        src/web/mcp/mcp-tools-execute-function-internal.h
        src/web/mcp/mcp-tools-execute-function-registry.c
        src/web/mcp/mcp-tools-execute-function-registry.h
        src/web/mcp/mcp-tools-query-metrics.c
        src/web/mcp/mcp-tools-query-metrics.h
        src/web/mcp/mcp-tools-weights.c
        src/web/mcp/mcp-tools-weights.h
        src/web/mcp/mcp-tools-alert-transitions.c
        src/web/mcp/mcp-tools-alert-transitions.h
        src/web/mcp/mcp-tools-configured-alerts.c
        src/web/mcp/mcp-tools-configured-alerts.h
        src/web/mcp/mcp-params.c
        src/web/mcp/mcp-params.h
        src/web/mcp/mcp-request-id.c
        src/web/mcp/mcp-request-id.h
        src/web/mcp/mcp-ping.c
        src/web/mcp/mcp-ping.h
        src/web/mcp/mcp-logging.c
        src/web/mcp/mcp-logging.h
        src/web/mcp/mcp-completion.c
        src/web/mcp/mcp-completion.h
        src/web/mcp/mcp-api-key.c
        src/web/mcp/mcp-api-key.h
        src/web/mcp/mcp.c
        src/web/mcp/mcp.h
        src/web/server/static/static-threaded.c
        src/web/server/static/static-threaded.h
        src/web/server/web_client.c
        src/web/server/web_client.h
        src/web/server/web_client_cache.c
        src/web/server/web_client_cache.h
        src/web/server/web_server.c
        src/web/server/web_server.h
        src/web/websocket/websocket-buffer.h
        src/web/websocket/websocket-compression.c
        src/web/websocket/websocket-compression.h
        src/web/websocket/websocket-echo.c
        src/web/websocket/websocket-echo.h
        src/web/websocket/websocket-handshake.c
        src/web/websocket/websocket-internal.h
        src/web/websocket/websocket-jsonrpc.c
        src/web/websocket/websocket-jsonrpc.h
        src/web/websocket/websocket-message.c
        src/web/websocket/websocket-receive.c
        src/web/websocket/websocket-send.c
        src/web/websocket/websocket-thread.c
        src/web/websocket/websocket-thread.h
        src/web/websocket/websocket-utils.c
        src/web/websocket/websocket.c
        src/web/websocket/websocket.h
)

set(CLAIM_PLUGIN_FILES
        src/claim/claim.c
        src/claim/claim.h
        src/claim/claim_id.c
        src/claim/claim_id.h
        src/claim/cloud-conf.c
        src/claim/claim-with-api.c
        src/claim/cloud-status.c
        src/claim/cloud-status.h
)

set(CLAIM_WINDOWS_FILES
        src/claim/main.c
        src/claim/main.h
        src/claim/ui.c
        src/claim/ui.h
)

set(ACLK_ALWAYS_BUILD
        src/aclk/aclk_proxy.c
        src/aclk/aclk_proxy.h
        src/aclk/aclk.c
        src/aclk/aclk.h
        src/aclk/aclk_capas.c
        src/aclk/aclk_capas.h
        src/aclk/aclk_util.c
        src/aclk/aclk_util.h
        src/aclk/https_client.c
        src/aclk/https_client.h
        src/libnetdata/c_rhash/c_rhash.c
        src/libnetdata/c_rhash/c_rhash.h
        src/libnetdata/c_rhash/c_rhash_internal.h
)

set(TIMEX_PLUGIN_FILES
        src/collectors/timex.plugin/plugin_timex.c
)

set(PROFILE_PLUGIN_FILES
        src/collectors/profile.plugin/plugin_profile.cc
)

set(CGROUPS_PLUGIN_FILES
        src/collectors/cgroups.plugin/sys_fs_cgroup.c
        src/collectors/cgroups.plugin/sys_fs_cgroup.h
        src/collectors/cgroups.plugin/cgroup-internals.h
        src/collectors/cgroups.plugin/cgroup-discovery.c
        src/collectors/cgroups.plugin/cgroup-charts.c
        src/collectors/cgroups.plugin/cgroup-top.c
)

set(DISKSPACE_PLUGIN_FILES
        src/collectors/diskspace.plugin/plugin_diskspace.c
)

set(MACOS_PLUGIN_FILES
        src/collectors/macos.plugin/plugin_macos.c
        src/collectors/macos.plugin/plugin_macos.h
        src/collectors/macos.plugin/macos_sysctl.c
        src/collectors/macos.plugin/macos_mach_smi.c
        src/collectors/macos.plugin/macos_fw.c
)

set(FREEBSD_PLUGIN_FILES
        src/collectors/freebsd.plugin/plugin_freebsd.c
        src/collectors/freebsd.plugin/plugin_freebsd.h
        src/collectors/freebsd.plugin/freebsd_sysctl.c
        src/collectors/freebsd.plugin/freebsd_getmntinfo.c
        src/collectors/freebsd.plugin/freebsd_getifaddrs.c
        src/collectors/freebsd.plugin/freebsd_devstat.c
        src/collectors/freebsd.plugin/freebsd_kstat_zfs.c
        src/collectors/freebsd.plugin/freebsd_ipfw.c
        src/collectors/proc.plugin/zfs_common.c
        src/collectors/proc.plugin/zfs_common.h
)

set(WINDOWS_EVENTS_PLUGIN_FILES
        src/collectors/windows-events.plugin/windows-events.c
        src/collectors/windows-events.plugin/windows-events.h
        src/collectors/windows-events.plugin/windows-events-query.h
        src/collectors/windows-events.plugin/windows-events-query.c
        src/collectors/windows-events.plugin/windows-events-sources.c
        src/collectors/windows-events.plugin/windows-events-sources.h
        src/collectors/windows-events.plugin/windows-events-unicode.c
        src/collectors/windows-events.plugin/windows-events-unicode.h
        src/collectors/windows-events.plugin/windows-events-xml.c
        src/collectors/windows-events.plugin/windows-events-xml.h
        src/collectors/windows-events.plugin/windows-events-providers.c
        src/collectors/windows-events.plugin/windows-events-providers.h
        src/collectors/windows-events.plugin/windows-events-fields-cache.c
        src/collectors/windows-events.plugin/windows-events-fields-cache.h
        src/collectors/windows-events.plugin/windows-events-query-builder.c
        src/collectors/windows-events.plugin/windows-events-query-builder.h
        src/collectors/windows-events.plugin/windows-events-query-evt-variant.c
)

set(WINDOWS_PLUGIN_FILES
        src/collectors/windows.plugin/windows_plugin.c
        src/collectors/windows.plugin/windows_plugin.h
        src/collectors/windows.plugin/GetSystemUptime.c
        src/collectors/windows.plugin/GetSystemRAM.c
        src/collectors/windows.plugin/GetSystemCPU.c
        src/collectors/windows.plugin/GetPowerSupply.c
        src/collectors/windows.plugin/GetServicesStatus.c
        src/collectors/windows.plugin/GetSensors.c
        src/collectors/windows.plugin/perflib-adcs.c
        src/collectors/windows.plugin/perflib-adfs.c
        src/collectors/windows.plugin/perflib-rrd.c
        src/collectors/windows.plugin/perflib-rrd.h
        src/collectors/windows.plugin/perflib-ad.c
        src/collectors/windows.plugin/perflib-mssql.c
        src/collectors/windows.plugin/perflib-mssql-queries.h
        src/collectors/windows.plugin/perflib-storage.c
        src/collectors/windows.plugin/perflib-processor.c
        src/collectors/windows.plugin/perflib-thermalzone.c
        src/collectors/windows.plugin/perflib-objects.c
        src/collectors/windows.plugin/perflib-network.c
        src/collectors/windows.plugin/perflib-netframework.c
        src/collectors/windows.plugin/perflib-memory.c
        src/collectors/windows.plugin/perflib-processes.c
        src/collectors/windows.plugin/perflib-web-service.c
        src/collectors/windows.plugin/perflib-hyperv.c
        src/collectors/windows.plugin/perflib-exchange.c
        src/collectors/windows.plugin/perflib-numa.c
        src/collectors/windows.plugin/perflib-asp.c
)

set(PROC_PLUGIN_FILES
        src/collectors/proc.plugin/ipc.c
        src/collectors/proc.plugin/plugin_proc.c
        src/collectors/proc.plugin/plugin_proc.h
        src/collectors/proc.plugin/proc_sys_fs_file_nr.c
        src/collectors/proc.plugin/proc_diskstats.c
        src/collectors/proc.plugin/proc_mdstat.c
        src/collectors/proc.plugin/proc_interrupts.c
        src/collectors/proc.plugin/proc_softirqs.c
        src/collectors/proc.plugin/proc_loadavg.c
        src/collectors/proc.plugin/proc_meminfo.c
        src/collectors/proc.plugin/proc_pagetypeinfo.c
        src/collectors/proc.plugin/proc_net_dev.c
        src/collectors/proc.plugin/proc_net_dev_renames.c
        src/collectors/proc.plugin/proc_net_dev_renames.h
        src/collectors/proc.plugin/proc_net_wireless.c
        src/collectors/proc.plugin/proc_net_ip_vs_stats.c
        src/collectors/proc.plugin/proc_net_netstat.c
        src/collectors/proc.plugin/proc_net_rpc_nfs.c
        src/collectors/proc.plugin/proc_net_rpc_nfsd.c
        src/collectors/proc.plugin/proc_net_sctp_snmp.c
        src/collectors/proc.plugin/proc_net_sockstat.c
        src/collectors/proc.plugin/proc_net_sockstat6.c
        src/collectors/proc.plugin/proc_net_softnet_stat.c
        src/collectors/proc.plugin/proc_net_stat_conntrack.c
        src/collectors/proc.plugin/proc_net_stat_synproxy.c
        src/collectors/proc.plugin/proc_self_mountinfo.c
        src/collectors/proc.plugin/proc_self_mountinfo.h
        src/collectors/proc.plugin/zfs_common.c
        src/collectors/proc.plugin/zfs_common.h
        src/collectors/proc.plugin/proc_spl_kstat_zfs.c
        src/collectors/proc.plugin/proc_stat.c
        src/collectors/proc.plugin/proc_sys_kernel_random_entropy_avail.c
        src/collectors/proc.plugin/proc_vmstat.c
        src/collectors/proc.plugin/proc_uptime.c
        src/collectors/proc.plugin/run_reboot_required.c
        src/collectors/proc.plugin/proc_pressure.c
        src/collectors/proc.plugin/proc_pressure.h
        src/collectors/proc.plugin/sys_kernel_mm_ksm.c
        src/collectors/proc.plugin/sys_block_zram.c
        src/collectors/proc.plugin/sys_devices_system_edac_mc.c
        src/collectors/proc.plugin/sys_devices_system_node.c
        src/collectors/proc.plugin/sys_class_infiniband.c
        src/collectors/proc.plugin/sys_fs_btrfs.c
        src/collectors/proc.plugin/sys_class_power_supply.c
        src/collectors/proc.plugin/sys_devices_pci_aer.c
        src/collectors/proc.plugin/sys_class_drm.c
)

set(TC_PLUGIN_FILES
        src/collectors/tc.plugin/plugin_tc.c
)

set(NETDATA_FILES
        src/collectors/all.h
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
                src/daemon/static_threads_linux.c
                ${CGROUPS_PLUGIN_FILES}
                ${DISKSPACE_PLUGIN_FILES}
                ${PROC_PLUGIN_FILES}
                ${TC_PLUGIN_FILES}
                ${TIMEX_PLUGIN_FILES}
                ${INTERNAL_COLLECTORS_FILES}
        )

        if(ENABLE_SENTRY)
            list(APPEND NETDATA_FILES
                    src/daemon/sentry-native/sentry-native.c
                    src/daemon/sentry-native/sentry-native.h)
        endif()
elseif(OS_MACOS)
        list(APPEND NETDATA_FILES
                src/daemon/static_threads_macos.c
                ${MACOS_PLUGIN_FILES}
                ${TIMEX_PLUGIN_FILES}
                ${INTERNAL_COLLECTORS_FILES}
        )
elseif(OS_FREEBSD)
        list(APPEND NETDATA_FILES
                src/daemon/static_threads_freebsd.c
                ${FREEBSD_PLUGIN_FILES}
                ${TIMEX_PLUGIN_FILES}
                ${INTERNAL_COLLECTORS_FILES}
        )
elseif(OS_WINDOWS)
        list(APPEND NETDATA_FILES
                src/daemon/static_threads_windows.c
                src/daemon/winsvc.cc
                ${WINDOWS_PLUGIN_FILES}
                ${INTERNAL_COLLECTORS_FILES}
        )
endif()

set(MQTT_WEBSOCKETS_FILES
        src/aclk/mqtt_websockets/mqtt_wss_client.c
        src/aclk/mqtt_websockets/mqtt_wss_client.h
        src/aclk/mqtt_websockets/ws_client.c
        src/aclk/mqtt_websockets/ws_client.h
        src/aclk/mqtt_websockets/mqtt_ng.c
        src/aclk/mqtt_websockets/mqtt_ng.h
        src/aclk/mqtt_websockets/common_public.c
        src/aclk/mqtt_websockets/common_public.h
        src/aclk/mqtt_websockets/common_internal.h
        src/aclk/mqtt_websockets/aclk_mqtt_workers.h
)

set(ACLK_PROTO_DEFS
        src/aclk/aclk-schemas/proto/aclk/v1/lib.proto
        src/aclk/aclk-schemas/proto/agent/v1/disconnect.proto
        src/aclk/aclk-schemas/proto/agent/v1/connection.proto
        src/aclk/aclk-schemas/proto/alarm/v1/config.proto
        src/aclk/aclk-schemas/proto/alarm/v1/stream.proto
        src/aclk/aclk-schemas/proto/nodeinstance/connection/v1/connection.proto
        src/aclk/aclk-schemas/proto/nodeinstance/create/v1/creation.proto
        src/aclk/aclk-schemas/proto/nodeinstance/info/v1/info.proto
        src/aclk/aclk-schemas/proto/context/v1/context.proto
        src/aclk/aclk-schemas/proto/context/v1/stream.proto
        src/aclk/aclk-schemas/proto/agent/v1/cmds.proto
)

set(ACLK_FILES
        src/aclk/aclk_query.c
        src/aclk/aclk_query.h
        src/aclk/aclk_query_queue.c
        src/aclk/aclk_query_queue.h
        src/aclk/aclk_otp.c
        src/aclk/aclk_otp.h
        src/aclk/aclk_tx_msgs.c
        src/aclk/aclk_tx_msgs.h
        src/aclk/aclk_rx_msgs.c
        src/aclk/aclk_rx_msgs.h
        src/aclk/aclk_alarm_api.c
        src/aclk/aclk_alarm_api.h
        src/aclk/aclk_contexts_api.c
        src/aclk/aclk_contexts_api.h
        src/aclk/schema-wrappers/connection.cc
        src/aclk/schema-wrappers/connection.h
        src/aclk/schema-wrappers/node_connection.cc
        src/aclk/schema-wrappers/node_connection.h
        src/aclk/schema-wrappers/node_creation.cc
        src/aclk/schema-wrappers/node_creation.h
        src/aclk/schema-wrappers/alarm_stream.cc
        src/aclk/schema-wrappers/alarm_stream.h
        src/aclk/schema-wrappers/alarm_config.cc
        src/aclk/schema-wrappers/alarm_config.h
        src/aclk/schema-wrappers/node_info.cc
        src/aclk/schema-wrappers/node_info.h
        src/aclk/schema-wrappers/capability.cc
        src/aclk/schema-wrappers/capability.h
        src/aclk/schema-wrappers/proto_2_json.cc
        src/aclk/schema-wrappers/proto_2_json.h
        src/aclk/schema-wrappers/context_stream.cc
        src/aclk/schema-wrappers/context_stream.h
        src/aclk/schema-wrappers/context.cc
        src/aclk/schema-wrappers/rrdcontext-context.h
        src/aclk/schema-wrappers/schema_wrappers.h
        src/aclk/schema-wrappers/schema_wrapper_utils.cc
        src/aclk/schema-wrappers/schema_wrapper_utils.h
        src/aclk/schema-wrappers/agent_cmds.cc
        src/aclk/schema-wrappers/agent_cmds.h
)


set(MONGODB_EXPORTING_FILES
        src/exporting/mongodb/mongodb.c
        src/exporting/mongodb/mongodb.h
)

set(PROMETHEUS_REMOTE_WRITE_EXPORTING_FILES
        src/exporting/prometheus/remote_write/remote_write.c
        src/exporting/prometheus/remote_write/remote_write.h
        src/exporting/prometheus/remote_write/remote_write_request.cc
        src/exporting/prometheus/remote_write/remote_write_request.h
)

#
# build libjudy
#

add_library(judy STATIC ${LIBJUDY_SOURCES})

target_compile_options(judy PRIVATE
        -Wno-all -Wno-extra
        -Wno-shadow
        -Wformat
)

target_compile_definitions(judy PRIVATE
        JUDYL
        $<$<EQUAL:${CMAKE_SIZEOF_VOID_P},8>:JU_64BIT>
)

target_include_directories(judy PUBLIC
        src/libnetdata/libjudy/vendored
        src/libnetdata/libjudy/vendored/JudyCommon
)

set_source_files_properties(${LIBJUDY_PREV_FILES} PROPERTIES COMPILE_OPTIONS "-DJUDYPREV")
set_source_files_properties(${LIBJUDY_NEXT_FILES} PROPERTIES COMPILE_OPTIONS "-DJUDYNEXT")
set_source_files_properties(src/libnetdata/libjudy/vendored/JudyL/j__udyLGet.c PROPERTIES COMPILE_OPTIONS "-DJUDYGETINLINE")
set_source_files_properties(src/libnetdata/libjudy/vendored/JudyL/JudyLByCount.c PROPERTIES COMPILE_OPTIONS "-DNOSMARTJBB -DNOSMARTJBU -DNOSMARTJLB")
set_source_files_properties(JudyLTables.c PROPERTIES COMPILE_OPTIONS "-I${CMAKE_SOURCE_DIR}/src/libnetdata/libjudy/src/JudyL")

#
# build libnetdata
#

add_library(libnetdata STATIC ${LIBNETDATA_FILES})

target_include_directories(libnetdata BEFORE PUBLIC ${CONFIG_H_DIR} ${CMAKE_SOURCE_DIR}/src)

# pthread (FIXME: use find_package for this)

# set(CMAKE_THREAD_PREFER_PTHREAD TRUE)
# set(THREADS_PREFER_PTHREAD_FLAG TRUE)
# find_package(Threads REQUIRED)

# add_executable(test test.cpp)
# target_link_libraries(test Threads::Threads)

target_link_libraries(libnetdata PUBLIC
        "$<$<BOOL:${ENABLE_MIMALLOC}>:mimalloc-static>"
        "$<$<NOT:$<BOOL:${HAVE_BUILTIN_ATOMICS}>>:atomic>"
        "$<$<OR:$<BOOL:${OS_LINUX}>,$<BOOL:${OS_FREEBSD}>>:pthread;rt>"
        "$<$<BOOL:${OS_WINDOWS}>:kernel32;advapi32;winmm;rpcrt4;wevtapi;ole32;oleaut32;wbemuuid;sensorsapi>"
        "$<$<BOOL:${LINK_LIBM}>:m>"
        "${SYSTEMD_LDFLAGS}")

if(HAVE_LIBBACKTRACE)
    netdata_add_libbacktrace_to_target(libnetdata)
endif()

if(OS_WINDOWS)
        set(HAVE_ETW True)
        set(HAVE_WEL True)

        # Output the results for debugging purposes
        message(STATUS "Have Event Tracing for Windows (ETW): ${HAVE_ETW}")
        message(STATUS "Have Windows Event Log (WEL): ${HAVE_WEL}")

        if(HAVE_WEL OR HAVE_ETW)
                # Define the source and generated file paths
                set(WEVT_GEN_SRC_H_FILE "${CMAKE_SOURCE_DIR}/src/libnetdata/log/nd_log-to-windows-common.h")
                set(WEVT_GEN_SRC_C_FILE "${CMAKE_SOURCE_DIR}/src/libnetdata/log/wevt_netdata_mc_generate.c")
                set(WEVT_GEN_BIN_FILE "${CMAKE_BINARY_DIR}/wevt_netdata_mc_generate")

                set(WEVT_BUILD_SCRIPT "${CMAKE_SOURCE_DIR}/src/libnetdata/log/wevt_netdata_compile.sh")

                set(WEVT_MC_FILE "${CMAKE_BINARY_DIR}/wevt_netdata.mc")
                set(WEVT_MAN_FILE "${CMAKE_BINARY_DIR}/wevt_netdata_manifest.xml")
                set(WEVT_RC_FILE "${CMAKE_BINARY_DIR}/wevt_netdata.rc")
                set(WEVT_MC_H_FILE "${CMAKE_BINARY_DIR}/wevt_netdata.h")
                set(WEVT_MAN_H_FILE "${CMAKE_BINARY_DIR}/wevt_netdata_manifest.h")
                set(WEVT_RES_OBJECT "${CMAKE_BINARY_DIR}/wevt_netdata_res.o")

                set(WEVT_DLL_FILE "${CMAKE_BINARY_DIR}/wevt_netdata.dll")
                set(WEVT_ETW_INSTALL_SCRIPT "${CMAKE_SOURCE_DIR}/src/libnetdata/log/wevt_netdata_install.bat")

                # we compile ${WEVT_GEN_BIN_FILE}, which generates the manifest, the .mc,
                # and the headers required for compiling libnetdata/logs

                if(HAVE_ETW)
                        # ETW method also supports WEL
                        # but it requires Microsoft tools mc, rc, and link
                        add_custom_command(
                                OUTPUT "${WEVT_MC_H_FILE}" "${WEVT_MAN_H_FILE}" "${WEVT_DLL_FILE}"
                                COMMAND "${CMAKE_C_COMPILER}" -o "${WEVT_GEN_BIN_FILE}" "${WEVT_GEN_SRC_C_FILE}"
                                COMMAND "${WEVT_GEN_BIN_FILE}" >"${WEVT_MC_FILE}"
                                COMMAND "${WEVT_GEN_BIN_FILE}" --manifest >"${WEVT_MAN_FILE}"
                                COMMAND "${WEVT_BUILD_SCRIPT}" "${CMAKE_SOURCE_DIR}/src/libnetdata/log" "${CMAKE_BINARY_DIR}"
                                DEPENDS "${WEVT_GEN_SRC_C_FILE}" "${WEVT_GEN_SRC_H_FILE}"
                                COMMENT "Compiling ${WEVT_MC_FILE} to generate ${WEVT_MC_H_FILE} and ${WEVT_DLL_FILE}"
                        )
                else()
                        # WEL method can be built with windmc, windres and the normal linker
                        add_custom_command(
                                OUTPUT "${WEVT_MC_H_FILE}" "${WEVT_DLL_FILE}"
                                COMMAND "${CMAKE_C_COMPILER}" -o "${WEVT_GEN_BIN_FILE}" "${WEVT_GEN_SRC_C_FILE}"
                                COMMAND "${WEVT_GEN_BIN_FILE}" >"${WEVT_MC_FILE}"
                                COMMAND "${WEVT_GEN_BIN_FILE}" --manifest >"${WEVT_MAN_FILE}"
                                COMMAND windmc -r "${CMAKE_BINARY_DIR}" -h "${CMAKE_BINARY_DIR}" ${WEVT_MC_FILE}
                                COMMAND echo "1 2004" "wevt_netdata_manifest.xml" >> "${WEVT_RC_FILE}"
                                COMMAND windres ${WEVT_RC_FILE} -o ${WEVT_RES_OBJECT}
                                COMMAND ${CMAKE_LINKER} -dll --entry 0 -nostdlib -o ${WEVT_DLL_FILE} ${WEVT_RES_OBJECT}
                                DEPENDS "${WEVT_GEN_SRC_C_FILE}" "${WEVT_GEN_SRC_H_FILE}"
                                COMMENT "Compiling ${WEVT_MC_FILE} to generate ${WEVT_MC_H_FILE} and ${WEVT_DLL_FILE}"
                        )
                endif()

                # Create a custom target for the DLL
                add_custom_target(wevt_netdata ALL DEPENDS ${WEVT_DLL_FILE})

                set_source_files_properties(src/libnetdata/log/nd_log-to-windows-events.c PROPERTIES
                        OBJECT_DEPENDS "${WEVT_MC_H_FILE}")

                if(HAVE_ETW)
                        set_source_files_properties(src/libnetdata/log/nd_log-to-windows-events.c PROPERTIES
                                OBJECT_DEPENDS "${WEVT_MAN_H_FILE}")

                        install(FILES "${WEVT_DLL_FILE}" "${WEVT_MAN_FILE}" "${WEVT_ETW_INSTALL_SCRIPT}"
                                COMPONENT wevt_netdata_dll
                                DESTINATION "${BINDIR}")
                else()
                        # do not install the manifest in this case
                        # the nsi installer will skip registering the ETW publisher
                        install(FILES "${WEVT_DLL_FILE}"
                                COMPONENT wevt_netdata_dll
                                DESTINATION "${BINDIR}")
                endif()
        endif()
endif()

# judy
target_link_libraries(libnetdata PUBLIC judy)

netdata_add_jsonc_to_target(libnetdata)

netdata_add_libyaml_to_target(libnetdata)

# jemalloc
if(ENABLE_JEMALLOC)
    target_link_libraries(libnetdata PUBLIC ${JEMALLOC_LIBRARIES})
    target_include_directories(libnetdata PUBLIC ${JEMALLOC_INCLUDE_DIRS})
    target_compile_options(libnetdata PUBLIC ${JEMALLOC_CFLAGS_OTHER})
endif()

# libunwind
if(ENABLE_LIBUNWIND)
  pkg_check_modules(LIBUNWIND libunwind IMPORTED_TARGET)
  if(TARGET PkgConfig::LIBUNWIND)
    set(HAVE_LIBUNWIND On)

    if(STATIC_BUILD)
      set(unwind_prefix /libunwind-static/lib/libunwind-)
      set(suffix ${CMAKE_STATIC_LIBRARY_SUFFIX})
    else()
      set(unwind_prefix -lunwind-)
      set(suffix "")
    endif()

    if(CMAKE_SYSTEM_PROCESSOR MATCHES "(x86_64)|(amd64)")
      target_link_libraries(libnetdata PUBLIC PkgConfig::LIBUNWIND ${unwind_prefix}x86_64${suffix})
    elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "^i?.86")
      target_link_libraries(libnetdata PUBLIC PkgConfig::LIBUNWIND ${unwind_prefix}x86${suffix})
    elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "(arm64)|(aarch64)")
      target_link_libraries(libnetdata PUBLIC PkgConfig::LIBUNWIND ${unwind_prefix}aarch64${suffix})
    elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "arm")
      target_link_libraries(libnetdata PUBLIC PkgConfig::LIBUNWIND ${unwind_prefix}arm${suffix})
    elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "p(ower)?pc64")
      target_link_libraries(libnetdata PUBLIC PkgConfig::LIBUNWIND ${unwind_prefix}ppc64${suffix})
    else()
      message(WARNING "Unknown architecture ${CMAKE_SYSTEM_PROCESSOR} for libunwind. Stack traces may not work.")
      target_link_libraries(libnetdata PUBLIC PkgConfig::LIBUNWIND)
    endif()
  else()
    message(FATAL "Could not find libunwind for logging of stack traces")
  endif()
endif()

# zlib
if(OS_MACOS)
        find_package(ZLIB REQUIRED)
        target_include_directories(libnetdata BEFORE PUBLIC ${ZLIB_INCLUDE_DIRS})
        target_link_libraries(libnetdata PUBLIC ZLIB::ZLIB)
else()
        pkg_check_modules(ZLIB REQUIRED zlib)
        target_include_directories(libnetdata BEFORE PUBLIC ${ZLIB_INCLUDE_DIRS})
        target_compile_options(libnetdata PUBLIC ${ZLIB_CFLAGS_OTHER})
        target_link_libraries(libnetdata PUBLIC ${ZLIB_LDFLAGS})
endif()

# lz4 - try to find a version that is compatible with streaming compression
# otherwise pick whichever one we can find to support dbengine but don't set
# ENABLE_LZ4.
pkg_check_modules(LIBLZ4 liblz4>=1.9.0)
if(LIBLZ4_FOUND)
        set(ENABLE_LZ4 On)
else()
        pkg_check_modules(LIBLZ4 REQUIRED liblz4)
endif()

target_include_directories(libnetdata BEFORE PUBLIC ${LIBLZ4_INCLUDE_DIRS})
target_compile_options(libnetdata PUBLIC ${LIBLZ4_CFLAGS_OTHER})
target_link_libraries(libnetdata PUBLIC ${LIBLZ4_LDFLAGS})

# zstd
pkg_check_modules(LIBZSTD libzstd)
if(LIBZSTD_FOUND)
        set(ENABLE_ZSTD On)
        target_include_directories(libnetdata BEFORE PUBLIC ${LIBZSTD_INCLUDE_DIRS})
        target_compile_options(libnetdata PUBLIC ${LIBZSTD_CFLAGS_OTHER})
        target_link_libraries(libnetdata PUBLIC ${LIBZSTD_LDFLAGS})
endif()

# brotli
pkg_check_modules(LIBBROTLI libbrotlidec libbrotlienc libbrotlicommon)
if(LIBBROTLI_FOUND)
        set(ENABLE_BROTLI On)
        target_include_directories(libnetdata PUBLIC ${LIBBROTLI_INCLUDE_DIRS})
        target_compile_options(libnetdata PUBLIC ${LIBBROTLI_CFLAGS_OTHER})
        target_link_libraries(libnetdata PUBLIC ${LIBBROTLI_LDFLAGS})
endif()

# uuid
if(OS_MACOS OR OS_WINDOWS)
        # UUID functionality is part of the system libraries here, so no extra
        # stuff needed.
else()
        pkg_check_modules(UUID REQUIRED uuid)
        target_include_directories(libnetdata BEFORE PUBLIC ${UUID_INCLUDE_DIRS})
        target_compile_options(libnetdata PUBLIC ${UUID_CFLAGS_OTHER})
        target_link_libraries(libnetdata PUBLIC ${UUID_LDFLAGS})
endif()

# uv
pkg_check_modules(LIBUV REQUIRED libuv)
target_include_directories(libnetdata BEFORE PUBLIC ${LIBUV_INCLUDE_DIRS})
target_compile_options(libnetdata PUBLIC ${LIBUV_CFLAGS_OTHER})
target_link_libraries(libnetdata PUBLIC ${LIBUV_LDFLAGS})

# crypto
target_link_libraries(libnetdata PUBLIC PkgConfig::CRYPTO)

# openssl
target_link_libraries(libnetdata PUBLIC PkgConfig::TLS)

# mnl
if(NOT OS_MACOS)
    pkg_check_modules(MNL libmnl)
    if(MNL_FOUND)
        set(HAVE_LIBMNL True)
    endif()
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
netdata_protoc_generate_cpp("${CMAKE_SOURCE_DIR}/src/aclk/aclk-schemas"
                            "${CMAKE_BINARY_DIR}/src/aclk/aclk-schemas"
                            ACLK_PROTO_BUILT_SRCS
                            ACLK_PROTO_BUILT_HDRS
                            ${ACLK_PROTO_DEFS})

list(APPEND ACLK_FILES ${ACLK_PROTO_BUILT_SRCS}
                       ${ACLK_PROTO_BUILT_HDRS})

#
# build plugins
#

if(ENABLE_PLUGIN_DEBUGFS)
    # Check for libcap (optional)
    pkg_check_modules(CAP QUIET libcap)

    # Define debugfs.plugin source files
    set(DEBUGFS_PLUGIN_FILES
            src/collectors/debugfs.plugin/debugfs_plugin.c
            src/collectors/debugfs.plugin/debugfs_plugin.h
            src/collectors/debugfs.plugin/module-numa-extfrag.c
            src/collectors/debugfs.plugin/module-zswap.c
            src/collectors/debugfs.plugin/module-devices-powercap.c
            src/collectors/debugfs.plugin/module-libsensors.c
    )

    # Add executable for debugfs.plugin
    add_executable(debugfs.plugin ${DEBUGFS_PLUGIN_FILES})

    # Add vendored libsensors library
    add_subdirectory(src/collectors/debugfs.plugin/libsensors)

    # Link debugfs.plugin with vendored libsensors
    target_link_libraries(debugfs.plugin PRIVATE vendored_libsensors)

    # Include vendored libsensors headers
    target_include_directories(debugfs.plugin PRIVATE
            src/collectors/debugfs.plugin/libsensors/vendored/lib
            ${CMAKE_CURRENT_BINARY_DIR}/src/collectors/debugfs.plugin/libsensors # For generated headers
    )

    # Link against libnetdata and optionally libcap
    target_link_libraries(debugfs.plugin PRIVATE libnetdata ${CAP_LIBRARIES})
    target_include_directories(debugfs.plugin PRIVATE ${CAP_INCLUDE_DIRS})
    target_compile_options(debugfs.plugin PRIVATE ${CAP_CFLAGS_OTHER})

    # Install the debugfs.plugin binary
    install(TARGETS debugfs.plugin
            COMPONENT plugin-debugfs
            DESTINATION usr/libexec/netdata/plugins.d)

    install(FILES
            src/collectors/debugfs.plugin/libsensors/vendored/etc/sensors.conf.default
            COMPONENT plugin-debugfs
            DESTINATION usr/lib/netdata/conf.d
            RENAME sensors3.conf)

    # Install additional packaging files if building for packaging
    if(BUILD_FOR_PACKAGING)
        install(FILES
                ${PKG_FILES_PATH}/copyright
                COMPONENT plugin-debugfs
                DESTINATION usr/share/doc/netdata-plugin-debugfs)
    endif()
endif()

add_executable(spawn-tester src/libnetdata/spawn_server/spawn-tester.c)
target_link_libraries(spawn-tester libnetdata)

if(ENABLE_PLUGIN_APPS)
    pkg_check_modules(CAP QUIET libcap)

    set(APPS_PLUGIN_FILES
            src/collectors/apps.plugin/apps_plugin.c
            src/collectors/apps.plugin/apps_plugin.h
            src/collectors/apps.plugin/apps_functions.c
            src/collectors/apps.plugin/apps_targets.c
            src/collectors/apps.plugin/apps_output.c
            src/collectors/apps.plugin/apps_pid_files.c
            src/collectors/apps.plugin/apps_pid.c
            src/collectors/apps.plugin/apps_aggregations.c
            src/collectors/apps.plugin/apps_os_linux.c
            src/collectors/apps.plugin/apps_os_freebsd.c
            src/collectors/apps.plugin/apps_os_macos.c
            src/collectors/apps.plugin/apps_os_windows.c
            src/collectors/apps.plugin/apps_incremental_collection.c
            src/collectors/apps.plugin/apps_os_windows_nt.c
            src/collectors/apps.plugin/apps_pid_match.c
    )

    add_executable(apps.plugin ${APPS_PLUGIN_FILES})

    target_link_libraries(apps.plugin libnetdata ${CAP_LIBRARIES}
            "$<$<BOOL:${OS_WINDOWS}>:Version;ntdll>")

    target_include_directories(apps.plugin PRIVATE ${CAP_INCLUDE_DIRS})
    target_compile_options(apps.plugin PRIVATE ${CAP_CFLAGS_OTHER})

    install(TARGETS apps.plugin
            COMPONENT plugin-apps
            DESTINATION usr/libexec/netdata/plugins.d)

    install(FILES src/collectors/apps.plugin/apps_groups.conf
            COMPONENT plugin-apps
            DESTINATION usr/lib/netdata/conf.d)

    if(BUILD_FOR_PACKAGING)
        install(FILES
                ${PKG_FILES_PATH}/copyright
                COMPONENT plugin-apps
                DESTINATION usr/share/doc/netdata-plugin-apps)
    endif()
endif()

if(CAP_FOUND)
    set(HAVE_CAPABILITY True)
endif()

if(ENABLE_PLUGIN_FREEIPMI)
    pkg_check_modules(IPMI REQUIRED libipmimonitoring)

    set(FREEIPMI_PLUGIN_FILES src/collectors/freeipmi.plugin/freeipmi_plugin.c)

    add_executable(freeipmi.plugin ${FREEIPMI_PLUGIN_FILES})
    target_link_libraries (freeipmi.plugin libnetdata ${IPMI_LIBRARIES})
    target_include_directories(freeipmi.plugin PRIVATE ${IPMI_INCLUDE_DIRS})
    target_compile_options(freeipmi.plugin PRIVATE ${IPMI_CFLAGS_OTHER})

    install(TARGETS freeipmi.plugin
            COMPONENT plugin-freeipmi
            DESTINATION usr/libexec/netdata/plugins.d)

    if(BUILD_FOR_PACKAGING)
        install(FILES
                ${PKG_FILES_PATH}/copyright
                COMPONENT plugin-freeipmi
                DESTINATION usr/share/doc/netdata-plugin-freeipmi)
    endif()
endif()

if(ENABLE_PLUGIN_NFACCT)
    if (NOT MNL_FOUND)
        message(FATAL_ERROR "Can not build nfacct.plugin because MNL library could not be found.")
    endif()

    pkg_check_modules(NFACCT REQUIRED libnetfilter_acct)

    set(NFACCT_PLUGIN_FILES src/collectors/nfacct.plugin/plugin_nfacct.c)

    add_executable(nfacct.plugin ${NFACCT_PLUGIN_FILES})
    target_link_libraries (nfacct.plugin libnetdata ${MNL_LIBRARIES} ${NFACCT_LIBRARIES})
    target_include_directories(nfacct.plugin PRIVATE ${MNL_INCLUDE_DIRS} ${NFACCT_INCLUDE_DIRS})
    target_compile_options(nfacct.plugin PRIVATE ${MNL_CFLAGS_OTHER} ${NFACCT_CFLAGS_OTHER})

    install(TARGETS nfacct.plugin
            COMPONENT plugin-nfacct
            DESTINATION usr/libexec/netdata/plugins.d)

    if(BUILD_FOR_PACKAGING)
        install(FILES
                ${PKG_FILES_PATH}/copyright
                COMPONENT plugin-nfacct
                DESTINATION usr/share/doc/netdata-plugin-nfacct)
    endif()
endif()

if(ENABLE_PLUGIN_XENSTAT)
    pkg_check_modules(XENSTAT REQUIRED xenstat)
    pkg_check_modules(XENLIGHT REQUIRED xenlight)

    set(XENSTAT_PLUGIN_FILES src/collectors/xenstat.plugin/xenstat_plugin.c)

    add_executable(xenstat.plugin ${XENSTAT_PLUGIN_FILES})
    target_link_libraries (xenstat.plugin libnetdata ${XENLIGHT_LIBRARIES} ${XENSTAT_LIBRARIES})
    target_include_directories(xenstat.plugin PRIVATE ${XENLIGHT_INCLUDE_DIRS} ${XENSTAT_INCLUDE_DIRS})
    target_compile_options(xenstat.plugin PRIVATE ${XENLIGHT_CFLAGS_OTHER} ${XENSTAT_CFLAGS_OTHER})

    install(TARGETS xenstat.plugin
            COMPONENT plugin-xenstat
            DESTINATION usr/libexec/netdata/plugins.d)

    if(BUILD_FOR_PACKAGING)
        install(FILES
                ${PKG_FILES_PATH}/copyright
                COMPONENT plugin-xenstat
                DESTINATION usr/share/doc/netdata-plugin-xenstat)
    endif()
endif()

if(ENABLE_PLUGIN_PERF)
    set(PERF_PLUGIN_FILES src/collectors/perf.plugin/perf_plugin.c)

    add_executable(perf.plugin ${PERF_PLUGIN_FILES})
    target_link_libraries(perf.plugin libnetdata)

    install(TARGETS perf.plugin
            COMPONENT plugin-perf
            DESTINATION usr/libexec/netdata/plugins.d)

    if(BUILD_FOR_PACKAGING)
         install(FILES
                 ${PKG_FILES_PATH}/copyright
                 COMPONENT plugin-perf
                 DESTINATION usr/share/doc/netdata-plugin-perf)
    endif()
endif()

if(ENABLE_PLUGIN_SLABINFO)
    set(SLABINFO_PLUGIN_FILES src/collectors/slabinfo.plugin/slabinfo.c)

    add_executable(slabinfo.plugin ${SLABINFO_PLUGIN_FILES})
    target_link_libraries(slabinfo.plugin libnetdata)

    install(TARGETS slabinfo.plugin
            COMPONENT plugin-slabinfo
            DESTINATION usr/libexec/netdata/plugins.d)

    if(BUILD_FOR_PACKAGING)
        install(FILES
                ${PKG_FILES_PATH}/copyright
                COMPONENT plugin-slabinfo
                DESTINATION usr/share/doc/netdata-plugin-slabinfo)
    endif()
endif()


if(ENABLE_PLUGIN_CUPS)
    pkg_check_modules(CUPS libcups)
    if(NOT CUPS_FOUND)
        pkg_check_modules(CUPS cups)
        if(NOT CUPS_FOUND)
                find_program(CUPS_CONFIG cups-config)
                if(CUPS_CONFIG)
                        execute_process(COMMAND ${CUPS_CONFIG} --api-version OUTPUT_VARIABLE CUPS_API_VERSION OUTPUT_STRIP_TRAILING_WHITESPACE)
                        if(CUPS_API_VERSION VERSION_LESS "1.7")
                                set(CUPS_FOUND False)
                        else()
                                execute_process(COMMAND ${CUPS_CONFIG} --cflags OUTPUT_VARIABLE CUPS_CFLAGS_OTHER OUTPUT_STRIP_TRAILING_WHITESPACE)
                                execute_process(COMMAND ${CUPS_CONFIG} --libs OUTPUT_VARIABLE CUPS_LIBRARIES OUTPUT_STRIP_TRAILING_WHITESPACE)
                                set(CUPS_FOUND True)
                        endif()
                endif()
        endif()
    endif()

    if(NOT CUPS_FOUND)
        message(WARNING "Could not find cups cflags and libs.")
    else()
        set(CUPS_PLUGIN_FILES src/collectors/cups.plugin/cups_plugin.c)
        add_executable(cups.plugin ${CUPS_PLUGIN_FILES})
        target_link_libraries (cups.plugin libnetdata ${CUPS_LIBRARIES})
        target_compile_options(cups.plugin PRIVATE ${CUPS_CFLAGS_OTHER})

        install(TARGETS cups.plugin
                COMPONENT plugin-cups
                DESTINATION usr/libexec/netdata/plugins.d)

        if(BUILD_FOR_PACKAGING)
                install(FILES
                        ${PKG_FILES_PATH}/copyright
                        COMPONENT plugin-cups
                        DESTINATION usr/share/doc/netdata-plugin-cups)
        endif()
    endif()
endif()

if(NEED_NDSUDO)
    set(NDSUDO_FILES src/collectors/utils/ndsudo.c)

    add_executable(ndsudo ${NDSUDO_FILES})

    install(TARGETS ndsudo
            COMPONENT netdata
            DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_CGROUP_NETWORK)
    set(CGROUP_NETWORK_FILES src/collectors/cgroups.plugin/cgroup-network.c)

    add_executable(cgroup-network ${CGROUP_NETWORK_FILES})
    target_link_libraries(cgroup-network libnetdata)

    install(TARGETS cgroup-network
            COMPONENT netdata
            DESTINATION usr/libexec/netdata/plugins.d)
endif()

# Enable rust implementation if we don't have systemd and we want the journal plugin
if(ENABLE_PLUGIN_SYSTEMD_JOURNAL AND NOT SYSTEMD_FOUND)
        if (NOT ENABLE_NETDATA_JOURNAL_FILE_READER)
                message(WARNING "Systemd journal package not found, will try netdata's journal reader which requires cargo.")
                set(ENABLE_NETDATA_JOURNAL_FILE_READER True)
        endif()
endif()

if(ENABLE_PLUGIN_SYSTEMD_JOURNAL)
        add_executable(systemd-journal.plugin ${SYSTEMD_JOURNAL_PLUGIN_FILES})

        if(ENABLE_NETDATA_JOURNAL_FILE_READER)
                target_compile_definitions(systemd-journal.plugin PRIVATE HAVE_RUST_PROVIDER)
                target_link_libraries(systemd-journal.plugin journal_reader_ffi)
        endif()

        target_link_libraries(systemd-journal.plugin libnetdata)

        install(TARGETS systemd-journal.plugin
                COMPONENT plugin-systemd-journal
                DESTINATION usr/libexec/netdata/plugins.d)

        if(BUILD_FOR_PACKAGING)
                install(FILES
                        ${PKG_FILES_PATH}/copyright
                        COMPONENT plugin-systemd-journal
                        DESTINATION usr/share/doc/netdata-plugin-systemd-journal)
        endif()
endif()

if(ENABLE_PLUGIN_SYSTEMD_UNITS)
        if(NOT SYSTEMD_FOUND)
                message(FATAL_ERROR "Systemd units plugin requires systemd, but systemd was not found.")
        endif()

        if(SYSTEMD_VERSION LESS 221)
            message(FATAL_ERROR "Systemd units plugin requires systemd 221 or newer, but only systemd ${SYSTEMD_VERSION} was found.")
        endif()

        include(FetchContent)

        add_executable(systemd-units.plugin ${SYSTEMD_UNITS_PLUGIN_FILES})
        target_link_libraries(systemd-units.plugin libnetdata)

        install(TARGETS systemd-units.plugin
                COMPONENT plugin-systemd-units
                DESTINATION usr/libexec/netdata/plugins.d)

        if(BUILD_FOR_PACKAGING)
                install(FILES
                        ${PKG_FILES_PATH}/copyright
                        COMPONENT plugin-systemd-units
                        DESTINATION usr/share/doc/netdata-plugin-systemd-units)
        endif()
endif()

if(OS_WINDOWS)
        add_executable(windows-events.plugin ${WINDOWS_EVENTS_PLUGIN_FILES})
        target_link_libraries(windows-events.plugin libnetdata wevtapi)

        install(TARGETS windows-events.plugin
                COMPONENT plugin-windows-events
                DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_EBPF)
    set(EBPF_PLUGIN_FILES
            src/collectors/ebpf.plugin/ebpf.c
            src/collectors/ebpf.plugin/ebpf.h
            src/collectors/ebpf.plugin/ebpf_cachestat.c
            src/collectors/ebpf.plugin/ebpf_cachestat.h
            src/collectors/ebpf.plugin/ebpf_dcstat.c
            src/collectors/ebpf.plugin/ebpf_dcstat.h
            src/collectors/ebpf.plugin/ebpf_disk.c
            src/collectors/ebpf.plugin/ebpf_disk.h
            src/collectors/ebpf.plugin/ebpf_fd.c
            src/collectors/ebpf.plugin/ebpf_fd.h
            src/collectors/ebpf.plugin/ebpf_hardirq.c
            src/collectors/ebpf.plugin/ebpf_hardirq.h
            src/collectors/ebpf.plugin/ebpf_mdflush.c
            src/collectors/ebpf.plugin/ebpf_mdflush.h
            src/collectors/ebpf.plugin/ebpf_mount.c
            src/collectors/ebpf.plugin/ebpf_mount.h
            src/collectors/ebpf.plugin/ebpf_filesystem.c
            src/collectors/ebpf.plugin/ebpf_filesystem.h
            src/collectors/ebpf.plugin/ebpf_oomkill.c
            src/collectors/ebpf.plugin/ebpf_oomkill.h
            src/collectors/ebpf.plugin/ebpf_process.c
            src/collectors/ebpf.plugin/ebpf_process.h
            src/collectors/ebpf.plugin/ebpf_shm.c
            src/collectors/ebpf.plugin/ebpf_shm.h
            src/collectors/ebpf.plugin/ebpf_socket.c
            src/collectors/ebpf.plugin/ebpf_socket.h
            src/collectors/ebpf.plugin/ebpf_socket_ipc.c
            src/collectors/ebpf.plugin/ebpf_socket_ipc.h
            src/collectors/ebpf.plugin/ebpf_softirq.c
            src/collectors/ebpf.plugin/ebpf_softirq.h
            src/collectors/ebpf.plugin/ebpf_sync.c
            src/collectors/ebpf.plugin/ebpf_sync.h
            src/collectors/ebpf.plugin/ebpf_swap.c
            src/collectors/ebpf.plugin/ebpf_swap.h
            src/collectors/ebpf.plugin/ebpf_vfs.c
            src/collectors/ebpf.plugin/ebpf_vfs.h
            src/collectors/ebpf.plugin/ebpf_apps.c
            src/collectors/ebpf.plugin/ebpf_apps.h
            src/collectors/ebpf.plugin/ebpf_cgroup.c
            src/collectors/ebpf.plugin/ebpf_cgroup.h
            src/collectors/ebpf.plugin/ebpf_unittest.c
            src/collectors/ebpf.plugin/ebpf_unittest.h
            src/collectors/ebpf.plugin/ebpf_functions.c
            src/collectors/ebpf.plugin/ebpf_functions.h
            src/collectors/ebpf.plugin/libbpf_api/ebpf.c
            src/collectors/ebpf.plugin/libbpf_api/ebpf.h
    )

    add_executable(ebpf.plugin ${EBPF_PLUGIN_FILES} ${INTERCOMMUNICATION_COLLECTORS_FILES})
    target_link_libraries(ebpf.plugin libnetdata)

    netdata_add_libbpf_to_target(ebpf.plugin)
    netdata_add_ebpf_co_re_to_target(ebpf.plugin)

    install(TARGETS ebpf.plugin
            COMPONENT plugin-ebpf
            DESTINATION usr/libexec/netdata/plugins.d)

    install(FILES
            src/collectors/ebpf.plugin/ebpf.d.conf
            COMPONENT plugin-ebpf
            DESTINATION usr/lib/netdata/conf.d)

    install(FILES
            src/collectors/ebpf.plugin/ebpf.d/cachestat.conf
            src/collectors/ebpf.plugin/ebpf.d/dcstat.conf
            src/collectors/ebpf.plugin/ebpf.d/disk.conf
            src/collectors/ebpf.plugin/ebpf.d/ebpf_kernel_reject_list.txt
            src/collectors/ebpf.plugin/ebpf.d/fd.conf
            src/collectors/ebpf.plugin/ebpf.d/filesystem.conf
            src/collectors/ebpf.plugin/ebpf.d/hardirq.conf
            src/collectors/ebpf.plugin/ebpf.d/mdflush.conf
            src/collectors/ebpf.plugin/ebpf.d/mount.conf
            src/collectors/ebpf.plugin/ebpf.d/network.conf
            src/collectors/ebpf.plugin/ebpf.d/oomkill.conf
            src/collectors/ebpf.plugin/ebpf.d/process.conf
            src/collectors/ebpf.plugin/ebpf.d/shm.conf
            src/collectors/ebpf.plugin/ebpf.d/softirq.conf
            src/collectors/ebpf.plugin/ebpf.d/swap.conf
            src/collectors/ebpf.plugin/ebpf.d/sync.conf
            src/collectors/ebpf.plugin/ebpf.d/vfs.conf
            COMPONENT plugin-ebpf
            DESTINATION usr/lib/netdata/conf.d/ebpf.d)

    if(BUILD_FOR_PACKAGING)
        install(FILES
                ${PKG_FILES_PATH}/copyright
                COMPONENT plugin-ebpf
                DESTINATION usr/share/doc/netdata-plugin-ebpf)
    endif()

    if(ENABLE_LEGACY_EBPF_PROGRAMS)
        netdata_install_legacy_ebpf_code()
    endif()
endif()

if(ENABLE_PLUGIN_LOCAL_LISTENERS)
        set(LOCAL_LISTENERS_FILES
                src/collectors/utils/local_listeners.c
                src/libnetdata/local-sockets/local-sockets.h
        )

        add_executable(local-listeners ${LOCAL_LISTENERS_FILES})

        target_compile_options(local-listeners PRIVATE
                               "$<$<BOOL:${MNL_FOUND}>:${MNL_CFLAGS_OTHER}>")
        target_include_directories(local-listeners PRIVATE
                                   "$<$<BOOL:${MNL_FOUND}>:${MNL_INCLUDE_DIRS}>")
        target_link_libraries(local-listeners libnetdata
                              "$<$<BOOL:${MNL_FOUND}>:${MNL_LIBRARIES}>")

        install(TARGETS local-listeners
                COMPONENT netdata
                DESTINATION usr/libexec/netdata/plugins.d)
endif()

if(ENABLE_PLUGIN_NETWORK_VIEWER)
        set(NETWORK_VIEWER_FILES
                src/libnetdata/local-sockets/local-sockets.h
                src/collectors/network-viewer.plugin/network-viewer.c
        )

        add_executable(network-viewer.plugin ${NETWORK_VIEWER_FILES})

        target_compile_options(network-viewer.plugin PRIVATE
                               "$<$<BOOL:${MNL_FOUND}>:${MNL_CFLAGS_OTHER}>")
        target_include_directories(network-viewer.plugin PRIVATE
                                   "$<$<BOOL:${MNL_FOUND}>:${MNL_INCLUDE_DIRS}>")
        target_link_libraries(network-viewer.plugin libnetdata
                              "$<$<BOOL:${MNL_FOUND}>:${MNL_LIBRARIES}>")

        install(TARGETS network-viewer.plugin
                COMPONENT plugin-network-viewer
                DESTINATION usr/libexec/netdata/plugins.d)

        if(BUILD_FOR_PACKAGING)
                install(FILES
                        ${PKG_FILES_PATH}/copyright
                        COMPONENT plugin-network-viewer
                        DESTINATION usr/share/doc/netdata-plugin-network-viewer)
        endif()
endif()

#
# exporters
#

if(ENABLE_EXPORTER_MONGODB)
        pkg_check_modules(MONGOC libmongoc-1.0>=1.7)

        if(MONGOC_FOUND)
                SET(HAVE_MONGOC True)
        else()
                SET(ENABLE_EXPORTER_MONGODB False)
        endif()
endif()

if(ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE)
        pkg_check_modules(SNAPPY snappy)
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
        set(NETDATA_CLAIM_RES_FILES "packaging/windows/resources/netdata_claim.rc")
        configure_file(packaging/windows/resources/netdata_claim.manifest.in ${CMAKE_SOURCE_DIR}/packaging/windows/resources/netdata_claim.manifest @ONLY)

        set(NETDATACLI_RES_FILES "packaging/windows/resources/netdatacli.rc")
        configure_file(packaging/windows/resources/netdatacli.manifest.in ${CMAKE_SOURCE_DIR}/packaging/windows/resources/netdatacli.manifest @ONLY)

        set(NETDATA_RES_FILES "packaging/windows/resources/netdata.rc")
        configure_file(packaging/windows/resources/netdata.manifest.in ${CMAKE_SOURCE_DIR}/packaging/windows/resources/netdata.manifest @ONLY)

        configure_file(packaging/windows/netdata.wxs.in netdata.wxs @ONLY)
        configure_file(packaging/windows/NetdataWhite.ico NetdataWhite.ico COPYONLY)
        configure_file(packaging/windows/eula.rtf eula.rtf COPYONLY)
        configure_file(packaging/windows/Top.bmp Top.bmp COPYONLY)
        configure_file(packaging/windows/BackGround.bmp BackGround.bmp COPYONLY)
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
        "$<$<BOOL:${OS_LINUX}>:rt>"
        "$<$<BOOL:${ENABLE_MQTTWEBSOCKETS}>:mqttwebsockets>"
        "$<$<BOOL:${ENABLE_EXPORTER_MONGODB}>:${MONGOC_LIBRARIES}>"
        "$<$<BOOL:${ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE}>:${SNAPPY_LIBRARIES}>"
        "$<$<BOOL:${OS_MACOS}>:${IOKIT};${FOUNDATION}>"
        "$<$<BOOL:${ENABLE_SENTRY}>:sentry>"
        "$<$<BOOL:${ENABLE_WEBRTC}>:LibDataChannel::LibDataChannelStatic>"
        "$<$<BOOL:${CURL_FOUND}>:PkgConfig::CURL>"
        "$<$<BOOL:${OS_WINDOWS}>:odbc32;setupapi>"
)

netdata_add_protobuf(netdata)

#
# build systemd-cat-native
#
set(SYSTEMD_CAT_NATIVE_FILES src/libnetdata/log/systemd-cat-native.c
                             src/libnetdata/log/systemd-cat-native.h)

add_executable(systemd-cat-native ${SYSTEMD_CAT_NATIVE_FILES})
target_link_libraries(systemd-cat-native
        libnetdata
        "$<$<BOOL:${CURL_FOUND}>:PkgConfig::CURL>"
)

install(TARGETS systemd-cat-native
        COMPONENT netdata
        DESTINATION "${BINDIR}")

#
# build log2journal
#

pkg_check_modules(PCRE2 libpcre2-8)

if(PCRE2_FOUND)
        set(LOG2JOURNAL_FILES
                ${CONFIG_H}
                src/collectors/log2journal/log2journal.h
                src/collectors/log2journal/log2journal.c
                src/collectors/log2journal/log2journal-help.c
                src/collectors/log2journal/log2journal-yaml.c
                src/collectors/log2journal/log2journal-json.c
                src/collectors/log2journal/log2journal-logfmt.c
                src/collectors/log2journal/log2journal-pcre2.c
                src/collectors/log2journal/log2journal-params.c
                src/collectors/log2journal/log2journal-inject.c
                src/collectors/log2journal/log2journal-pattern.c
                src/collectors/log2journal/log2journal-replace.c
                src/collectors/log2journal/log2journal-rename.c
                src/collectors/log2journal/log2journal-rewrite.c
                src/collectors/log2journal/log2journal-txt.h
                src/collectors/log2journal/log2journal-hashed-key.h
        )

        add_executable(log2journal ${LOG2JOURNAL_FILES})
        target_include_directories(log2journal BEFORE PUBLIC ${CONFIG_H_DIR} ${CMAKE_SOURCE_DIR}/src ${PCRE2_INCLUDE_DIRS})
        target_compile_options(log2journal PUBLIC ${PCRE2_CFLAGS_OTHER})

        target_link_libraries(log2journal PUBLIC libnetdata)
        target_link_libraries(log2journal PUBLIC "${PCRE2_LDFLAGS}")
        netdata_add_libyaml_to_target(log2journal)

        install(TARGETS log2journal
                COMPONENT netdata
                DESTINATION "${BINDIR}")

        install(DIRECTORY src/collectors/log2journal/log2journal.d
                COMPONENT netdata
                DESTINATION usr/lib/netdata/conf.d)
endif()

#
# build netdatacli
#

set(NETDATACLI_FILES
        src/daemon/commands.h
        src/daemon/pipename.c
        src/daemon/pipename.h
        src/cli/cli.c
)

add_executable(netdatacli ${NETDATACLI_FILES} "$<$<BOOL:${OS_WINDOWS}>:${NETDATACLI_RES_FILES}>")
target_link_libraries(netdatacli libnetdata)

install(TARGETS netdatacli
        COMPONENT netdata
        DESTINATION "${BINDIR}")

#
# Build go.d.plugin
#

if(ENABLE_PLUGIN_GO)
    add_go_target(go-plugin go.d.plugin src/go cmd/godplugin)

    install(PROGRAMS ${CMAKE_BINARY_DIR}/go.d.plugin
            COMPONENT plugin-go
            DESTINATION usr/libexec/netdata/plugins.d)

    # Build and install nd-mcp (stdio-golang bridge) exactly like go.d.plugin
    if (OS_WINDOWS)
        set(ND_MCP_NAME nd-mcp.exe)
    else()
        set(ND_MCP_NAME nd-mcp)
    endif()

    add_go_target(nd-mcp-target ${ND_MCP_NAME} src/web/mcp/bridges/stdio-golang .)
    install(PROGRAMS
            ${CMAKE_BINARY_DIR}/${ND_MCP_NAME}
            COMPONENT plugin-go
            DESTINATION "${BINDIR}")
endif()

#
<<<<<<< HEAD
# otel.plugin
=======
# Build ibm.d.plugin
#

if(ENABLE_PLUGIN_IBM)
    include(NetdataIBMPlugin)
    add_ibm_plugin_target()
    
    install(PROGRAMS ${CMAKE_BINARY_DIR}/ibm.d.plugin
            COMPONENT plugin-ibm
            DESTINATION usr/libexec/netdata/plugins.d)
    
    # Install IBM plugin configuration files
    install(FILES src/go/plugin/ibm.d/config/ibm.d.conf
            COMPONENT plugin-ibm
            DESTINATION usr/lib/netdata/conf.d)
    
    install(FILES 
            src/go/plugin/ibm.d/config/ibm.d/as400.conf
            src/go/plugin/ibm.d/config/ibm.d/db2.conf
            src/go/plugin/ibm.d/config/ibm.d/websphere_liberty.conf
            src/go/plugin/ibm.d/config/ibm.d/websphere_mp.conf
            src/go/plugin/ibm.d/config/ibm.d/websphere_pmi.conf
            COMPONENT plugin-ibm
            DESTINATION usr/lib/netdata/conf.d/ibm.d)
    
    # Install IBM library download script
    install(PROGRAMS packaging/installer/install-ibm-libs.sh
            COMPONENT plugin-ibm
            DESTINATION usr/libexec/netdata)
endif()

#
# Handle OTel Collector plugin
>>>>>>> 2c6de1b08 (feat(ibm.d): add self-contained plugin with automatic library download)
#
if(ENABLE_PLUGIN_OTEL)
    corrosion_install(TARGETS otel-plugin
                      PERMISSIONS OWNER_READ OWNER_WRITE OWNER_EXECUTE GROUP_READ GROUP_EXECUTE
                      RUNTIME DESTINATION usr/libexec/netdata/plugins.d
                      COMPONENT plugin-otel)

    install(FILES src/crates/jf/otel-plugin/configs/otel.yml
            COMPONENT plugin-otel
            DESTINATION usr/lib/netdata/conf.d)

    install(FILES src/crates/jf/otel-plugin/configs/otel.d/v1/metrics/hostmetrics-receiver.yml
            COMPONENT plugin-otel
            DESTINATION usr/lib/netdata/conf.d/otel.d/v1/metrics)

    install(DIRECTORY COMPONENT plugin-otel DESTINATION etc/netdata/otel.d/v1/metrics)
endif()

#
# Generate config file
#
if(NOT NETDATA_RUNTIME_PREFIX STREQUAL "")
  string(REGEX REPLACE "/$" "" NETDATA_RUNTIME_PREFIX "${NETDATA_RUNTIME_PREFIX}")
endif()

set(CACHE_DIR "${NETDATA_RUNTIME_PREFIX}/var/cache/netdata")
set(CONFIG_DIR "${NETDATA_RUNTIME_PREFIX}/etc/netdata")
set(LIBCONFIG_DIR "${NETDATA_RUNTIME_PREFIX}/usr/lib/netdata/conf.d")
set(LOG_DIR "${NETDATA_RUNTIME_PREFIX}/var/log/netdata")
set(PLUGINS_DIR "${NETDATA_RUNTIME_PREFIX}/usr/libexec/netdata/plugins.d")
set(VARLIB_DIR "${NETDATA_RUNTIME_PREFIX}/var/lib/netdata")

# A non-default value is only used when building Debian packages (/var/lib/netdata/www)
if(NOT DEFINED WEB_DIR)
  set(WEB_DIR "usr/share/netdata/web")
else()
  string(REGEX REPLACE "^/" "" WEB_DIR "${WEB_DIR}")
endif()
set(WEB_DEST "${WEB_DIR}")
set(WEB_DIR "${NETDATA_RUNTIME_PREFIX}/${WEB_DEST}")

# Collect all compiler flags
get_directory_property(NETDATA_COMPILE_OPTIONS COMPILE_OPTIONS)
get_directory_property(NETDATA_COMPILE_DEFINITIONS COMPILE_DEFINITIONS)

# Collect all linker flags
get_directory_property(NETDATA_LINK_OPTIONS LINK_OPTIONS)
get_directory_property(NETDATA_LINK_LIBRARIES LINK_LIBRARIES)

list(APPEND CONFIGURE_OPTIONS
        # Build type and languages
        "-DCMAKE_BUILD_TYPE=${CMAKE_BUILD_TYPE}"
        "-DCMAKE_C_STANDARD=${CMAKE_C_STANDARD}"
        "-DCMAKE_CXX_STANDARD=${CMAKE_CXX_STANDARD}"
        "-DBUILD_SHARED_LIBS=${BUILD_SHARED_LIBS}"

        # Compiler flags (dynamically collected)
        "-DCMAKE_C_FLAGS='${CMAKE_C_FLAGS} ${NETDATA_COMPILE_OPTIONS}'"
        "-DCMAKE_CXX_FLAGS='${CMAKE_CXX_FLAGS} ${NETDATA_COMPILE_OPTIONS}'"
        "-DCMAKE_COMPILE_DEFINITIONS='${NETDATA_COMPILE_DEFINITIONS}'"

        # Linker flags (dynamically collected)
        "-DCMAKE_EXE_LINKER_FLAGS='${CMAKE_EXE_LINKER_FLAGS} ${NETDATA_LINK_OPTIONS}'"
        "-DCMAKE_SHARED_LINKER_FLAGS='${CMAKE_SHARED_LINKER_FLAGS}'"
)

string(JOIN " " CONFIGURE_COMMAND "cmake" ${CONFIGURE_OPTIONS})

if (NOT NETDATA_USER)
        set(NETDATA_USER "netdata")
endif()

set(BUILD_INFO_CMAKE_CACHE_ARCHIVE_NAME "build-info-cmake-cache.gz")
set(BUILD_INFO_CMAKE_CACHE_ARCHIVE_PATH "usr/share/netdata")

configure_file(packaging/cmake/config.cmake.h.in config.h)

#
# install
#

install(TARGETS netdata COMPONENT netdata DESTINATION "${BINDIR}")

install(DIRECTORY COMPONENT netdata DESTINATION var/cache/netdata)
install(DIRECTORY COMPONENT netdata DESTINATION var/log/netdata)
install(DIRECTORY COMPONENT netdata DESTINATION var/lib/netdata/registry)
install(DIRECTORY COMPONENT netdata DESTINATION var/lib/netdata/cloud.d)
install(DIRECTORY COMPONENT netdata DESTINATION var/run/netdata)
install(DIRECTORY COMPONENT netdata DESTINATION etc/netdata)
install(DIRECTORY COMPONENT netdata DESTINATION etc/netdata/custom-plugins.d)
install(DIRECTORY COMPONENT netdata DESTINATION etc/netdata/health.d)
install(DIRECTORY COMPONENT netdata DESTINATION etc/netdata/ssl)
install(DIRECTORY COMPONENT netdata DESTINATION etc/netdata/statsd.d)
install(DIRECTORY COMPONENT netdata DESTINATION usr/lib/netdata/conf.d)
install(DIRECTORY COMPONENT netdata DESTINATION usr/lib/netdata/conf.d/schema.d)
install(DIRECTORY COMPONENT netdata DESTINATION usr/libexec/netdata/plugins.d)
install(DIRECTORY COMPONENT netdata DESTINATION ${WEB_DEST})

set(libsysdir_POST "${NETDATA_RUNTIME_PREFIX}/usr/lib/netdata/system")
set(pkglibexecdir_POST "${NETDATA_RUNTIME_PREFIX}/usr/libexec/netdata")
set(localstatedir_POST "${NETDATA_RUNTIME_PREFIX}/var")
set(sbindir_POST "${NETDATA_RUNTIME_PREFIX}/${BINDIR}")
set(configdir_POST "${NETDATA_RUNTIME_PREFIX}/etc/netdata")
set(libconfigdir_POST "${NETDATA_RUNTIME_PREFIX}/usr/lib/netdata/conf.d")
set(cachedir_POST "${NETDATA_RUNTIME_PREFIX}/var/cache/netdata")
set(registrydir_POST "${NETDATA_RUNTIME_PREFIX}/var/lib/netdata/registry")
set(varlibdir_POST "${NETDATA_RUNTIME_PREFIX}/var/lib/netdata")
set(netdata_user_POST "${NETDATA_USER}")
set(netdata_group_POST "${NETDATA_USER}")

if(NOT OS_WINDOWS)
        configure_file(src/claim/netdata-claim.sh.in src/claim/netdata-claim.sh @ONLY)
        install(PROGRAMS
                ${CMAKE_BINARY_DIR}/src/claim/netdata-claim.sh
                COMPONENT netdata
                DESTINATION "${BINDIR}")
else()
        install(PROGRAMS
                ${CMAKE_BINARY_DIR}/NetdataClaim.exe
                COMPONENT netdata
                DESTINATION "${BINDIR}")
endif()

#
# We don't check ENABLE_PLUGIN_CGROUP_NETWORK because rpm builds assume
# the files exists unconditionally.
#
configure_file(src/collectors/cgroups.plugin/cgroup-network-helper.sh.in
               src/collectors/cgroups.plugin/cgroup-network-helper.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/collectors/cgroups.plugin/cgroup-network-helper.sh
        COMPONENT netdata
        DESTINATION usr/libexec/netdata/plugins.d)

configure_file(src/collectors/cgroups.plugin/cgroup-name.sh.in
               src/collectors/cgroups.plugin/cgroup-name.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/collectors/cgroups.plugin/cgroup-name.sh
        COMPONENT netdata
        DESTINATION usr/libexec/netdata/plugins.d)

#
# statsd
#
install(FILES
        src/collectors/statsd.plugin/asterisk.conf
        src/collectors/statsd.plugin/example.conf
        src/collectors/statsd.plugin/k6.conf
        COMPONENT netdata
        DESTINATION usr/lib/netdata/conf.d/statsd.d)

#
# exporting
#
install(FILES
        src/exporting/exporting.conf
        COMPONENT netdata
        DESTINATION usr/lib/netdata/conf.d)

#
# ioping.plugin
#
install(FILES
        src/collectors/ioping.plugin/ioping.conf
        COMPONENT netdata
        DESTINATION usr/lib/netdata/conf.d)

#
# streaming
#
install(FILES
        src/streaming/stream.conf
        COMPONENT netdata
        DESTINATION usr/lib/netdata/conf.d)

#
# swagger
#
install(FILES
        src/web/api/netdata-swagger.json
        src/web/api/netdata-swagger.yaml
        COMPONENT netdata
        DESTINATION ${WEB_DEST})

#
# service files
#

configure_file(system/install-service.sh.in system/install-service.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/system/install-service.sh
        COMPONENT netdata
        DESTINATION usr/libexec/netdata)

configure_file(system/launchd/netdata.plist.in system/launchd/netdata.plist @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/launchd/netdata.plist
        COMPONENT netdata
        DESTINATION usr/lib/netdata/system/launchd)

configure_file(system/freebsd/rc.d/netdata.in system/freebsd/rc.d/netdata @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/freebsd/rc.d/netdata
        COMPONENT netdata
        DESTINATION usr/lib/netdata/system/freebsd/rc.d)

configure_file(system/initd/init.d/netdata.in system/initd/init.d/netdata @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/initd/init.d/netdata
        COMPONENT netdata
        DESTINATION usr/lib/netdata/system/initd/init.d)

configure_file(system/logrotate/netdata.in system/logrotate/netdata @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/logrotate/netdata
        COMPONENT netdata
        DESTINATION usr/lib/netdata/system/logrotate)
install(FILES
        ${CMAKE_BINARY_DIR}/system/logrotate/netdata
        COMPONENT netdata
        DESTINATION etc/logrotate.d)

configure_file(system/lsb/init.d/netdata.in system/lsb/init.d/netdata @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/lsb/init.d/netdata
        COMPONENT netdata
        DESTINATION usr/lib/netdata/system/lsb/init.d)

configure_file(system/openrc/conf.d/netdata.in system/openrc/conf.d/netdata @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/openrc/conf.d/netdata
        COMPONENT netdata
        DESTINATION usr/lib/netdata/system/openrc/conf.d)

configure_file(system/openrc/init.d/netdata.in system/openrc/init.d/netdata @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/openrc/init.d/netdata
        COMPONENT netdata
        DESTINATION usr/lib/netdata/system/openrc/init.d)

configure_file(system/runit/run.in system/runit/run @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/runit/run
        COMPONENT netdata
        DESTINATION usr/lib/netdata/system/runit)

configure_file(system/dinit/netdata.in system/dinit/netdata @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/dinit/netdata
        COMPONENT netdata
        DESTINATION usr/lib/netdata/system/dinit)

configure_file(system/systemd/netdata.service.in system/systemd/netdata.service @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/systemd/netdata.service
        COMPONENT netdata
        DESTINATION usr/lib/netdata/system/systemd)

install(FILES
        system/systemd/journald@netdata.conf
        COMPONENT netdata
        DESTINATION usr/lib/netdata/system/systemd)

configure_file(system/systemd/netdata.service.v235.in system/systemd/netdata.service.v235 @ONLY)
install(FILES
        ${CMAKE_BINARY_DIR}/system/systemd/netdata.service.v235
        COMPONENT netdata
        DESTINATION usr/lib/netdata/system/systemd)


if(BUILD_FOR_PACKAGING)
        install(FILES
                ${CMAKE_BINARY_DIR}/system/systemd/netdata.service
                COMPONENT netdata
                DESTINATION lib/systemd/system)
        install(DIRECTORY
                COMPONENT netdata
                DESTINATION usr/lib/systemd/journald@netdata.conf.d)
        install(FILES
                system/systemd/journald@netdata.conf
                COMPONENT netdata
                DESTINATION usr/lib/systemd/journald@netdata.conf.d
                RENAME netdata.conf)
endif()

install(FILES
        system/systemd/50-netdata.preset
        COMPONENT netdata
        DESTINATION usr/lib/netdata/system/systemd)

install(FILES
        system/vnodes/vnodes.conf
        COMPONENT netdata
        DESTINATION usr/lib/netdata/conf.d/vnodes)

install(FILES
        system/.install-type
        COMPONENT netdata
        DESTINATION etc/netdata)

install(PROGRAMS
        system/edit-config
        COMPONENT netdata
        DESTINATION etc/netdata)

if(BUILD_FOR_PACKAGING)
        set(NETDATA_CONF_DEST "etc/netdata")
else()
        set(NETDATA_CONF_DEST "usr/lib/netdata/conf.d")
endif()

#
# misc files
#
if(BUILD_FOR_PACKAGING)
        install(FILES
                ${PKG_FILES_PATH}/deb/netdata/etc/default/netdata
                COMPONENT netdata
                DESTINATION etc/default)

        install(PROGRAMS
                ${PKG_FILES_PATH}/deb/netdata/etc/init.d/netdata
                COMPONENT netdata
                DESTINATION etc/init.d)
endif()

if(NOT OS_WINDOWS)
  install(PROGRAMS
          packaging/installer/netdata-updater.sh
          COMPONENT netdata
          DESTINATION usr/libexec/netdata)

  install(FILES
          system/netdata.conf
          system/netdata-updater.conf
          COMPONENT netdata
          DESTINATION ${NETDATA_CONF_DEST})

  configure_file(system/cron/netdata-updater-daily.in
                 system/cron/netdata-updater-daily
                 @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/cron/netdata-updater-daily
          COMPONENT netdata
          DESTINATION usr/lib/netdata/system/cron)

  configure_file(system/systemd/netdata-updater.service.in
                 system/systemd/netdata-updater.service
                 @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/systemd/netdata-updater.service
          COMPONENT netdata
          DESTINATION usr/lib/netdata/system/systemd)

  install(FILES
          system/systemd/netdata-updater.timer
          COMPONENT netdata
          DESTINATION usr/lib/netdata/system/systemd)

  if(BUILD_FOR_PACKAGING)
    install(FILES
            ${CMAKE_BINARY_DIR}/system/systemd/netdata-updater.service
            COMPONENT netdata
            DESTINATION lib/systemd/system)
    install(FILES
            system/systemd/netdata-updater.timer
            COMPONENT netdata
            DESTINATION lib/systemd/system)
  endif()
endif()

#
# TODO: check the following files for correct substitutions
#
configure_file(src/daemon/anonymous-statistics.sh.in src/daemon/anonymous-statistics.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/daemon/anonymous-statistics.sh
        COMPONENT netdata
        DESTINATION usr/libexec/netdata/plugins.d)

configure_file(src/daemon/get-kubernetes-labels.sh.in src/daemon/get-kubernetes-labels.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/daemon/get-kubernetes-labels.sh
        COMPONENT netdata
        DESTINATION usr/libexec/netdata/plugins.d)

install(PROGRAMS
        src/daemon/system-info.sh
        COMPONENT netdata
        DESTINATION usr/libexec/netdata/plugins.d)

#
# health files
#

file(GLOB_RECURSE HEALTH_CONF_FILES "src/health/health.d/*.conf")
install(FILES
        ${HEALTH_CONF_FILES}
        COMPONENT netdata
        DESTINATION usr/lib/netdata/conf.d/health.d)

configure_file(src/health/notifications/alarm-notify.sh.in src/health/notifications/alarm-notify.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/health/notifications/alarm-notify.sh
        COMPONENT netdata
        DESTINATION usr/libexec/netdata/plugins.d)

if(CMAKE_BUILD_TYPE STREQUAL "Debug")
        install(PROGRAMS
                src/health/notifications/alarm-email.sh
                src/health/notifications/alarm-test.sh
                COMPONENT netdata
                DESTINATION usr/libexec/netdata/plugins.d)
endif()

install(FILES
        src/health/notifications/health_alarm_notify.conf
        src/health/notifications/health_email_recipients.conf
        COMPONENT netdata
        DESTINATION usr/lib/netdata/conf.d)
#
# test/ files
#

if(CMAKE_BUILD_TYPE STREQUAL "Debug")
        configure_file(tests/health_mgmtapi/health-cmdapi-test.sh.in tests/health_mgmtapi/health-cmdapi-test.sh @ONLY)
        configure_file(tests/acls/acl.sh.in tests/acls/acl.sh @ONLY)
        configure_file(tests/urls/request.sh.in tests/urls/request.sh @ONLY)
        configure_file(tests/alarm_repetition/alarm.sh.in tests/alarm_repetition/alarm.sh @ONLY)
        configure_file(tests/template_dimension/template_dim.sh.in tests/template_dimension/template_dim.sh @ONLY)
        configure_file(tests/ebpf/ebpf_thread_function.sh.in tests/ebpf/ebpf_thread_function.sh @ONLY)

        install(FILES
                ${CMAKE_BINARY_DIR}/tests/health_mgmtapi/health-cmdapi-test.sh
                ${CMAKE_BINARY_DIR}/tests/acls/acl.sh
                ${CMAKE_BINARY_DIR}/tests/urls/request.sh
                ${CMAKE_BINARY_DIR}/tests/alarm_repetition/alarm.sh
                ${CMAKE_BINARY_DIR}/tests/template_dimension/template_dim.sh
                ${CMAKE_BINARY_DIR}/tests/ebpf/ebpf_thread_function.sh
                DESTINATION usr/libexec/netdata/plugins.d)
endif()

#
# charts.d plugin
#

if(ENABLE_PLUGIN_CHARTS)
  install(DIRECTORY COMPONENT plugin-chartsd DESTINATION etc/netdata/charts.d)

  configure_file(src/collectors/charts.d.plugin/charts.d.plugin.in src/collectors/charts.d.plugin/charts.d.plugin @ONLY)
  install(PROGRAMS
          ${CMAKE_BINARY_DIR}/src/collectors/charts.d.plugin/charts.d.plugin
          COMPONENT plugin-chartsd
          DESTINATION usr/libexec/netdata/plugins.d)

  install(PROGRAMS
          src/collectors/charts.d.plugin/charts.d.dryrun-helper.sh
          COMPONENT plugin-chartsd
          DESTINATION usr/libexec/netdata/plugins.d)

  install(FILES
          src/collectors/charts.d.plugin/charts.d.conf
          COMPONENT plugin-chartsd
          DESTINATION usr/lib/netdata/conf.d)

  install(PROGRAMS
          src/collectors/charts.d.plugin/example/example.chart.sh
          src/collectors/charts.d.plugin/libreswan/libreswan.chart.sh
          src/collectors/charts.d.plugin/opensips/opensips.chart.sh
          COMPONENT plugin-chartsd
          DESTINATION usr/libexec/netdata/charts.d)

  install(FILES
          src/collectors/charts.d.plugin/example/example.conf
          src/collectors/charts.d.plugin/libreswan/libreswan.conf
          src/collectors/charts.d.plugin/opensips/opensips.conf
          COMPONENT plugin-chartsd
          DESTINATION usr/lib/netdata/conf.d/charts.d)

  if(BUILD_FOR_PACKAGING)
    install(FILES
            ${PKG_FILES_PATH}/copyright
            COMPONENT plugin-chartsd
            DESTINATION usr/share/doc/netdata-plugin-chartsd)
  endif()
endif()

# This is needed both by the TC plugin (which only gets built on Linux) and the charts plugin.
if(OS_LINUX OR ENABLE_PLUGIN_CHARTS)
  install(FILES
          src/collectors/charts.d.plugin/loopsleepms.sh.inc
          COMPONENT netdata
          DESTINATION usr/libexec/netdata/plugins.d)
endif()

#
# tc-qos-helper
#

if(OS_LINUX)
  configure_file(src/collectors/tc.plugin/tc-qos-helper.sh.in src/collectors/tc.plugin/tc-qos-helper.sh @ONLY)
  install(PROGRAMS
          ${CMAKE_BINARY_DIR}/src/collectors/tc.plugin/tc-qos-helper.sh
          COMPONENT netdata
          DESTINATION usr/libexec/netdata/plugins.d)
endif()

# confs
install(FILES
        src/collectors/systemd-journal.plugin/schema.d/systemd-journal%3Amonitored-directories.json
        src/health/schema.d/health%3Aalert%3Aprototype.json
        COMPONENT netdata
        DESTINATION usr/lib/netdata/conf.d/schema.d)

#
# python.d plugin
#

if(ENABLE_PLUGIN_PYTHON)
  install(DIRECTORY COMPONENT plugin-pythond DESTINATION etc/netdata/python.d)

  configure_file(src/collectors/python.d.plugin/python.d.plugin.in src/collectors/python.d.plugin/python.d.plugin @ONLY)
  install(PROGRAMS ${CMAKE_BINARY_DIR}/src/collectors/python.d.plugin/python.d.plugin
          COMPONENT plugin-pythond
          DESTINATION usr/libexec/netdata/plugins.d)

  install(DIRECTORY src/collectors/python.d.plugin/python_modules
          COMPONENT plugin-pythond
          DESTINATION usr/libexec/netdata/python.d)

  if(OS_WINDOWS)
    include(NetdataUtil)
    precompile_python(usr/libexec/netdata/python.d plugin-pythond)
  endif()

  install(FILES src/collectors/python.d.plugin/python.d.conf
          COMPONENT plugin-pythond
          DESTINATION usr/lib/netdata/conf.d)

  install(FILES
          src/collectors/python.d.plugin/am2320/am2320.conf
          src/collectors/python.d.plugin/go_expvar/go_expvar.conf
          src/collectors/python.d.plugin/haproxy/haproxy.conf
          src/collectors/python.d.plugin/pandas/pandas.conf
          src/collectors/python.d.plugin/traefik/traefik.conf
          COMPONENT plugin-pythond
          DESTINATION usr/lib/netdata/conf.d/python.d)

  install(FILES
          src/collectors/python.d.plugin/am2320/am2320.chart.py
          src/collectors/python.d.plugin/go_expvar/go_expvar.chart.py
          src/collectors/python.d.plugin/haproxy/haproxy.chart.py
          src/collectors/python.d.plugin/pandas/pandas.chart.py
          src/collectors/python.d.plugin/traefik/traefik.chart.py
          COMPONENT plugin-pythond
          DESTINATION usr/libexec/netdata/python.d)

  if(BUILD_FOR_PACKAGING)
    install(FILES
            ${PKG_FILES_PATH}/copyright
            COMPONENT plugin-pythond
            DESTINATION usr/share/doc/netdata-plugin-pythond)
  endif()
endif()

#
# ioping.plugin
#

configure_file(src/collectors/ioping.plugin/ioping.plugin.in src/collectors/ioping.plugin/ioping.plugin @ONLY)
install(PROGRAMS ${CMAKE_BINARY_DIR}/src/collectors/ioping.plugin/ioping.plugin
        COMPONENT netdata
        DESTINATION usr/libexec/netdata/plugins.d)

#
# go.d.plugin
#
if(ENABLE_PLUGIN_GO)
    install(DIRECTORY COMPONENT plugin-go DESTINATION etc/netdata/go.d)

    install(FILES src/go/plugin/go.d/config/go.d.conf
            COMPONENT plugin-go
            DESTINATION usr/lib/netdata/conf.d)

    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION usr/lib/netdata/conf.d/go.d)
    file(GLOB GO_CONF_FILES src/go/plugin/go.d/config/go.d/*.conf)
    install(FILES ${GO_CONF_FILES}
            COMPONENT plugin-go
            DESTINATION usr/lib/netdata/conf.d/go.d)

    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION usr/lib/netdata/conf.d/go.d/sd)
    file(GLOB GO_SD_CONF_FILES src/go/plugin/go.d/config/go.d/sd/*.conf)
    install(FILES ${GO_SD_CONF_FILES}
            COMPONENT plugin-go
            DESTINATION usr/lib/netdata/conf.d/go.d/sd)

    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION usr/lib/netdata/conf.d/go.d/snmp.profiles)
    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION usr/lib/netdata/conf.d/go.d/snmp.profiles/default)
    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION usr/lib/netdata/conf.d/go.d/snmp.profiles/metadata)

    file(GLOB GO_SNMP_PROFILE_FILES src/go/plugin/go.d/config/go.d/snmp.profiles/default/*.yaml)
    install(FILES ${GO_SNMP_PROFILE_FILES}
            COMPONENT plugin-go
            DESTINATION usr/lib/netdata/conf.d/go.d/snmp.profiles/default)
    file(GLOB GO_SNMP_META_FILES src/go/plugin/go.d/config/go.d/snmp.profiles/metadata/*.yaml)
    install(FILES ${GO_SNMP_META_FILES}
            COMPONENT plugin-go
            DESTINATION usr/lib/netdata/conf.d/go.d/snmp.profiles/metadata)

    if(BUILD_FOR_PACKAGING)
        install(FILES
                ${PKG_FILES_PATH}/copyright
                COMPONENT plugin-go
                DESTINATION usr/share/doc/netdata-plugin-go)
    endif()
endif()

#
# ibm.d.plugin (requires CGO)
#

include(src/go/plugin/ibm.d/CMakeLists.txt)

#
# dashboard
#

if(ENABLE_DASHBOARD)
  include(NetdataDashboard)
  bundle_dashboard()

  install(FILES
          COMPONENT dashboard
          DESTINATION ${WEB_DEST})

endif()

#
# Ship CMakeCache.txt as an archive
#

install(CODE "
        execute_process(COMMAND gzip -nc \"${CMAKE_BINARY_DIR}/CMakeCache.txt\"
                        OUTPUT_FILE \"${BUILD_INFO_CMAKE_CACHE_ARCHIVE_NAME}\"
                        WORKING_DIRECTORY \"${CMAKE_BINARY_DIR}\"
                        RESULT_VARIABLE result)
        if(NOT result EQUAL 0)
                message(WARNING \"Failed to compress CMakeCache.txt\")
        endif()

        file(INSTALL \"${CMAKE_BINARY_DIR}/${BUILD_INFO_CMAKE_CACHE_ARCHIVE_NAME}\"
             DESTINATION \"\${CMAKE_INSTALL_PREFIX}/${BUILD_INFO_CMAKE_CACHE_ARCHIVE_PATH}\")
")

#
# vendor msys stuff on Windows
#

if(OS_WINDOWS)
        install(FILES /usr/bin/msys-protobuf-32.dll
                      /usr/bin/msys-uv-1.dll
                      DESTINATION "${BINDIR}")

        # Make bash & netdata happy
        install(DIRECTORY DESTINATION tmp)

        # Make curl work with ssl
        install(DIRECTORY /usr/ssl DESTINATION usr)
endif()

#
# Include packaging logic
#

include(Packaging)
